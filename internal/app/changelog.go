package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/vyachin/rustore-publisher/internal/exitcode"
)

// Допустимая раскладка списка изменений и его предел.
const (
	changelogLocale   = "ru-RU"
	changelogDirName  = "changelogs"
	maxChangelogRunes = 5000
)

// ReadChangelog читает список изменений для кода версии из каталога метаданных.
// Поддерживается только раскладка metadata/ru-RU/changelogs/<versionCode>.txt.
func ReadChangelog(metadataDir string, versionCode int64) (string, error) {
	if versionCode <= 0 {
		return "", exitcode.Errorf(exitcode.KindInvalidArguments,
			"код версии должен быть положительным, получен %d", versionCode)
	}

	localeDir, err := onlyEntry(metadataDir, changelogLocale,
		"поддерживается только локаль ru-RU")
	if err != nil {
		return "", err
	}

	changelogsDir, err := onlyEntry(filepath.Join(localeDir), changelogDirName,
		"поддерживается только каталог changelogs; файлы карточки, изображений и тестировщиков не принимаются")
	if err != nil {
		return "", err
	}

	entries, err := os.ReadDir(changelogsDir)
	if err != nil {
		return "", invalidChangelog("прочитать каталог %q: %v", changelogsDir, err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if entry.IsDir() || !isChangelogName(entry.Name()) {
			return "", invalidChangelog("в каталоге %q найден посторонний элемент %q",
				changelogsDir, entry.Name())
		}
	}

	name := strconv.FormatInt(versionCode, 10) + ".txt"
	path := filepath.Join(changelogsDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", exitcode.WithHint(
				exitcode.Errorf(exitcode.KindInvalidChangelog,
					"нет списка изменений для версии %d: ожидался файл %q", versionCode, path),
				"добавьте metadata/ru-RU/changelogs/<код версии>.txt для этой сборки",
			)
		}
		return "", invalidChangelog("прочитать %q: %v", path, err)
	}

	if !utf8.Valid(data) {
		return "", invalidChangelog("файл %q содержит некорректный UTF-8", path)
	}

	text := normalizeNewlines(string(data))
	if strings.TrimSpace(text) == "" {
		return "", invalidChangelog("файл %q пуст", path)
	}
	if runes := utf8.RuneCountInString(text); runes > maxChangelogRunes {
		return "", invalidChangelog("файл %q содержит %d символов, допустимо не более %d",
			path, runes, maxChangelogRunes)
	}

	return text, nil
}

// normalizeNewlines приводит переводы строк к LF: так текст сравнивается
// с ответом API и передаётся в whatsNew.
func normalizeNewlines(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n")
}

// onlyEntry проверяет, что в каталоге есть ровно нужный подкаталог,
// и игнорирует служебные файлы, начинающиеся с точки.
func onlyEntry(dir, want, hint string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", exitcode.WithHint(
			exitcode.Wrap(exitcode.KindInvalidChangelog,
				fmt.Errorf("прочитать каталог метаданных %q: %w", dir, err)),
			"проверьте путь в --metadata-dir",
		)
	}

	found := ""
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if !entry.IsDir() || entry.Name() != want {
			return "", exitcode.WithHint(
				exitcode.Errorf(exitcode.KindInvalidChangelog,
					"в каталоге %q найден элемент %q", dir, entry.Name()),
				hint,
			)
		}
		found = filepath.Join(dir, entry.Name())
	}
	if found == "" {
		return "", exitcode.WithHint(
			exitcode.Errorf(exitcode.KindInvalidChangelog,
				"в каталоге %q нет %q", dir, want),
			hint,
		)
	}
	return found, nil
}

// isChangelogName проверяет имя файла вида <код версии>.txt.
func isChangelogName(name string) bool {
	if !strings.HasSuffix(name, ".txt") {
		return false
	}
	base := strings.TrimSuffix(name, ".txt")
	if base == "" {
		return false
	}
	for _, r := range base {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func invalidChangelog(format string, args ...any) error {
	return exitcode.WithHint(
		exitcode.Errorf(exitcode.KindInvalidChangelog, format, args...),
		"список изменений задаётся файлом metadata/ru-RU/changelogs/<код версии>.txt в UTF-8 до 5000 символов",
	)
}
