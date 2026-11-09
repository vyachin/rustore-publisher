package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/spf13/cobra"

	appcore "github.com/vyachin/rustore-publisher/internal/app"
	"github.com/vyachin/rustore-publisher/internal/artifact"
	"github.com/vyachin/rustore-publisher/internal/exitcode"
)

func newDoctorCmd(a *App, g *globalFlags) *cobra.Command {
	var packageName string
	var aabPath string

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Проверить доступ и состояние приложения",
		Long: "Проверить локальные данные, авторизацию и доступ к приложению без " +
			"создания черновика или загрузки файла.",
		Example: "  rustore-publisher doctor -p com.example.app\n" +
			"  rustore-publisher doctor -p com.example.app -f app-release.aab",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			result := NewResult("doctor")
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

			bundle, err := openBundleForDoctor(aabPath, packageName)
			if err != nil {
				return fail(a, g, result, PhasePreflight, err)
			}
			if bundle != nil {
				defer func() { _ = bundle.Close() }()
			}

			client, err := newRustoreClient(a, g)
			if err != nil {
				return fail(a, g, result, PhasePreflight, err)
			}

			progress := &appcore.Progress{}
			report, err := appcore.ReadDoctor(cmd.Context(), client,
				appcore.DoctorInput{Package: packageName}, progress)
			if err != nil {
				return fail(a, g, result, progress.Phase, err)
			}

			result.Phase = PhaseStatus
			result.Outcome = OutcomeInspected
			applyVersionResult(result, report.Active, report.Detail)
			result.Warnings = mapWarnings(report.Warnings)

			if g.JSON {
				return EncodeJSON(a.Stdout, result)
			}

			printDoctor(a, packageName, aabPath, bundle, report)
			printWarnings(a, result.Warnings)
			return nil
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&packageName, "package", "p", "", "имя пакета приложения (обязательно)")
	flags.StringVarP(&aabPath, "aab", "f", "", "путь к AAB для локальных проверок")

	return cmd
}

// openBundleForDoctor проверяет AAB, если путь задан.
func openBundleForDoctor(path, packageName string) (*artifact.Bundle, error) {
	if path == "" {
		return nil, nil
	}
	bundle, err := artifact.Open(path)
	if err != nil {
		return nil, err
	}
	if err := appcore.CheckIdentity(bundle.Identity, packageName, 0); err != nil {
		_ = bundle.Close()
		return nil, err
	}
	return bundle, nil
}

// printDoctor выводит результат проверки.
func printDoctor(a *App, packageName, aabPath string, bundle *artifact.Bundle, report appcore.DoctorReport) {
	_, _ = fmt.Fprintf(a.Stdout, "Пакет: %s\n", packageName)
	_, _ = fmt.Fprintf(a.Stdout, "Доступ: подтверждён, прочитано версий: %d\n", report.Versions)

	if report.Active == nil {
		_, _ = fmt.Fprintln(a.Stdout, "Активная версия: не найдена")
	} else {
		printFact(a.Stdout, "Активная версия",
			newVersionFact(report.Active, report.Detail))
		if report.Detail != nil {
			_, _ = fmt.Fprintf(a.Stdout, "Источник разрешений и SDK: подробности версии %d\n",
				report.Active.VersionID)
			printDiagnostics(a.Stdout,
				report.Detail.Permissions,
				mainPackageSDKRaw(report.Detail, report.Active.VersionCode))
		}
	}

	if bundle != nil {
		_, _ = fmt.Fprintf(a.Stdout, "AAB: %s, пакет %s, код версии %d\n",
			aabPath, bundle.Identity.Package, bundle.Identity.VersionCode)
		if report.Active != nil && bundle.Identity.VersionCode <= report.Active.VersionCode {
			_, _ = fmt.Fprintf(a.Stdout,
				"  код версии AAB не больше активного (%d); новая публикация потребует большего кода\n",
				report.Active.VersionCode)
		}
	}
}

// printDiagnostics выводит разрешения и автоопределённые SDK.
func printDiagnostics(w io.Writer, permissionsRaw, sdkRaw json.RawMessage) {
	permissions := permissionNames(permissionsRaw)
	if len(permissions) == 0 {
		_, _ = fmt.Fprintln(w, "Разрешения: нет данных")
	} else {
		_, _ = fmt.Fprintf(w, "Разрешения (%d): %s\n", len(permissions), joinLimit(permissions, 8))
	}

	sdks := sdkNames(sdkRaw)
	if len(sdks) == 0 {
		_, _ = fmt.Fprintln(w, "SDK: нет данных")
	} else {
		_, _ = fmt.Fprintf(w, "SDK (%d): %s\n", len(sdks), joinLimit(sdks, 8))
	}
}

// permissionNames собирает имена разрешений из ответа API.
func permissionNames(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var grouped map[string][]struct {
		Permission string `json:"permission"`
	}
	if err := json.Unmarshal(raw, &grouped); err != nil {
		return nil
	}
	var names []string
	for _, items := range grouped {
		for _, item := range items {
			if item.Permission != "" {
				names = append(names, item.Permission)
			}
		}
	}
	sort.Strings(names)
	return names
}

// sdkNames собирает названия SDK из ответа API.
func sdkNames(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var grouped map[string][]string
	if err := json.Unmarshal(raw, &grouped); err != nil {
		return nil
	}
	var names []string
	for category, items := range grouped {
		for _, item := range items {
			names = append(names, category+": "+item)
		}
	}
	sort.Strings(names)
	return names
}

// joinLimit объединяет значения и ограничивает вывод.
func joinLimit(values []string, limit int) string {
	if len(values) <= limit {
		return joinComma(values)
	}
	return joinComma(values[:limit]) + fmt.Sprintf(" и ещё %d", len(values)-limit)
}

func joinComma(values []string) string {
	result := ""
	for i, value := range values {
		if i > 0 {
			result += ", "
		}
		result += value
	}
	return result
}
