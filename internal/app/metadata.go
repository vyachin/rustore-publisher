package app

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/vyachin/rustore-publisher/internal/exitcode"
)

// Допустимая раскладка метаданных и пределы текстов.
const (
	metadataLocale   = "ru-RU"
	changelogDirName = "changelogs"

	appNameFileName          = "title.txt"
	shortDescriptionFileName = "short_description.txt"
	fullDescriptionFileName  = "full_description.txt"

	maxChangelogRunes        = 5000
	maxAppNameRunes          = 50
	maxShortDescriptionRunes = 80
	maxFullDescriptionRunes  = 4000
)

// Listing — тексты карточки, прочитанные из каталога метаданных.
// nil означает, что файла нет и поле нужно унаследовать из активной версии.
type Listing struct {
	AppName          *string
	ShortDescription *string
	FullDescription  *string
}

// Metadata — всё, что публикация читает из каталога метаданных.
// Ignored перечисляет элементы, которые CLI не использует.
type Metadata struct {
	Changelog string
	Listing   Listing
	Ignored   []string
}

// ReadMetadata читает список изменений и тексты карточки из каталога
// метаданных. Обязательна раскладка metadata/ru-RU/changelogs/<versionCode>.txt;
// title.txt, short_description.txt и full_description.txt необязательны.
// Остальные элементы каталога не публикуются и перечисляются в Ignored.
func ReadMetadata(metadataDir string, versionCode int64) (Metadata, error) {
	if versionCode <= 0 {
		return Metadata{}, exitcode.Errorf(exitcode.KindInvalidArguments,
			"код версии должен быть положительным, получен %d", versionCode)
	}

	ignored, err := ignoredMetadataEntries(metadataDir)
	if err != nil {
		return Metadata{}, err
	}

	changelog, err := readChangelog(metadataDir, versionCode)
	if err != nil {
		return Metadata{}, err
	}

	listing, err := readListing(metadataDir)
	if err != nil {
		return Metadata{}, err
	}

	return Metadata{Changelog: changelog, Listing: listing, Ignored: ignored}, nil
}

// readChangelog читает список изменений для кода версии.
func readChangelog(metadataDir string, versionCode int64) (string, error) {
	changelogsDir := filepath.Join(metadataDir, metadataLocale, changelogDirName)
	info, err := os.Stat(changelogsDir)
	if err != nil {
		return "", exitcode.WithHint(
			exitcode.Wrap(exitcode.KindInvalidChangelog,
				fmt.Errorf("прочитать каталог %q: %w", changelogsDir, err)),
			"добавьте каталог "+metadataLocale+"/"+changelogDirName+" со списком изменений",
		)
	}
	if !info.IsDir() {
		return "", exitcode.WithHint(
			exitcode.Errorf(exitcode.KindInvalidChangelog,
				"%q не является каталогом", changelogsDir),
			"добавьте каталог "+metadataLocale+"/"+changelogDirName+" со списком изменений",
		)
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
		if errors.Is(err, fs.ErrNotExist) {
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

// readListing читает необязательные тексты карточки.
func readListing(metadataDir string) (Listing, error) {
	localeDir := filepath.Join(metadataDir, metadataLocale)

	appName, err := readOptionalText(filepath.Join(localeDir, appNameFileName),
		maxAppNameRunes, true)
	if err != nil {
		return Listing{}, err
	}
	shortDescription, err := readOptionalText(filepath.Join(localeDir, shortDescriptionFileName),
		maxShortDescriptionRunes, true)
	if err != nil {
		return Listing{}, err
	}
	fullDescription, err := readOptionalText(filepath.Join(localeDir, fullDescriptionFileName),
		maxFullDescriptionRunes, false)
	if err != nil {
		return Listing{}, err
	}

	return Listing{
		AppName:          appName,
		ShortDescription: shortDescription,
		FullDescription:  fullDescription,
	}, nil
}

// readOptionalText читает необязательный текстовый файл. Отсутствие файла
// означает «не передавать поле», а пустой или некорректный файл — ошибку:
// иначе опечатка привела бы к случайному наследованию старого текста.
func readOptionalText(path string, limit int, singleLine bool) (*string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, invalidMetadata("прочитать %q: %v", path, err)
	}
	if !utf8.Valid(data) {
		return nil, invalidMetadata("файл %q содержит некорректный UTF-8", path)
	}

	text := strings.TrimPrefix(string(data), "\uFEFF")
	text = normalizeNewlines(text)
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, invalidMetadata("файл %q пуст", path)
	}
	if singleLine && strings.Contains(text, "\n") {
		return nil, invalidMetadata("файл %q должен содержать одну строку", path)
	}
	if runes := utf8.RuneCountInString(text); runes > limit {
		return nil, invalidMetadata("файл %q содержит %d символов, допустимо не более %d",
			path, runes, limit)
	}

	return &text, nil
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
		case entry.IsDir() && name == metadataLocale:
			continue
		case !entry.IsDir() && name == ContactsFileName:
			continue
		}
		ignored = append(ignored, name)
	}

	localeDir := filepath.Join(metadataDir, metadataLocale)
	localeEntries, err := os.ReadDir(localeDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, exitcode.WithHint(
			exitcode.Wrap(exitcode.KindInvalidChangelog,
				fmt.Errorf("прочитать каталог %q: %w", localeDir, err)),
			"проверьте раскладку "+metadataLocale,
		)
	}
	for _, entry := range localeEntries {
		name := entry.Name()
		switch {
		case strings.HasPrefix(name, "."):
			continue
		case entry.IsDir() && name == changelogDirName:
			continue
		case !entry.IsDir() && isListingName(name):
			continue
		}
		ignored = append(ignored, filepath.ToSlash(filepath.Join(metadataLocale, name)))
	}

	sort.Strings(ignored)
	return ignored, nil
}

// isListingName сообщает, что файл участвует в публикации текстов карточки.
func isListingName(name string) bool {
	switch name {
	case appNameFileName, shortDescriptionFileName, fullDescriptionFileName:
		return true
	}
	return false
}

// normalizeNewlines приводит переводы строк к LF: так текст сравнивается
// с ответом API и передаётся в whatsNew и поля карточки.
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

func invalidMetadata(format string, args ...any) error {
	return exitcode.WithHint(
		exitcode.Errorf(exitcode.KindInvalidMetadata, format, args...),
		"тексты карточки задаются файлами metadata/ru-RU/title.txt, short_description.txt и full_description.txt в UTF-8",
	)
}
