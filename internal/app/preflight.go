package app

import (
	"os"

	"github.com/vyachin/rustore-publisher/internal/artifact"
	"github.com/vyachin/rustore-publisher/internal/exitcode"
)

// CredentialsEnv — переменная окружения с путём к файлу учётных данных.
const CredentialsEnv = "RUSTORE_CREDENTIALS_FILE"

// ResolveCredentialsPath выбирает путь к учётным данным: флаг важнее
// переменной окружения.
func ResolveCredentialsPath(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	return os.Getenv(CredentialsEnv)
}

// CheckIdentity сверяет явно заданные package и код версии с данными AAB.
// Нулевой код версии и пустой package означают «не задано».
func CheckIdentity(identity artifact.Identity, packageName string, versionCode int64) error {
	if versionCode < 0 {
		return exitcode.Errorf(exitcode.KindInvalidArguments,
			"--version-code должен быть положительным, получен %d", versionCode)
	}
	if packageName != "" && packageName != identity.Package {
		return exitcode.WithHint(
			exitcode.Errorf(exitcode.KindInvalidAAB,
				"package %q не совпадает с %q из AAB", packageName, identity.Package),
			"уберите --package или укажите пакет из сборки",
		)
	}
	if versionCode > 0 && versionCode != identity.VersionCode {
		return exitcode.WithHint(
			exitcode.Errorf(exitcode.KindInvalidAAB,
				"код версии %d не совпадает с %d из AAB", versionCode, identity.VersionCode),
			"уберите --version-code или проверьте, что передан нужный AAB",
		)
	}
	return nil
}

// CheckArtifactPath проверяет, что путь к AAB задан.
func CheckArtifactPath(path string) error {
	if path == "" {
		return exitcode.WithHint(
			exitcode.Errorf(exitcode.KindInvalidArguments, "не задан файл AAB"),
			"передайте --aab или -f",
		)
	}
	return nil
}
