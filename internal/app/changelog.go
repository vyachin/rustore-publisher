package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

// ChangelogReport — список изменений версии и элементы каталога метаданных,
// которые публикация не использует.
type ChangelogReport struct {
	Text    string
	Ignored []string
}

// ReadChangelog читает список изменений для кода версии из каталога метаданных.
// Обязательна раскладка metadata/ru-RU/changelogs/<versionCode>.txt. Остальные
// элементы каталога не публикуются: они перечисляются в Ignored, чтобы один
// каталог можно было передавать и инструменту карточки другого магазина.
func ReadChangelog(metadataDir string, versionCode int64) (ChangelogReport, error) {
	if versionCode <= 0 {
		return ChangelogReport{}, exitcode.Errorf(exitcode.KindInvalidArguments,
			"код версии должен быть положительным, получен %d", versionCode)
	}

	ignored, err := ignoredMetadataEntries(metadataDir)
	if err != nil {
		return ChangelogReport{}, err
	}

	changelogsDir := filepath.Join(metadataDir, changelogLocale, changelogDirName)
	info, err := os.Stat(changelogsDir)
	if err != nil {
		return ChangelogReport{}, exitcode.WithHint(
			exitcode.Wrap(exitcode.KindInvalidChangelog,
				fmt.Errorf("прочитать каталог %q: %w", changelogsDir, err)),
			"добавьте каталог "+changelogLocale+"/"+changelogDirName+" со списком изменений",
		)
	}
	if !info.IsDir() {
		return ChangelogReport{}, exitcode.WithHint(
			exitcode.Errorf(exitcode.KindInvalidChangelog,
				"%q не является каталогом", changelogsDir),
			"добавьте каталог "+changelogLocale+"/"+changelogDirName+" со списком изменений",
		)
	}

	entries, err := os.ReadDir(changelogsDir)
	if err != nil {
		return ChangelogReport{}, invalidChangelog("прочитать каталог %q: %v", changelogsDir, err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if entry.IsDir() || !isChangelogName(entry.Name()) {
			return ChangelogReport{}, invalidChangelog("в каталоге %q найден посторонний элемент %q",
				changelogsDir, entry.Name())
		}
	}

	name := strconv.FormatInt(versionCode, 10) + ".txt"
	path := filepath.Join(changelogsDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ChangelogReport{}, exitcode.WithHint(
				exitcode.Errorf(exitcode.KindInvalidChangelog,
					"нет списка изменений для версии %d: ожидался файл %q", versionCode, path),
				"добавьте metadata/ru-RU/changelogs/<код версии>.txt для этой сборки",
			)
		}
		return ChangelogReport{}, invalidChangelog("прочитать %q: %v", path, err)
	}

	if !utf8.Valid(data) {
		return ChangelogReport{}, invalidChangelog("файл %q содержит некорректный UTF-8", path)
	}

	text := normalizeNewlines(string(data))
	if strings.TrimSpace(text) == "" {
		return ChangelogReport{}, invalidChangelog("файл %q пуст", path)
	}
	if runes := utf8.RuneCountInString(text); runes > maxChangelogRunes {
		return ChangelogReport{}, invalidChangelog("файл %q содержит %d символов, допустимо не более %d",
			path, runes, maxChangelogRunes)
	}

	return ChangelogReport{Text: text, Ignored: ignored}, nil
}

// ignoredMetadataEntries перечисляет элементы каталога метаданных, которые не
// участвуют в публикации: раскладку карточки другого магазина и служебные файлы.
// Пути выводятся через косую черту, чтобы сообщение не зависело от платформы.
func ignoredMetadataEntries(metadataDir string) ([]string, error) {
	entries, err := os.ReadDir(metadataDir)
	if err != nil {
		return nil, exitcode.WithHint(
			exitcode.Wrap(exitcode.KindInvalidChangelog,
				fmt.Errorf("прочитать каталог метаданных %q: %w", metadataDir, err)),
			"проверьте путь в --metadata-dir",
		)
	}

	ignored := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		switch {
		case strings.HasPrefix(name, "."):
			continue
		case entry.IsDir() && name == changelogLocale:
			continue
		case !entry.IsDir() && name == ContactsFileName:
			continue
		}
		ignored = append(ignored, name)
	}

	localeDir := filepath.Join(metadataDir, changelogLocale)
	localeEntries, err := os.ReadDir(localeDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, exitcode.WithHint(
			exitcode.Wrap(exitcode.KindInvalidChangelog,
				fmt.Errorf("прочитать каталог %q: %w", localeDir, err)),
			"проверьте раскладку "+changelogLocale,
		)
	}
	for _, entry := range localeEntries {
		name := entry.Name()
		switch {
		case strings.HasPrefix(name, "."):
			continue
		case entry.IsDir() && name == changelogDirName:
			continue
		}
		ignored = append(ignored, filepath.ToSlash(filepath.Join(changelogLocale, name)))
	}

	sort.Strings(ignored)
	return ignored, nil
}

// normalizeNewlines приводит переводы строк к LF: так текст сравнивается
// с ответом API и передаётся в whatsNew.
func normalizeNewlines(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n")
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
