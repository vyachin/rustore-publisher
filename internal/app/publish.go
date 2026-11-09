package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vyachin/rustore-publisher/internal/artifact"
	"github.com/vyachin/rustore-publisher/internal/exitcode"
	"github.com/vyachin/rustore-publisher/internal/rustore"
)

// Фазы публикации.
const (
	PhaseCreate     = "create"
	PhaseUpload     = "upload"
	PhaseProcessing = "processing"
	PhaseCommit     = "commit"
)

// Итоги публикации.
const (
	OutcomeSubmitted = "submitted"
	OutcomeSkipped   = "skipped"
	OutcomeDryRun    = "dry_run"
)

// PublishInput — проверенные локальные данные для публикации.
type PublishInput struct {
	Package           string
	Bundle            *artifact.Bundle
	Changelog         *string
	PublishType       string
	IfExists          string
	DryRun            bool
	ProcessingTimeout time.Duration
	PollInterval      time.Duration
	Now               func() time.Time
	Sleep             func(context.Context, time.Duration) error
}

// PublishReport — накопленные факты публикации.
type PublishReport struct {
	VersionID      int64
	Version        *rustore.Version
	Detail         *rustore.VersionDetail
	Upload         *rustore.UploadResult
	Outcome        string
	Skipped        bool
	OutcomeUnknown bool
	Warnings       []Warning
}

// Publish выполняет публикацию переданного AAB по контракту 0.0.1.
func Publish(ctx context.Context, client *rustore.Client, in PublishInput, progress *Progress) (PublishReport, error) {
	report := PublishReport{}
	if in.Bundle == nil || in.Bundle.Identity.VersionCode <= 0 {
		return report, exitcode.Errorf(exitcode.KindInternal, "публикация без проверенного AAB")
	}
	if in.PollInterval <= 0 {
		in.PollInterval = 5 * time.Second
	}
	if in.Now == nil {
		in.Now = time.Now
	}
	if in.Sleep == nil {
		in.Sleep = sleepContext
	}

	versionCode := in.Bundle.Identity.VersionCode

	if progress != nil {
		progress.Phase = PhaseAuth
	}
	if _, err := client.Token(ctx); err != nil {
		return report, err
	}

	if progress != nil {
		progress.Phase = PhaseStatus
	}
	versions, err := client.ListVersions(ctx, in.Package, rustore.ListOptions{TestingType: "ALL"})
	if err != nil {
		return report, err
	}

	active := SelectActive(versions)
	if active == nil {
		return report, exitcode.WithHint(
			exitcode.Errorf(exitcode.KindVersionStateConflict,
				"у приложения %s нет активной версии", in.Package),
			"создайте и опубликуйте первую версию в консоли RuStore",
		)
	}

	if drafts := liveDrafts(versions); len(drafts) > 0 {
		report.VersionID = drafts[0].VersionID
		report.Version = drafts[0]
		return report, exitcode.WithHint(
			exitcode.Errorf(exitcode.KindExistingDraft,
				"у приложения уже есть черновик %d (код %d)",
				drafts[0].VersionID, drafts[0].VersionCode),
			"завершите или удалите черновик в консоли RuStore; CLI не заменяет его автоматически",
		)
	}

	matches := matchingReleases(versions, versionCode)
	if len(matches) > 1 {
		return report, exitcode.WithHint(
			exitcode.Errorf(exitcode.KindVersionStateConflict,
				"найдено несколько релизных версий с кодом %d", versionCode),
			"уточните состояние в консоли RuStore",
		)
	}
	if len(matches) == 1 {
		existing := matches[0]
		report.VersionID = existing.VersionID
		report.Version = existing
		if err := decideExisting(existing, in.IfExists); err != nil {
			return report, err
		}
		if in.IfExists == "skip" {
			report.Outcome = OutcomeSkipped
			report.Skipped = true
			if in.Changelog == nil {
				report.Warnings = append(report.Warnings, changelogInheritedWarning())
			}
			return report, nil
		}
	}

	if versionCode <= active.VersionCode {
		return report, exitcode.WithHint(
			exitcode.Errorf(exitcode.KindVersionStateConflict,
				"код версии AAB %d не больше активного %d", versionCode, active.VersionCode),
			"соберите AAB с большим кодом версии",
		)
	}

	if in.DryRun {
		report.Outcome = OutcomeDryRun
		if in.Changelog == nil {
			report.Warnings = append(report.Warnings, changelogInheritedWarning())
		}
		return report, nil
	}

	if progress != nil {
		progress.Phase = PhaseCreate
	}
	versionID, err := client.CreateDraft(ctx, in.Package, rustore.CreateDraftRequest{
		PublishType:  in.PublishType,
		PartialValue: 100,
		WhatsNew:     in.Changelog,
	})
	if err != nil {
		markUnknown(&report, err)
		return report, err
	}
	report.VersionID = versionID

	if progress != nil {
		progress.Phase = PhaseCreate
	}
	detail, err := client.VersionDetail(ctx, in.Package, versionID)
	if err != nil {
		return report, err
	}
	if err := verifyDraft(&detail, in.PublishType, in.Changelog); err != nil {
		return report, err
	}
	report.Detail = &detail
	report.Version = versionFromDetail(&detail)

	if progress != nil {
		progress.Phase = PhaseUpload
	}
	upload, err := client.UploadAAB(ctx, in.Package, versionID, in.Bundle.Info.Path)
	if err != nil {
		recovered, recoverErr := recoverUpload(ctx, client, in.Package, versionID, versionCode)
		if recoverErr != nil || !recovered {
			markUnknown(&report, err)
			return report, err
		}
	}
	report.Upload = &upload

	if progress != nil {
		progress.Phase = PhaseProcessing
	}
	deadline := in.Now().Add(in.ProcessingTimeout)
	if err := waitForPackages(ctx, client, in.Package, versionID, versionCode, deadline, in); err != nil {
		return report, err
	}

	if progress != nil {
		progress.Phase = PhaseCommit
	}
	if err := submit(ctx, client, in, versionID, versionCode, deadline); err != nil {
		markUnknown(&report, err)
		return report, err
	}

	report.Outcome = OutcomeSubmitted
	report.Skipped = false
	if in.Changelog == nil {
		report.Warnings = append(report.Warnings, changelogInheritedWarning())
	}

	if progress != nil {
		progress.Phase = PhaseStatus
	}
	readBack, err := client.ListVersions(ctx, in.Package,
		rustore.ListOptions{IDs: []int64{versionID}})
	if err != nil {
		report.Warnings = append(report.Warnings, Warning{
			Code:    WarningStatusUnavailable,
			Message: fmt.Sprintf("статус версии %d после отправки недоступен: %v", versionID, err),
		})
	} else if version := FindVersion(readBack, versionID); version != nil {
		report.Version = version
	} else {
		report.Warnings = append(report.Warnings, Warning{
			Code:    WarningStatusUnavailable,
			Message: fmt.Sprintf("версия %d не найдена при чтении статуса после отправки", versionID),
		})
	}

	return report, nil
}

// submit отправляет черновик на модерацию с ограниченными повторами.
func submit(ctx context.Context, client *rustore.Client, in PublishInput,
	versionID, versionCode int64, deadline time.Time,
) error {
	for {
		err := client.Commit(ctx, in.Package, versionID)
		if err == nil {
			return nil
		}

		if isPackagesNotFound(err) {
			if !in.Now().Add(in.PollInterval).Before(deadline) {
				return processingTimeout(versionID, err)
			}
			if err := sleep(ctx, in, in.PollInterval); err != nil {
				return err
			}
			continue
		}

		if isHMSOnly(err) {
			detail, detailErr := client.VersionDetail(ctx, in.Package, versionID)
			if detailErr == nil && hasPackages(&detail, versionCode) && allHMS(&detail, versionCode) {
				return exitcode.WithHint(
					exitcode.Wrap(exitcode.KindBuildRejected, err),
					"в AAB нет основного пакета с сервисом, отличным от HMS; исправьте сборку",
				)
			}
			if !in.Now().Add(in.PollInterval).Before(deadline) {
				return processingTimeout(versionID, err)
			}
			if err := sleep(ctx, in, in.PollInterval); err != nil {
				return err
			}
			continue
		}

		if exitcode.KindOf(err) == exitcode.KindOutcomeUnknown {
			if proveSubmitted(ctx, client, in.Package, versionID) {
				return nil
			}
			return exitcode.WithHint(
				exitcode.Wrap(exitcode.KindOutcomeUnknown,
					fmt.Errorf("результат отправки версии %d неизвестен", versionID)),
				"проверьте версию командой status и в консоли RuStore; не запускайте новую публикацию до выяснения",
			)
		}

		return err
	}
}

// waitForPackages ждёт появления основного пакета в пределах бюджета.
func waitForPackages(ctx context.Context, client *rustore.Client, packageName string,
	versionID, versionCode int64, deadline time.Time, in PublishInput,
) error {
	for in.Now().Before(deadline) {
		detail, err := client.VersionDetail(ctx, packageName, versionID)
		if err == nil && hasPackages(&detail, versionCode) {
			return nil
		}
		if err := sleep(ctx, in, in.PollInterval); err != nil {
			return err
		}
	}
	// Бюджет истёк: commit может дать подтверждённый отказ о неготовности,
	// который обрабатывается в submit.
	return nil
}

// recoverUpload проверяет, не применилась ли загрузка несмотря на ошибку.
func recoverUpload(ctx context.Context, client *rustore.Client, packageName string,
	versionID, versionCode int64,
) (bool, error) {
	detail, err := client.VersionDetail(ctx, packageName, versionID)
	if err != nil {
		return false, err
	}
	return hasPackages(&detail, versionCode), nil
}

// decideExisting применяет таблицу состояний совпавшей версии.
func decideExisting(version *rustore.Version, ifExists string) error {
	switch {
	case version.Status == rustore.StatusDraft:
		return exitcode.WithHint(
			exitcode.Errorf(exitcode.KindExistingDraft,
				"версия %d с кодом %d уже является черновиком",
				version.VersionID, version.VersionCode),
			"завершите или удалите черновик в консоли RuStore",
		)
	case version.Status == rustore.StatusArchived:
		return exitcode.WithHint(
			exitcode.Errorf(exitcode.KindVersionUnavailable,
				"версия %d с кодом %d находится в архиве",
				version.VersionID, version.VersionCode),
			"соберите AAB с новым кодом версии",
		)
	case rejectedStatus(version.Status):
		return exitcode.WithHint(
			exitcode.Errorf(exitcode.KindVersionRejected,
				"версия %d с кодом %d отклонена: %s",
				version.VersionID, version.VersionCode, version.Status),
			"исправьте замечания и соберите AAB с новым кодом версии",
		)
	case submittedStatus(version.Status):
		if ifExists == "skip" {
			return nil
		}
		return exitcode.WithHint(
			exitcode.Errorf(exitcode.KindVersionExists,
				"версия %d с кодом %d уже отправлена: %s",
				version.VersionID, version.VersionCode, version.Status),
			"используйте --if-exists=skip, если это ожидаемо",
		)
	default:
		if version.SendDateForModer != nil && *version.SendDateForModer != "" {
			if ifExists == "skip" {
				return nil
			}
			return exitcode.WithHint(
				exitcode.Errorf(exitcode.KindVersionExists,
					"версия %d с кодом %d уже отправлена", version.VersionID, version.VersionCode),
				"используйте --if-exists=skip, если это ожидаемо",
			)
		}
		return exitcode.WithHint(
			exitcode.Errorf(exitcode.KindVersionStateConflict,
				"состояние версии %d с кодом %d неоднозначно: %s",
				version.VersionID, version.VersionCode, version.Status),
			"проверьте версию в консоли RuStore",
		)
	}
}

// verifyDraft сверяет сохранённые настройки созданного черновика.
func verifyDraft(detail *rustore.VersionDetail, publishType string, whatsNew *string) error {
	var differences []string
	if !strings.EqualFold(detail.PublishType, publishType) {
		differences = append(differences,
			fmt.Sprintf("publishType: ожидался %s, сохранён %q", publishType, detail.PublishType))
	}
	if detail.PartialValue == nil || *detail.PartialValue != 100 {
		differences = append(differences, "partialValue: ожидался 100")
	}
	if whatsNew != nil {
		stored := ""
		if detail.WhatsNew != nil {
			stored = *detail.WhatsNew
		}
		if stored != *whatsNew {
			differences = append(differences, "whatsNew не совпадает с переданным")
		}
	}
	if detail.Status != rustore.StatusDraft {
		differences = append(differences,
			fmt.Sprintf("versionStatus: ожидался DRAFT, получен %q", detail.Status))
	}
	if len(differences) == 0 {
		return nil
	}
	return exitcode.WithHint(
		exitcode.Errorf(exitcode.KindRemoteStateMismatch,
			"сервер сохранил черновик %d иначе: %s", detail.VersionID, strings.Join(differences, "; ")),
		"не исправляйте настройки автоматически; проверьте черновик в консоли RuStore",
	)
}

// matchedReleases выбирает релизные версии с заданным кодом.
func matchingReleases(versions []rustore.Version, versionCode int64) []*rustore.Version {
	var matches []*rustore.Version
	for i := range versions {
		version := &versions[i]
		if !version.Testing.IsRelease() || version.VersionCode != versionCode {
			continue
		}
		if version.Status == rustore.StatusDeletedDraft {
			continue
		}
		matches = append(matches, version)
	}
	return matches
}

func liveDrafts(versions []rustore.Version) []*rustore.Version {
	var drafts []*rustore.Version
	for i := range versions {
		if versions[i].Status == rustore.StatusDraft {
			drafts = append(drafts, &versions[i])
		}
	}
	return drafts
}

func submittedStatus(status string) bool {
	switch status {
	case rustore.StatusAutoCheck, rustore.StatusTakenForModeration,
		rustore.StatusModeration, rustore.StatusReadyForPublication,
		rustore.StatusActive, rustore.StatusPartialActive,
		rustore.StatusPreviousActive:
		return true
	}
	return false
}

func rejectedStatus(status string) bool {
	switch status {
	case rustore.StatusRejectedByModerator, rustore.StatusRejectedBySecurity,
		rustore.StatusAutoCheckFailed:
		return true
	}
	return false
}

// proveSubmitted проверяет по списку версий, что commit всё-таки применился.
func proveSubmitted(ctx context.Context, client *rustore.Client, packageName string, versionID int64) bool {
	versions, err := client.ListVersions(ctx, packageName,
		rustore.ListOptions{IDs: []int64{versionID}})
	if err != nil {
		return false
	}
	version := FindVersion(versions, versionID)
	if version == nil {
		return false
	}
	if submittedStatus(version.Status) {
		return true
	}
	return version.SendDateForModer != nil && *version.SendDateForModer != ""
}

func hasPackages(detail *rustore.VersionDetail, versionCode int64) bool {
	for _, pkg := range detail.Packages {
		if pkg.VerCode == versionCode {
			return true
		}
	}
	return false
}

func allHMS(detail *rustore.VersionDetail, versionCode int64) bool {
	found := false
	for _, pkg := range detail.Packages {
		if pkg.VerCode != versionCode {
			continue
		}
		found = true
		if !strings.EqualFold(pkg.ServicesType, "HMS") {
			return false
		}
	}
	return found
}

func isPackagesNotFound(err error) bool {
	message := strings.ToLower(apiMessage(err))
	return strings.Contains(message, "packages for version with id") ||
		strings.Contains(message, "не загрузили apk")
}

func isHMSOnly(err error) bool {
	message := strings.ToLower(apiMessage(err))
	return strings.Contains(message, "non-hms") ||
		strings.Contains(message, "hms apk-file")
}

func apiMessage(err error) string {
	var apiErr *rustore.APIError
	if errors.As(err, &apiErr) {
		return apiErr.Message
	}
	return ""
}

func processingTimeout(versionID int64, cause error) error {
	return exitcode.WithHint(
		exitcode.Wrap(exitcode.KindAABProcessingTimeout,
			fmt.Errorf("обработка версии %d не завершилась: %w", versionID, cause)),
		fmt.Sprintf("проверьте версию %d командой status и повторите отправку в консоли RuStore", versionID),
	)
}

// markUnknown отмечает недоказанный результат записи.
func markUnknown(report *PublishReport, err error) {
	if exitcode.KindOf(err) == exitcode.KindOutcomeUnknown {
		report.OutcomeUnknown = true
	}
}

func changelogInheritedWarning() Warning {
	return Warning{
		Code:    "CHANGELOG_INHERITED",
		Message: "Список изменений не передан: пользователи увидят заметки активной версии.",
	}
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func sleep(ctx context.Context, in PublishInput, delay time.Duration) error {
	return in.Sleep(ctx, delay)
}

// versionFromDetail собирает версию из подробностей для результата.
func versionFromDetail(detail *rustore.VersionDetail) *rustore.Version {
	if detail == nil {
		return nil
	}
	return &rustore.Version{
		VersionID:       detail.VersionID,
		Status:          detail.Status,
		PublishType:     detail.PublishType,
		Testing:         detail.Testing,
		PublishDateTime: detail.PublishDateTime,
		PartialValue:    detail.PartialValue,
		WhatsNew:        detail.WhatsNew,
	}
}
