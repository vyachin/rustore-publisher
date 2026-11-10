package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	appcore "github.com/vyachin/rustore-publisher/internal/app"
	"github.com/vyachin/rustore-publisher/internal/artifact"
	"github.com/vyachin/rustore-publisher/internal/exitcode"
	"github.com/vyachin/rustore-publisher/internal/rustore"
)

func newPublishCmd(a *App, g *globalFlags) *cobra.Command {
	var (
		packageName       string
		aabPath           string
		metadataDir       string
		contactsFile      string
		versionCode       int64
		publishType       string
		ifExists          string
		processingTimeout time.Duration
		dryRun            bool
	)

	cmd := &cobra.Command{
		Use:   "publish",
		Short: "Загрузить AAB и отправить его на модерацию",
		Long: "Проверить AAB, список изменений, тексты карточки и иконку, создать черновик, " +
			"загрузить файл и отправить версию на модерацию. Публикация после " +
			"одобрения выполняется в консоли RuStore для ручного режима.",
		Example: "  rustore-publisher publish -f app-release.aab --metadata-dir metadata\n" +
			"  rustore-publisher publish -f app-release.aab --publish-type instantly",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			result := NewResult("publish")
			if packageName != "" {
				result.Package = &packageName
			}
			// Значение по умолчанию подставляется здесь, чтобы справка
			// не показывала английское "(default 2m0s)".
			if !cmd.Flags().Changed("processing-timeout") {
				processingTimeout = 2 * time.Minute
			}

			publishTypeValue, err := normalizePublishType(publishType)
			if err != nil {
				return failPublish(a, g, result, PhasePreflight, err, appcore.PublishReport{})
			}
			if ifExists != "fail" && ifExists != "skip" {
				return failPublish(a, g, result, PhasePreflight,
					exitcode.Errorf(exitcode.KindInvalidArguments,
						"--if-exists принимает fail или skip, получено %q", ifExists),
					appcore.PublishReport{})
			}
			if processingTimeout <= 0 {
				return failPublish(a, g, result, PhasePreflight,
					exitcode.Errorf(exitcode.KindInvalidArguments,
						"--processing-timeout должен быть положительным, получен %s", processingTimeout),
					appcore.PublishReport{})
			}
			if err := appcore.CheckArtifactPath(aabPath); err != nil {
				return failPublish(a, g, result, PhasePreflight, err, appcore.PublishReport{})
			}

			bundle, err := artifact.Open(aabPath)
			if err != nil {
				return failPublish(a, g, result, PhasePreflight, err, appcore.PublishReport{})
			}
			defer func() { _ = bundle.Close() }()

			result.VersionCode = int64Ptr(bundle.Identity.VersionCode)
			if err := appcore.CheckIdentity(bundle.Identity, packageName, versionCode); err != nil {
				return failPublish(a, g, result, PhasePreflight, err, appcore.PublishReport{})
			}
			if packageName == "" {
				packageName = bundle.Identity.Package
				result.Package = &packageName
			}

			var changelog *string
			var listing appcore.Listing
			var icon *appcore.Icon
			if metadataDir != "" {
				metadata, err := appcore.ReadMetadata(metadataDir, bundle.Identity.VersionCode)
				if err != nil {
					return failPublish(a, g, result, PhasePreflight, err, appcore.PublishReport{})
				}
				changelog = &metadata.Changelog
				listing = metadata.Listing
				icon = metadata.Icon
				if len(metadata.Ignored) > 0 {
					result.Warnings = append(result.Warnings, Warning{
						Code: appcore.WarningMetadataIgnored,
						Message: fmt.Sprintf("в каталоге метаданных не публикуются: %s",
							strings.Join(metadata.Ignored, ", ")),
					})
				}
			}

			contacts, err := readContacts(contactsFile, metadataDir)
			if err != nil {
				return failPublish(a, g, result, PhasePreflight, err, appcore.PublishReport{})
			}

			client, err := newRustoreClient(a, g)
			if err != nil {
				return failPublish(a, g, result, PhasePreflight, err, appcore.PublishReport{})
			}

			progress := &appcore.Progress{}
			report, err := appcore.Publish(cmd.Context(), client, appcore.PublishInput{
				Package:           packageName,
				Bundle:            bundle,
				Changelog:         changelog,
				Listing:           listing,
				Icon:              icon,
				Contacts:          contacts,
				PublishType:       publishTypeValue,
				IfExists:          ifExists,
				DryRun:            dryRun,
				ProcessingTimeout: processingTimeout,
				Now:               a.Now,
			}, progress)
			if err != nil {
				return failPublish(a, g, result, progress.Phase, err, report)
			}

			result.Phase = progress.Phase
			if result.Phase == "" {
				result.Phase = PhaseStatus
			}
			result.Outcome = report.Outcome
			result.Skipped = report.Skipped
			applyPublishReport(result, report)
			result.Warnings = append(result.Warnings, mapWarnings(report.Warnings)...)

			if g.JSON {
				return EncodeJSON(a.Stdout, result)
			}

			printPublish(a, result, publishTypeValue)
			printWarnings(a, result.Warnings)
			return nil
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&packageName, "package", "p", "", "имя пакета приложения; проверяется по AAB")
	flags.StringVarP(&aabPath, "aab", "f", "", "путь к AAB (обязательно)")
	flags.StringVar(&metadataDir, "metadata-dir", "",
		"каталог metadata со списком изменений, текстами карточки и иконкой")
	flags.StringVar(&contactsFile, "contacts-file", "",
		"файл контактов разработчика; по умолчанию metadata/"+appcore.ContactsFileName)
	flags.Int64Var(&versionCode, "version-code", 0, "ожидаемый код версии; проверяется по AAB")
	flags.StringVar(&publishType, "publish-type", "manual", "режим публикации: manual или instantly")
	flags.StringVar(&ifExists, "if-exists", "fail", "при уже отправленной версии: fail или skip")
	flags.DurationVar(&processingTimeout, "processing-timeout", 0,
		"бюджет ожидания обработки AAB, по умолчанию 2m")
	flags.BoolVar(&dryRun, "dry-run", false, "только проверить данные и состояние, без записи")

	return cmd
}

// readContacts выбирает файл контактов: явный флаг важнее файла в метаданных.
func readContacts(contactsFile, metadataDir string) (*rustore.DeveloperContacts, error) {
	path := contactsFile
	if path == "" && metadataDir != "" {
		candidate := filepath.Join(metadataDir, appcore.ContactsFileName)
		if _, err := os.Stat(candidate); err == nil {
			path = candidate
		}
	}
	if path == "" {
		return nil, nil
	}

	contacts, err := appcore.ReadDeveloperContacts(path)
	if err != nil {
		return nil, err
	}
	return &contacts, nil
}

// normalizePublishType переводит режим CLI в значение API.
func normalizePublishType(value string) (string, error) {
	switch strings.ToLower(value) {
	case "manual":
		return "MANUAL", nil
	case "instantly":
		return "INSTANTLY", nil
	default:
		return "", exitcode.Errorf(exitcode.KindInvalidArguments,
			"--publish-type принимает manual или instantly, получено %q", value)
	}
}

// applyPublishReport переносит факты публикации в результат.
func applyPublishReport(result *Result, report appcore.PublishReport) {
	if report.VersionID > 0 {
		result.VersionID = int64Ptr(report.VersionID)
	}
	if report.Version != nil {
		applyVersionResult(result, report.Version, report.Detail)
	}
	if report.Upload != nil {
		result.Permissions = decodeRaw(report.Upload.Permissions)
		result.AutoDetectedSDKs = decodeRaw(report.Upload.AutoDetectedSDKs)
	}
}

// failPublish печатает ошибку с накопленными фактами публикации.
func failPublish(a *App, g *globalFlags, result *Result, phase string, err error, report appcore.PublishReport) error {
	applyPublishReport(result, report)
	result.Phase = phase
	result.Outcome = OutcomeFailed
	result.Error = NewErrorDetail(err)
	if exitcode.KindOf(err) == exitcode.KindOutcomeUnknown || report.OutcomeUnknown {
		result.Outcome = OutcomeUnknown
		result.OutcomeUnknown = true
	}
	if g.JSON {
		if writeErr := EncodeJSON(a.Stdout, result); writeErr != nil {
			return writeErr
		}
		return &renderedError{err: err}
	}
	return err
}

// printPublish выводит итог публикации.
func printPublish(a *App, result *Result, publishType string) {
	version := valueOr(result.VersionID, "?")
	code := valueOr(result.VersionCode, "?")

	switch result.Outcome {
	case appcore.OutcomeSkipped:
		status := valueOr(result.VersionStatus, "неизвестно")
		_, _ = fmt.Fprintf(a.Stdout,
			"Версия %s (id %s) уже отправлена, статус %s; действие пропущено.\n", code, version, status)
	case appcore.OutcomeDryRun:
		_, _ = fmt.Fprintf(a.Stdout,
			"Проверка пройдена: версию с кодом %s можно отправить на модерацию. Изменения не вносились.\n", code)
	default:
		status := valueOr(result.VersionStatus, "неизвестно")
		_, _ = fmt.Fprintf(a.Stdout,
			"Версия %s (id %s) отправлена на модерацию, текущий статус: %s.\n", code, version, status)
		if publishType == "MANUAL" {
			_, _ = fmt.Fprintln(a.Stdout,
				"После одобрения опубликуйте версию вручную в консоли RuStore.")
		} else {
			_, _ = fmt.Fprintln(a.Stdout,
				"После одобрения версия будет опубликована автоматически.")
		}
	}
}
