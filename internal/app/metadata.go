package app

import (
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // регистрирует JPEG-декодер для image.DecodeConfig
	_ "image/png"  // регистрирует PNG-декодер для image.DecodeConfig
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/vyachin/rustore-publisher/internal/exitcode"
	"github.com/vyachin/rustore-publisher/internal/rustore"
)

// Допустимая раскладка метаданных и пределы текстов и иконки.
const (
	metadataLocale   = "ru-RU"
	changelogDirName = "changelogs"
	imagesDirName    = "images"

	appNameFileName          = "title.txt"
	shortDescriptionFileName = "short_description.txt"
	fullDescriptionFileName  = "full_description.txt"

	maxChangelogRunes        = 5000
	maxAppNameRunes          = 50
	maxShortDescriptionRunes = 80
	maxFullDescriptionRunes  = 4000

	iconSize     = 512
	maxIconBytes = 3 << 20

	minScreenshots      = 3
	maxScreenshots      = 10
	minScreenshotSide   = 320
	maxScreenshotSide   = 3840
	maxScreenshotBytes  = 5 << 20
	screenshotAspectTol = 0.02
)

// screenshotDirs сопоставляет каталоги Google Play типам скриншотов RuStore.
var screenshotDirs = []struct {
	name string
	typ  rustore.ScreenshotType
}{
	{name: "phoneScreenshots", typ: rustore.ScreenshotPhone},
	{name: "tenInchScreenshots", typ: rustore.ScreenshotTablet},
	{name: "tvScreenshots", typ: rustore.ScreenshotTV},
}

// Listing — тексты карточки, прочитанные из каталога метаданных.
// nil означает, что файла нет и поле нужно унаследовать из активной версии.
type Listing struct {
	AppName          *string
	ShortDescription *string
	FullDescription  *string
}

// Icon — проверенная иконка приложения из каталога метаданных.
type Icon struct {
	Path string
}

// Screenshot — проверенный скриншот из каталога метаданных.
type Screenshot struct {
	Path        string
	Type        rustore.ScreenshotType
	Orientation rustore.ScreenshotOrientation
	Ordinal     int
}

// Metadata — всё, что публикация читает из каталога метаданных.
// Ignored перечисляет элементы, которые CLI не использует.
type Metadata struct {
	Changelog   string
	Listing     Listing
	Icon        *Icon
	Screenshots []Screenshot
	Ignored     []string
}

// ReadMetadata читает список изменений, тексты карточки, иконку и скриншоты
// из каталога метаданных. Обязательна раскладка
// metadata/ru-RU/changelogs/<versionCode>.txt; title.txt,
// short_description.txt, full_description.txt, images/icon.png и каталоги
// images/phoneScreenshots, images/tenInchScreenshots, images/tvScreenshots
// необязательны. Остальные элементы каталога не публикуются и перечисляются
// в Ignored.
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

	icon, err := readIcon(metadataDir)
	if err != nil {
		return Metadata{}, err
	}

	screenshots, err := readScreenshots(metadataDir)
	if err != nil {
		return Metadata{}, err
	}

	return Metadata{
		Changelog:   changelog,
		Listing:     listing,
		Icon:        icon,
		Screenshots: screenshots,
		Ignored:     ignored,
	}, nil
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

// readIcon читает необязательную иконку приложения из images/icon.png
// или images/icon.jpg. Несколько файлов иконки одновременно запрещены.
func readIcon(metadataDir string) (*Icon, error) {
	imagesDir := filepath.Join(metadataDir, metadataLocale, imagesDirName)
	info, err := os.Stat(imagesDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, invalidMetadata("прочитать каталог %q: %v", imagesDir, err)
	}
	if !info.IsDir() {
		return nil, invalidMetadata("%q не является каталогом", imagesDir)
	}

	entries, err := os.ReadDir(imagesDir)
	if err != nil {
		return nil, invalidMetadata("прочитать каталог %q: %v", imagesDir, err)
	}

	var icons []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") || !isIconName(entry.Name()) {
			continue
		}
		if entry.IsDir() {
			return nil, invalidMetadata("%q не является файлом иконки",
				filepath.Join(imagesDir, entry.Name()))
		}
		icons = append(icons, filepath.Join(imagesDir, entry.Name()))
	}
	if len(icons) == 0 {
		return nil, nil
	}
	if len(icons) > 1 {
		return nil, invalidMetadata("найдено несколько иконок: %s; оставьте один файл",
			strings.Join(icons, ", "))
	}

	path := icons[0]
	if err := validateIcon(path); err != nil {
		return nil, err
	}
	return &Icon{Path: path}, nil
}

// validateIcon проверяет формат, размер файла и размеры изображения.
func validateIcon(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return invalidMetadata("прочитать %q: %v", path, err)
	}
	if !info.Mode().IsRegular() {
		return invalidMetadata("%q не является обычным файлом", path)
	}
	if info.Size() == 0 {
		return invalidMetadata("файл %q пуст", path)
	}
	if info.Size() > maxIconBytes {
		return invalidMetadata("файл %q больше 3 МБ", path)
	}

	file, err := os.Open(path)
	if err != nil {
		return invalidMetadata("открыть %q: %v", path, err)
	}
	defer func() { _ = file.Close() }()

	config, format, err := image.DecodeConfig(file)
	if err != nil {
		return invalidMetadata("файл %q не является изображением PNG или JPEG: %v", path, err)
	}
	if config.Width != iconSize || config.Height != iconSize {
		return invalidMetadata("иконка %q имеет размер %d×%d, ожидался %d×%d",
			path, config.Width, config.Height, iconSize, iconSize)
	}

	ext := strings.ToLower(filepath.Ext(path))
	switch format {
	case "png":
		if ext != ".png" {
			return invalidMetadata("иконка %q содержит PNG, но расширение %q", path, ext)
		}
	case "jpeg":
		if ext != ".jpg" && ext != ".jpeg" {
			return invalidMetadata("иконка %q содержит JPEG, но расширение %q", path, ext)
		}
	default:
		return invalidMetadata("иконка %q имеет формат %q, поддерживаются PNG и JPEG", path, format)
	}
	return nil
}

// isIconName сообщает, что файл является одним из допустимых имён иконки.
func isIconName(name string) bool {
	switch name {
	case "icon.png", "icon.jpg", "icon.jpeg":
		return true
	}
	return false
}

// readScreenshots читает поддерживаемые каталоги скриншотов. Пустой каталог
// означает «не менять»: Google Play использует пустой каталог для очистки
// своего набора, а RuStore не должен из-за этого менять свой.
func readScreenshots(metadataDir string) ([]Screenshot, error) {
	var screenshots []Screenshot
	for _, dir := range screenshotDirs {
		files, err := readScreenshotSet(metadataDir, dir.name, dir.typ)
		if err != nil {
			return nil, err
		}
		screenshots = append(screenshots, files...)
	}
	return screenshots, nil
}

// readScreenshotSet читает и проверяет один каталог скриншотов.
func readScreenshotSet(metadataDir, dirName string, typ rustore.ScreenshotType) ([]Screenshot, error) {
	dir := filepath.Join(metadataDir, metadataLocale, imagesDirName, dirName)
	info, err := os.Stat(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, invalidMetadata("прочитать каталог %q: %v", dir, err)
	}
	if !info.IsDir() {
		return nil, invalidMetadata("%q не является каталогом", dir)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, invalidMetadata("прочитать каталог %q: %v", dir, err)
	}

	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if entry.IsDir() {
			return nil, invalidMetadata("в каталоге %q найден вложенный каталог %q",
				dir, entry.Name())
		}
		paths = append(paths, filepath.Join(dir, entry.Name()))
	}
	if len(paths) == 0 {
		return nil, nil
	}
	if len(paths) < minScreenshots {
		return nil, invalidMetadata("в каталоге %q %d скриншотов, RuStore требует минимум %d (Google Play допускает 2)",
			dir, len(paths), minScreenshots)
	}
	if len(paths) > maxScreenshots {
		return nil, invalidMetadata("в каталоге %q %d скриншотов, RuStore допускает не более %d (Google Play — до 8)",
			dir, len(paths), maxScreenshots)
	}

	sort.Strings(paths)
	screenshots := make([]Screenshot, 0, len(paths))
	var orientation rustore.ScreenshotOrientation
	for ordinal, path := range paths {
		current, err := validateScreenshot(path)
		if err != nil {
			return nil, err
		}
		if ordinal == 0 {
			orientation = current
		} else if current != orientation {
			return nil, invalidMetadata("скриншоты в каталоге %q должны быть одной ориентации: %s и %s",
				dir, orientation, current)
		}
		screenshots = append(screenshots, Screenshot{
			Path:        path,
			Type:        typ,
			Orientation: orientation,
			Ordinal:     ordinal,
		})
	}
	return screenshots, nil
}

// validateScreenshot проверяет изображение и возвращает его ориентацию.
func validateScreenshot(path string) (rustore.ScreenshotOrientation, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
		return "", invalidMetadata("скриншот %q: поддерживаются .png, .jpg и .jpeg", path)
	}

	info, err := os.Stat(path)
	if err != nil {
		return "", invalidMetadata("прочитать %q: %v", path, err)
	}
	if !info.Mode().IsRegular() {
		return "", invalidMetadata("%q не является обычным файлом", path)
	}
	if info.Size() == 0 {
		return "", invalidMetadata("файл %q пуст", path)
	}
	if info.Size() > maxScreenshotBytes {
		return "", invalidMetadata("файл %q больше 5 МБ (Google Play допускает до 8 МБ)", path)
	}

	file, err := os.Open(path)
	if err != nil {
		return "", invalidMetadata("открыть %q: %v", path, err)
	}
	defer func() { _ = file.Close() }()

	config, format, err := image.DecodeConfig(file)
	if err != nil {
		return "", invalidMetadata("файл %q не является изображением PNG или JPEG: %v", path, err)
	}
	if config.Width < minScreenshotSide || config.Width > maxScreenshotSide ||
		config.Height < minScreenshotSide || config.Height > maxScreenshotSide {
		return "", invalidMetadata("скриншот %q имеет размер %d×%d, стороны должны быть от %d до %d px",
			path, config.Width, config.Height, minScreenshotSide, maxScreenshotSide)
	}

	orientation, ok := screenshotOrientation(config.Width, config.Height)
	if !ok {
		return "", invalidMetadata("скриншот %q имеет размер %d×%d, нужны пропорции 16:9 или 9:16",
			path, config.Width, config.Height)
	}

	switch format {
	case "png":
		if ext != ".png" {
			return "", invalidMetadata("скриншот %q содержит PNG, но расширение %q", path, ext)
		}
	case "jpeg":
		if ext != ".jpg" && ext != ".jpeg" {
			return "", invalidMetadata("скриншот %q содержит JPEG, но расширение %q", path, ext)
		}
	default:
		return "", invalidMetadata("скриншот %q имеет формат %q, поддерживаются PNG и JPEG", path, format)
	}
	return orientation, nil
}

// screenshotOrientation определяет ориентацию и проверяет пропорции 16:9.
func screenshotOrientation(width, height int) (rustore.ScreenshotOrientation, bool) {
	if width == height {
		return "", false
	}
	long, short := width, height
	if short > long {
		long, short = short, long
	}
	ratio := float64(long) / float64(short)
	expected := 16.0 / 9.0
	if math.Abs(ratio-expected)/expected > screenshotAspectTol {
		return "", false
	}
	if width > height {
		return rustore.OrientationLandscape, true
	}
	return rustore.OrientationPortrait, true
}

// isSupportedScreenshotDir сообщает, что каталог участвует в публикации.
func isSupportedScreenshotDir(name string) bool {
	for _, dir := range screenshotDirs {
		if dir.name == name {
			return true
		}
	}
	return false
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
		case entry.IsDir() && name == imagesDirName:
			imageEntries, err := os.ReadDir(filepath.Join(localeDir, name))
			if err != nil {
				return nil, invalidMetadata("прочитать каталог %q: %v",
					filepath.Join(localeDir, name), err)
			}
			for _, imageEntry := range imageEntries {
				imageName := imageEntry.Name()
				if strings.HasPrefix(imageName, ".") {
					continue
				}
				if !imageEntry.IsDir() && isIconName(imageName) {
					continue
				}
				if imageEntry.IsDir() && isSupportedScreenshotDir(imageName) {
					continue
				}
				ignored = append(ignored, filepath.ToSlash(
					filepath.Join(metadataLocale, name, imageName)))
			}
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
		"проверьте тексты, иконку и скриншоты в metadata/ru-RU: PNG/JPEG, иконка 512×512 до 3 МБ, скриншоты 3–10 файлов 16:9 или 9:16 до 5 МБ",
	)
}
