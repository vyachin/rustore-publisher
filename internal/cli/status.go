package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	appcore "github.com/vyachin/rustore-publisher/internal/app"
	"github.com/vyachin/rustore-publisher/internal/exitcode"
)

func newStatusCmd(a *App, g *globalFlags) *cobra.Command {
	var packageName string
	var versionID int64

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Показать состояние версий приложения",
		Long: "Прочитать версии приложения в RuStore и показать последний выпуск, " +
			"живой черновик или указанную версию.",
		Example: "  rustore-publisher status -p com.example.app\n" +
			"  rustore-publisher status -p com.example.app --version-id 123456",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			result := NewResult("status")
			if packageName != "" {
				result.Package = &packageName
			}

			if packageName == "" {
				return fail(a, g, result, PhasePreflight,
					exitcode.WithHint(
						exitcode.Errorf(exitcode.KindInvalidArguments, "не задан package приложения"),
						"передайте --package или -p",
					))
			}
			if versionID < 0 {
				return fail(a, g, result, PhasePreflight,
					exitcode.Errorf(exitcode.KindInvalidArguments,
						"--version-id должен быть положительным, получен %d", versionID))
			}

			client, err := newRustoreClient(a, g)
			if err != nil {
				return fail(a, g, result, PhasePreflight, err)
			}

			input := appcore.StatusInput{Package: packageName}
			if versionID > 0 {
				input.VersionID = &versionID
			}

			progress := &appcore.Progress{}
			report, err := appcore.ReadStatus(cmd.Context(), client, input, progress)
			if err != nil {
				return failStatus(a, g, result, progress.Phase, err, nil, nil)
			}

			result.Phase = PhaseStatus
			result.Outcome = OutcomeInspected
			applyVersionResult(result, report.Selected, report.Detail)
			latest := newVersionFact(report.LatestRelease, nil)
			draft := newVersionFact(report.Draft, nil)
			result.Warnings = mapWarnings(report.Warnings)

			if g.JSON {
				return EncodeJSON(a.Stdout, StatusResult{
					Result:        *result,
					LatestRelease: latest,
					Draft:         draft,
				})
			}

			printStatus(a, result, latest, draft, report)
			printWarnings(a, result.Warnings)
			return nil
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&packageName, "package", "p", "", "имя пакета приложения (обязательно)")
	flags.Int64Var(&versionID, "version-id", 0, "идентификатор версии RuStore")

	return cmd
}

// printStatus выводит человекочитаемое состояние версий.
func printStatus(a *App, result *Result, latest, draft *VersionFact, report appcore.StatusReport) {
	pkg := ""
	if result.Package != nil {
		pkg = *result.Package
	}
	_, _ = fmt.Fprintf(a.Stdout, "Пакет: %s\n", pkg)

	if result.VersionID != nil {
		printFact(a.Stdout, "Версия", newVersionFact(report.Selected, report.Detail))
	} else {
		if latest != nil {
			printFact(a.Stdout, "Последний выпуск", latest)
		} else {
			_, _ = fmt.Fprintln(a.Stdout, "Последний выпуск: нет")
		}
		if draft != nil {
			printFact(a.Stdout, "Черновик", draft)
		} else {
			_, _ = fmt.Fprintln(a.Stdout, "Черновик: нет")
		}
	}

	if report.Detail != nil && report.Detail.StatusComment != nil && *report.Detail.StatusComment != "" {
		_, _ = fmt.Fprintf(a.Stdout, "Комментарий модерации: %s\n", *report.Detail.StatusComment)
	}
}

// printFact выводит одну версию.
func printFact(w io.Writer, title string, fact *VersionFact) {
	if fact == nil {
		_, _ = fmt.Fprintf(w, "%s: нет\n", title)
		return
	}

	_, _ = fmt.Fprintf(w, "%s: %s (id %s) — %s\n",
		title, valueOr(fact.VersionCode, "?"), valueOr(fact.VersionID, "?"),
		valueOr(fact.VersionStatus, "неизвестно"))

	details := make([]string, 0, 3)
	if fact.PublishType != nil {
		details = append(details, "публикация: "+*fact.PublishType)
	}
	if fact.PartialValue != nil {
		details = append(details, fmt.Sprintf("охват: %d%%", *fact.PartialValue))
	}
	if fact.SendDateForModer != nil && *fact.SendDateForModer != "" {
		details = append(details, "на модерации с "+*fact.SendDateForModer)
	}
	if len(details) > 0 {
		_, _ = fmt.Fprintf(w, "  %s\n", strings.Join(details, ", "))
	}
	if fact.WhatsNew != nil && *fact.WhatsNew != "" {
		_, _ = fmt.Fprintf(w, "  изменения: %s\n", *fact.WhatsNew)
	}
}

// valueOr возвращает значение указателя или запасной текст.
func valueOr[T any](value *T, fallback string) string {
	if value == nil {
		return fallback
	}
	return fmt.Sprint(*value)
}

// printWarnings выводит предупреждения в stderr.
func printWarnings(a *App, warnings []Warning) {
	for _, warning := range warnings {
		_, _ = fmt.Fprintf(a.Stderr, "предупреждение %s: %s\n", warning.Code, warning.Message)
	}
}
