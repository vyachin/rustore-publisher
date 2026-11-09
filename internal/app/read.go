package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/vyachin/rustore-publisher/internal/exitcode"
	"github.com/vyachin/rustore-publisher/internal/rustore"
)

// Фазы, в которых может отказать чтение.
const (
	PhaseAuth   = "auth"
	PhaseStatus = "status"
)

// Коды предупреждений CLI.
const (
	WarningStatusUnavailable = "STATUS_UNAVAILABLE"
)

// Warning — предупреждение, не отменяющее успешное чтение.
type Warning struct {
	Code    string
	Message string
}

// Progress хранит текущую фазу команды, чтобы ошибка указывала место сбоя.
type Progress struct {
	Phase string
}

// StatusInput задаёт параметры status.
type StatusInput struct {
	Package   string
	VersionID *int64
}

// StatusReport — результат чтения состояния приложения.
type StatusReport struct {
	Selected      *rustore.Version
	LatestRelease *rustore.Version
	Draft         *rustore.Version
	Detail        *rustore.VersionDetail
	Warnings      []Warning
}

// ReadStatus читает все страницы версий и выбирает нужные по контракту.
func ReadStatus(ctx context.Context, client *rustore.Client, in StatusInput, progress *Progress) (StatusReport, error) {
	if progress != nil {
		progress.Phase = PhaseAuth
	}
	if _, err := client.Token(ctx); err != nil {
		return StatusReport{}, err
	}

	if progress != nil {
		progress.Phase = PhaseStatus
	}
	versions, err := client.ListVersions(ctx, in.Package, rustore.ListOptions{TestingType: "ALL"})
	if err != nil {
		return StatusReport{}, err
	}

	report := StatusReport{}
	if in.VersionID != nil {
		report.Selected = FindVersion(versions, *in.VersionID)
		if report.Selected == nil {
			return StatusReport{}, exitcode.WithHint(
				exitcode.Errorf(exitcode.KindVersionNotFound,
					"версия %d не найдена у приложения %s", *in.VersionID, in.Package),
				"проверьте --version-id и имя пакета",
			)
		}
	} else {
		report.LatestRelease = SelectLatestRelease(versions)
		draft, err := SelectDraft(versions)
		if err != nil {
			return StatusReport{}, err
		}
		report.Draft = draft
	}

	if report.Selected != nil {
		detail, detailErr := client.VersionDetail(ctx, in.Package, report.Selected.VersionID)
		if detailErr != nil {
			report.Warnings = append(report.Warnings, Warning{
				Code:    WarningStatusUnavailable,
				Message: fmt.Sprintf("подробности версии %d недоступны: %v", report.Selected.VersionID, detailErr),
			})
		} else {
			report.Detail = &detail
		}
	}

	return report, nil
}

// FindVersion ищет версию по идентификатору.
func FindVersion(versions []rustore.Version, versionID int64) *rustore.Version {
	for i := range versions {
		if versions[i].VersionID == versionID {
			return &versions[i]
		}
	}
	return nil
}

// SelectLatestRelease выбирает релизную версию с максимальным кодом,
// исключая черновики. При равенстве кода выбирается больший versionId.
func SelectLatestRelease(versions []rustore.Version) *rustore.Version {
	var best *rustore.Version
	for i := range versions {
		version := &versions[i]
		if !version.Testing.IsRelease() {
			continue
		}
		if version.Status == rustore.StatusDraft || version.Status == rustore.StatusDeletedDraft {
			continue
		}
		if best == nil ||
			version.VersionCode > best.VersionCode ||
			(version.VersionCode == best.VersionCode && version.VersionID > best.VersionID) {
			best = version
		}
	}
	return best
}

// SelectDraft выбирает живой черновик. Несколько черновиков противоречат
// контракту API и считаются конфликтом.
func SelectDraft(versions []rustore.Version) (*rustore.Version, error) {
	var drafts []*rustore.Version
	for i := range versions {
		if versions[i].Status == rustore.StatusDraft {
			drafts = append(drafts, &versions[i])
		}
	}
	switch len(drafts) {
	case 0:
		return nil, nil
	case 1:
		return drafts[0], nil
	default:
		ids := make([]string, 0, len(drafts))
		for _, draft := range drafts {
			ids = append(ids, fmt.Sprintf("%d", draft.VersionID))
		}
		return nil, exitcode.WithHint(
			exitcode.Errorf(exitcode.KindVersionStateConflict,
				"у приложения несколько живых черновиков: %s", strings.Join(ids, ", ")),
			"завершите или удалите лишние черновики в консоли RuStore",
		)
	}
}

// DoctorInput задаёт параметры doctor.
type DoctorInput struct {
	Package string
}

// DoctorReport — результат проверки приложения без изменений.
type DoctorReport struct {
	Versions int
	Active   *rustore.Version
	Detail   *rustore.VersionDetail
	Warnings []Warning
}

// ReadDoctor проверяет доступ к приложению и читает данные активной версии.
func ReadDoctor(ctx context.Context, client *rustore.Client, in DoctorInput, progress *Progress) (DoctorReport, error) {
	if progress != nil {
		progress.Phase = PhaseAuth
	}
	if _, err := client.Token(ctx); err != nil {
		return DoctorReport{}, err
	}

	if progress != nil {
		progress.Phase = PhaseStatus
	}
	versions, err := client.ListVersions(ctx, in.Package, rustore.ListOptions{TestingType: "ALL"})
	if err != nil {
		return DoctorReport{}, err
	}

	report := DoctorReport{
		Versions: len(versions),
		Active:   SelectActive(versions),
	}
	if report.Active != nil {
		detail, detailErr := client.VersionDetail(ctx, in.Package, report.Active.VersionID)
		if detailErr != nil {
			report.Warnings = append(report.Warnings, Warning{
				Code:    WarningStatusUnavailable,
				Message: fmt.Sprintf("подробности активной версии %d недоступны: %v", report.Active.VersionID, detailErr),
			})
		} else {
			report.Detail = &detail
		}
	}

	return report, nil
}

// SelectActive выбирает опубликованную версию с максимальным кодом.
func SelectActive(versions []rustore.Version) *rustore.Version {
	var best *rustore.Version
	for i := range versions {
		version := &versions[i]
		if version.Status != rustore.StatusActive && version.Status != rustore.StatusPartialActive {
			continue
		}
		if best == nil ||
			version.VersionCode > best.VersionCode ||
			(version.VersionCode == best.VersionCode && version.VersionID > best.VersionID) {
			best = version
		}
	}
	return best
}

// MainPackage возвращает основной пакет версии.
func MainPackage(detail *rustore.VersionDetail, versionCode int64) *rustore.Package {
	if detail == nil {
		return nil
	}
	var fallback *rustore.Package
	for i := range detail.Packages {
		pkg := &detail.Packages[i]
		if pkg.IsMainApk {
			if fallback == nil {
				fallback = pkg
			}
			if pkg.VerCode == versionCode {
				return pkg
			}
		}
	}
	return fallback
}
