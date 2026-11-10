package app_test

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vyachin/rustore-publisher/internal/app"
	"github.com/vyachin/rustore-publisher/internal/exitcode"
	"github.com/vyachin/rustore-publisher/internal/rustore"
)

func writeMetadata(t *testing.T, files map[string]string) string {
	t.Helper()

	root := t.TempDir()
	for name, body := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return root
}

func writeImage(t *testing.T, path string, width, height int, format string) {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	var buf bytes.Buffer
	var err error
	switch format {
	case "png":
		err = png.Encode(&buf, img)
	case "jpeg":
		err = jpeg.Encode(&buf, img, nil)
	default:
		t.Fatalf("неизвестный формат %q", format)
	}
	if err != nil {
		t.Fatalf("encode %s: %v", format, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestReadMetadata(t *testing.T) {
	t.Parallel()

	root := writeMetadata(t, map[string]string{
		"ru-RU/changelogs/42.txt": "Исправлены ошибки.\nУскорен запуск.\n",
	})

	report, err := app.ReadMetadata(root, 42)
	if err != nil {
		t.Fatalf("ReadMetadata: %v", err)
	}
	if report.Changelog != "Исправлены ошибки.\nУскорен запуск.\n" {
		t.Errorf("text = %q", report.Changelog)
	}
	if len(report.Ignored) != 0 {
		t.Errorf("ignored = %v, want пусто", report.Ignored)
	}
}

func TestReadMetadataNormalizesNewlines(t *testing.T) {
	t.Parallel()

	root := writeMetadata(t, map[string]string{
		"ru-RU/changelogs/7.txt": "Первая строка\r\nВторая строка\rТретья",
	})

	report, err := app.ReadMetadata(root, 7)
	if err != nil {
		t.Fatalf("ReadMetadata: %v", err)
	}
	if report.Changelog != "Первая строка\nВторая строка\nТретья" {
		t.Errorf("text = %q", report.Changelog)
	}
}

func TestReadMetadataIgnoresDotFiles(t *testing.T) {
	t.Parallel()

	root := writeMetadata(t, map[string]string{
		".DS_Store":               "служебный",
		"ru-RU/.DS_Store":         "служебный",
		"ru-RU/changelogs/.keep":  "",
		"ru-RU/changelogs/42.txt": "Изменения",
	})

	report, err := app.ReadMetadata(root, 42)
	if err != nil {
		t.Fatalf("ReadMetadata: %v", err)
	}
	if len(report.Ignored) != 0 {
		t.Errorf("ignored = %v, want пусто", report.Ignored)
	}
}

func TestReadMetadataAcceptsPlayListing(t *testing.T) {
	t.Parallel()

	root := writeMetadata(t, map[string]string{
		"README.md":                       "Как устроены метаданные",
		"developer-contacts.json":         `{"email":"dev@example.com"}`,
		"ru-RU/title.txt":                 "Приложение",
		"ru-RU/short_description.txt":     "Короткое описание",
		"ru-RU/full_description.txt":      "Полное описание",
		"ru-RU/images/featureGraphic.png": "картинка",
		"ru-RU/changelogs/42.txt":         "Изменения версии 42",
		"ru-RU/changelogs/41.txt":         "Изменения версии 41",
		"ru-RU/changelogs/.DS_Store":      "служебный",
		"ru-RU/changelogs/.gitkeep":       "",
		".circleci/config.yml":            "служебный",
	})

	report, err := app.ReadMetadata(root, 42)
	if err != nil {
		t.Fatalf("ReadMetadata: %v", err)
	}
	if report.Changelog != "Изменения версии 42" {
		t.Errorf("text = %q", report.Changelog)
	}
	if report.Listing.AppName == nil || *report.Listing.AppName != "Приложение" {
		t.Errorf("appName = %v", report.Listing.AppName)
	}
	if report.Listing.ShortDescription == nil || *report.Listing.ShortDescription != "Короткое описание" {
		t.Errorf("shortDescription = %v", report.Listing.ShortDescription)
	}
	if report.Listing.FullDescription == nil || *report.Listing.FullDescription != "Полное описание" {
		t.Errorf("fullDescription = %v", report.Listing.FullDescription)
	}
	want := []string{
		"README.md",
		"ru-RU/images/featureGraphic.png",
	}
	if !slices.Equal(report.Ignored, want) {
		t.Errorf("ignored = %v, want %v", report.Ignored, want)
	}
}

func TestReadMetadataAcceptsFiveThousandRunes(t *testing.T) {
	t.Parallel()

	text := strings.Repeat("я", 5000)
	root := writeMetadata(t, map[string]string{"ru-RU/changelogs/42.txt": text})

	report, err := app.ReadMetadata(root, 42)
	if err != nil {
		t.Fatalf("ReadMetadata: %v", err)
	}
	if len([]rune(report.Changelog)) != 5000 {
		t.Errorf("длина = %d", len([]rune(report.Changelog)))
	}
}

func TestReadMetadataErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		metadata map[string]string
		code     int64
		want     string
		kind     exitcode.Kind
	}{
		{
			name:     "нет файла версии",
			metadata: map[string]string{"ru-RU/changelogs/41.txt": "старое"},
			code:     42,
			want:     "нет списка изменений",
			kind:     exitcode.KindInvalidChangelog,
		},
		{
			name:     "пустой файл",
			metadata: map[string]string{"ru-RU/changelogs/42.txt": ""},
			code:     42,
			want:     "пуст",
			kind:     exitcode.KindInvalidChangelog,
		},
		{
			name:     "только пробелы",
			metadata: map[string]string{"ru-RU/changelogs/42.txt": "  \n\t"},
			code:     42,
			want:     "пуст",
			kind:     exitcode.KindInvalidChangelog,
		},
		{
			name:     "нет локали ru-RU",
			metadata: map[string]string{"en-US/changelogs/42.txt": "changes"},
			code:     42,
			want:     "ru-RU",
			kind:     exitcode.KindInvalidChangelog,
		},
		{
			name: "посторонний файл в changelogs",
			metadata: map[string]string{
				"ru-RU/changelogs/42.txt":      "изменения",
				"ru-RU/changelogs/testers.txt": "тестировщики",
			},
			code: 42,
			want: "посторонний элемент",
			kind: exitcode.KindInvalidChangelog,
		},
		{
			name:     "слишком длинный",
			metadata: map[string]string{"ru-RU/changelogs/42.txt": strings.Repeat("я", 5001)},
			code:     42,
			want:     "5000",
			kind:     exitcode.KindInvalidChangelog,
		},
		{
			name:     "не UTF-8",
			metadata: map[string]string{"ru-RU/changelogs/42.txt": "\xff\xfe"},
			code:     42,
			want:     "UTF-8",
			kind:     exitcode.KindInvalidChangelog,
		},
		{
			name:     "неположительный код",
			metadata: map[string]string{"ru-RU/changelogs/42.txt": "изменения"},
			code:     0,
			want:     "положительным",
			kind:     exitcode.KindInvalidArguments,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root := writeMetadata(t, test.metadata)
			_, err := app.ReadMetadata(root, test.code)
			if err == nil {
				t.Fatal("ожидалась ошибка")
			}
			if kind := exitcode.KindOf(err); kind != test.kind {
				t.Errorf("KindOf = %q, want %q", kind, test.kind)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("err = %q, want %q", err, test.want)
			}
		})
	}
}

func TestReadMetadataMissingDirectory(t *testing.T) {
	t.Parallel()

	_, err := app.ReadMetadata(filepath.Join(t.TempDir(), "missing"), 42)
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
	if kind := exitcode.KindOf(err); kind != exitcode.KindInvalidChangelog {
		t.Errorf("KindOf = %q", kind)
	}
}

func TestReadMetadataAllowsContactsFile(t *testing.T) {
	t.Parallel()

	root := writeMetadata(t, map[string]string{
		"developer-contacts.json": `{"email":"dev@example.com"}`,
		"ru-RU/changelogs/42.txt": "Изменения",
	})

	report, err := app.ReadMetadata(root, 42)
	if err != nil {
		t.Fatalf("ReadMetadata: %v", err)
	}
	if report.Changelog != "Изменения" {
		t.Errorf("text = %q", report.Changelog)
	}
	if len(report.Ignored) != 0 {
		t.Errorf("ignored = %v, want пусто", report.Ignored)
	}
}

func TestReadMetadataIgnoresUnknownMetadataFile(t *testing.T) {
	t.Parallel()

	root := writeMetadata(t, map[string]string{
		"notes.txt":               "посторонний",
		"ru-RU/changelogs/42.txt": "Изменения",
	})

	report, err := app.ReadMetadata(root, 42)
	if err != nil {
		t.Fatalf("ReadMetadata: %v", err)
	}
	if !slices.Equal(report.Ignored, []string{"notes.txt"}) {
		t.Errorf("ignored = %v, want [notes.txt]", report.Ignored)
	}
}

func TestReadMetadataWithoutListingInherits(t *testing.T) {
	t.Parallel()

	root := writeMetadata(t, map[string]string{
		"ru-RU/changelogs/42.txt": "Изменения",
	})

	report, err := app.ReadMetadata(root, 42)
	if err != nil {
		t.Fatalf("ReadMetadata: %v", err)
	}
	if report.Listing.AppName != nil || report.Listing.ShortDescription != nil ||
		report.Listing.FullDescription != nil {
		t.Errorf("listing = %+v, want пустые поля", report.Listing)
	}
	if len(report.Ignored) != 0 {
		t.Errorf("ignored = %v, want пусто", report.Ignored)
	}
}

func TestReadMetadataListingNormalizes(t *testing.T) {
	t.Parallel()

	root := writeMetadata(t, map[string]string{
		"ru-RU/changelogs/42.txt":     "Изменения",
		"ru-RU/title.txt":             "\uFEFF  Приложение \r\n",
		"ru-RU/short_description.txt": "Короткое описание\r\n",
		"ru-RU/full_description.txt":  "Первая строка\r\nВторая строка\n",
	})

	report, err := app.ReadMetadata(root, 42)
	if err != nil {
		t.Fatalf("ReadMetadata: %v", err)
	}
	if report.Listing.AppName == nil || *report.Listing.AppName != "Приложение" {
		t.Errorf("appName = %v", report.Listing.AppName)
	}
	if report.Listing.ShortDescription == nil || *report.Listing.ShortDescription != "Короткое описание" {
		t.Errorf("shortDescription = %v", report.Listing.ShortDescription)
	}
	if report.Listing.FullDescription == nil ||
		*report.Listing.FullDescription != "Первая строка\nВторая строка" {
		t.Errorf("fullDescription = %v", report.Listing.FullDescription)
	}
}

func TestReadMetadataListingLimits(t *testing.T) {
	t.Parallel()

	root := writeMetadata(t, map[string]string{
		"ru-RU/changelogs/42.txt":     "Изменения",
		"ru-RU/title.txt":             strings.Repeat("я", 50),
		"ru-RU/short_description.txt": strings.Repeat("я", 80),
		"ru-RU/full_description.txt":  strings.Repeat("я", 4000),
	})

	report, err := app.ReadMetadata(root, 42)
	if err != nil {
		t.Fatalf("ReadMetadata: %v", err)
	}
	if report.Listing.AppName == nil || len([]rune(*report.Listing.AppName)) != 50 {
		t.Errorf("appName = %v", report.Listing.AppName)
	}
	if report.Listing.ShortDescription == nil || len([]rune(*report.Listing.ShortDescription)) != 80 {
		t.Errorf("shortDescription = %v", report.Listing.ShortDescription)
	}
	if report.Listing.FullDescription == nil || len([]rune(*report.Listing.FullDescription)) != 4000 {
		t.Errorf("fullDescription = %v", report.Listing.FullDescription)
	}
}

func TestReadMetadataListingErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		file string
		body string
		want string
	}{
		{name: "пустой title", file: "ru-RU/title.txt", body: "", want: "пуст"},
		{name: "пробельный short", file: "ru-RU/short_description.txt", body: "  \n\t", want: "пуст"},
		{name: "длинный title", file: "ru-RU/title.txt", body: strings.Repeat("я", 51), want: "50"},
		{name: "длинный short", file: "ru-RU/short_description.txt", body: strings.Repeat("я", 81), want: "80"},
		{name: "длинный full", file: "ru-RU/full_description.txt", body: strings.Repeat("я", 4001), want: "4000"},
		{name: "не UTF-8", file: "ru-RU/title.txt", body: "\xff\xfe", want: "UTF-8"},
		{name: "title в две строки", file: "ru-RU/title.txt", body: "Первая\nВторая", want: "одну строку"},
		{name: "short в две строки", file: "ru-RU/short_description.txt", body: "Первая\nВторая", want: "одну строку"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root := writeMetadata(t, map[string]string{
				"ru-RU/changelogs/42.txt": "Изменения",
				test.file:                 test.body,
			})
			_, err := app.ReadMetadata(root, 42)
			if err == nil {
				t.Fatal("ожидалась ошибка")
			}
			if kind := exitcode.KindOf(err); kind != exitcode.KindInvalidMetadata {
				t.Errorf("KindOf = %q, want %q", kind, exitcode.KindInvalidMetadata)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("err = %q, want %q", err, test.want)
			}
		})
	}
}

func TestReadMetadataListingDirectory(t *testing.T) {
	t.Parallel()

	root := writeMetadata(t, map[string]string{
		"ru-RU/changelogs/42.txt": "Изменения",
	})
	if err := os.Mkdir(filepath.Join(root, "ru-RU", "title.txt"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	_, err := app.ReadMetadata(root, 42)
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
	if kind := exitcode.KindOf(err); kind != exitcode.KindInvalidMetadata {
		t.Errorf("KindOf = %q", kind)
	}
}

func TestReadMetadataIgnoresOtherLocaleListing(t *testing.T) {
	t.Parallel()

	root := writeMetadata(t, map[string]string{
		"en-US/title.txt":            "Application",
		"en-US/full_description.txt": "Description",
		"ru-RU/changelogs/42.txt":    "Изменения",
	})

	report, err := app.ReadMetadata(root, 42)
	if err != nil {
		t.Fatalf("ReadMetadata: %v", err)
	}
	if report.Listing.AppName != nil || report.Listing.FullDescription != nil {
		t.Errorf("listing = %+v, want пустые поля", report.Listing)
	}
	if !slices.Equal(report.Ignored, []string{"en-US"}) {
		t.Errorf("ignored = %v, want [en-US]", report.Ignored)
	}
}

func TestReadMetadataPartialListing(t *testing.T) {
	t.Parallel()

	root := writeMetadata(t, map[string]string{
		"ru-RU/changelogs/42.txt":    "Изменения",
		"ru-RU/full_description.txt": "Только полное описание",
	})

	report, err := app.ReadMetadata(root, 42)
	if err != nil {
		t.Fatalf("ReadMetadata: %v", err)
	}
	if report.Listing.AppName != nil || report.Listing.ShortDescription != nil {
		t.Errorf("listing = %+v, want только fullDescription", report.Listing)
	}
	if report.Listing.FullDescription == nil ||
		*report.Listing.FullDescription != "Только полное описание" {
		t.Errorf("fullDescription = %v", report.Listing.FullDescription)
	}
}

func TestReadMetadataIcon(t *testing.T) {
	t.Parallel()

	root := writeMetadata(t, map[string]string{
		"ru-RU/changelogs/42.txt":                 "Изменения",
		"ru-RU/images/featureGraphic.png":         "не иконка",
		"ru-RU/images/sevenInchScreenshots/1.png": "скриншот",
	})
	writeImage(t, filepath.Join(root, "ru-RU", "images", "icon.png"), 512, 512, "png")

	report, err := app.ReadMetadata(root, 42)
	if err != nil {
		t.Fatalf("ReadMetadata: %v", err)
	}
	if report.Icon == nil || filepath.Base(report.Icon.Path) != "icon.png" {
		t.Errorf("icon = %+v", report.Icon)
	}
	want := []string{
		"ru-RU/images/featureGraphic.png",
		"ru-RU/images/sevenInchScreenshots",
	}
	if !slices.Equal(report.Ignored, want) {
		t.Errorf("ignored = %v, want %v", report.Ignored, want)
	}
}

func TestReadMetadataJPEGIcon(t *testing.T) {
	t.Parallel()

	root := writeMetadata(t, map[string]string{
		"ru-RU/changelogs/42.txt": "Изменения",
	})
	writeImage(t, filepath.Join(root, "ru-RU", "images", "icon.jpg"), 512, 512, "jpeg")

	report, err := app.ReadMetadata(root, 42)
	if err != nil {
		t.Fatalf("ReadMetadata: %v", err)
	}
	if report.Icon == nil || filepath.Base(report.Icon.Path) != "icon.jpg" {
		t.Errorf("icon = %+v", report.Icon)
	}
}

func TestReadMetadataIconErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(t *testing.T, root string)
		want  string
	}{
		{
			name: "пустой файл",
			setup: func(t *testing.T, root string) {
				writeMetadataFile(t, root, "ru-RU/images/icon.png", "")
			},
			want: "пуст",
		},
		{
			name: "неверный размер",
			setup: func(t *testing.T, root string) {
				writeImage(t, filepath.Join(root, "ru-RU", "images", "icon.png"), 511, 512, "png")
			},
			want: "512",
		},
		{
			name: "больше 3 МБ",
			setup: func(t *testing.T, root string) {
				writeMetadataFile(t, root, "ru-RU/images/icon.png", strings.Repeat("x", 3<<20+1))
			},
			want: "3 МБ",
		},
		{
			name: "несколько иконок",
			setup: func(t *testing.T, root string) {
				writeImage(t, filepath.Join(root, "ru-RU", "images", "icon.png"), 512, 512, "png")
				writeImage(t, filepath.Join(root, "ru-RU", "images", "icon.jpg"), 512, 512, "jpeg")
			},
			want: "несколько иконок",
		},
		{
			name: "расширение не совпадает",
			setup: func(t *testing.T, root string) {
				path := filepath.Join(root, "ru-RU", "images", "icon.png")
				writeImage(t, path, 512, 512, "png")
				if err := os.Rename(path, filepath.Join(root, "ru-RU", "images", "icon.jpg")); err != nil {
					t.Fatalf("rename: %v", err)
				}
			},
			want: "PNG",
		},
		{
			name: "images не каталог",
			setup: func(t *testing.T, root string) {
				writeMetadataFile(t, root, "ru-RU/images", "файл")
			},
			want: "не является каталогом",
		},
		{
			name: "иконка — каталог",
			setup: func(t *testing.T, root string) {
				if err := os.MkdirAll(filepath.Join(root, "ru-RU", "images", "icon.png"), 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
			},
			want: "не является файлом",
		},
		{
			name: "не изображение",
			setup: func(t *testing.T, root string) {
				writeMetadataFile(t, root, "ru-RU/images/icon.png", "не картинка")
			},
			want: "PNG или JPEG",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root := writeMetadata(t, map[string]string{
				"ru-RU/changelogs/42.txt": "Изменения",
			})
			test.setup(t, root)

			_, err := app.ReadMetadata(root, 42)
			if err == nil {
				t.Fatal("ожидалась ошибка")
			}
			if kind := exitcode.KindOf(err); kind != exitcode.KindInvalidMetadata {
				t.Errorf("KindOf = %q, want %q", kind, exitcode.KindInvalidMetadata)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("err = %q, want %q", err, test.want)
			}
		})
	}
}

func writeMetadataFile(t *testing.T, root, name, body string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestReadMetadataScreenshots(t *testing.T) {
	t.Parallel()

	root := writeMetadata(t, map[string]string{
		"ru-RU/changelogs/42.txt":         "Изменения",
		"ru-RU/images/featureGraphic.png": "не скриншот",
	})
	phone := filepath.Join(root, "ru-RU", "images", "phoneScreenshots")
	tablet := filepath.Join(root, "ru-RU", "images", "tenInchScreenshots")
	tv := filepath.Join(root, "ru-RU", "images", "tvScreenshots")
	for i := 1; i <= 3; i++ {
		writeImage(t, filepath.Join(phone, fmt.Sprintf("%02d.png", i)), 1080, 1920, "png")
		writeImage(t, filepath.Join(tablet, fmt.Sprintf("%02d.png", i)), 1920, 1080, "png")
		writeImage(t, filepath.Join(tv, fmt.Sprintf("%02d.png", i)), 1920, 1080, "png")
	}

	report, err := app.ReadMetadata(root, 42)
	if err != nil {
		t.Fatalf("ReadMetadata: %v", err)
	}
	if len(report.Screenshots) != 9 {
		t.Fatalf("screenshots = %d, want 9", len(report.Screenshots))
	}
	counts := map[rustore.ScreenshotType]int{}
	for _, shot := range report.Screenshots {
		counts[shot.Type]++
		if shot.Ordinal < 0 || shot.Ordinal > 2 {
			t.Errorf("ordinal = %d", shot.Ordinal)
		}
	}
	if counts[rustore.ScreenshotPhone] != 3 || counts[rustore.ScreenshotTablet] != 3 ||
		counts[rustore.ScreenshotTV] != 3 {
		t.Errorf("counts = %v", counts)
	}
	if !slices.Equal(report.Ignored, []string{"ru-RU/images/featureGraphic.png"}) {
		t.Errorf("ignored = %v", report.Ignored)
	}
}

func TestReadMetadataScreenshotEmptyDirInherits(t *testing.T) {
	t.Parallel()

	root := writeMetadata(t, map[string]string{
		"ru-RU/changelogs/42.txt": "Изменения",
	})
	if err := os.MkdirAll(filepath.Join(root, "ru-RU", "images", "phoneScreenshots"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	report, err := app.ReadMetadata(root, 42)
	if err != nil {
		t.Fatalf("ReadMetadata: %v", err)
	}
	if len(report.Screenshots) != 0 {
		t.Errorf("screenshots = %v, want пусто", report.Screenshots)
	}
}

func TestReadMetadataScreenshotErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(t *testing.T, root string)
		want  string
	}{
		{
			name: "два скриншота",
			setup: func(t *testing.T, root string) {
				dir := filepath.Join(root, "ru-RU", "images", "phoneScreenshots")
				writeImage(t, filepath.Join(dir, "01.png"), 1080, 1920, "png")
				writeImage(t, filepath.Join(dir, "02.png"), 1080, 1920, "png")
			},
			want: "минимум 3",
		},
		{
			name: "одиннадцать скриншотов",
			setup: func(t *testing.T, root string) {
				dir := filepath.Join(root, "ru-RU", "images", "phoneScreenshots")
				for i := 1; i <= 11; i++ {
					writeImage(t, filepath.Join(dir, fmt.Sprintf("%02d.png", i)), 320, 568, "png")
				}
			},
			want: "не более 10",
		},
		{
			name: "разная ориентация",
			setup: func(t *testing.T, root string) {
				dir := filepath.Join(root, "ru-RU", "images", "phoneScreenshots")
				writeImage(t, filepath.Join(dir, "01.png"), 1080, 1920, "png")
				writeImage(t, filepath.Join(dir, "02.png"), 1080, 1920, "png")
				writeImage(t, filepath.Join(dir, "03.png"), 1920, 1080, "png")
			},
			want: "одной ориентации",
		},
		{
			name: "неверные пропорции",
			setup: func(t *testing.T, root string) {
				dir := filepath.Join(root, "ru-RU", "images", "phoneScreenshots")
				for i := 1; i <= 3; i++ {
					writeImage(t, filepath.Join(dir, fmt.Sprintf("%02d.png", i)), 1000, 1000, "png")
				}
			},
			want: "16:9",
		},
		{
			name: "сторона меньше 320",
			setup: func(t *testing.T, root string) {
				dir := filepath.Join(root, "ru-RU", "images", "phoneScreenshots")
				for i := 1; i <= 3; i++ {
					writeImage(t, filepath.Join(dir, fmt.Sprintf("%02d.png", i)), 319, 567, "png")
				}
			},
			want: "320",
		},
		{
			name: "сторона больше 3840",
			setup: func(t *testing.T, root string) {
				dir := filepath.Join(root, "ru-RU", "images", "phoneScreenshots")
				for i := 1; i <= 3; i++ {
					writeImage(t, filepath.Join(dir, fmt.Sprintf("%02d.png", i)), 3841, 2160, "png")
				}
			},
			want: "3840",
		},
		{
			name: "файл больше 5 МБ",
			setup: func(t *testing.T, root string) {
				dir := filepath.Join(root, "ru-RU", "images", "phoneScreenshots")
				writeImage(t, filepath.Join(dir, "01.png"), 1080, 1920, "png")
				writeImage(t, filepath.Join(dir, "02.png"), 1080, 1920, "png")
				writeMetadataFile(t, root, "ru-RU/images/phoneScreenshots/03.png",
					strings.Repeat("x", 5<<20+1))
			},
			want: "5 МБ",
		},
		{
			name: "неподдерживаемое расширение",
			setup: func(t *testing.T, root string) {
				dir := filepath.Join(root, "ru-RU", "images", "phoneScreenshots")
				writeImage(t, filepath.Join(dir, "01.png"), 1080, 1920, "png")
				writeImage(t, filepath.Join(dir, "02.png"), 1080, 1920, "png")
				writeMetadataFile(t, root, "ru-RU/images/phoneScreenshots/03.gif", "gif")
			},
			want: "поддерживаются .png",
		},
		{
			name: "не изображение",
			setup: func(t *testing.T, root string) {
				dir := filepath.Join(root, "ru-RU", "images", "phoneScreenshots")
				writeImage(t, filepath.Join(dir, "01.png"), 1080, 1920, "png")
				writeImage(t, filepath.Join(dir, "02.png"), 1080, 1920, "png")
				writeMetadataFile(t, root, "ru-RU/images/phoneScreenshots/03.png", "не картинка")
			},
			want: "PNG или JPEG",
		},
		{
			name: "расширение не совпадает",
			setup: func(t *testing.T, root string) {
				dir := filepath.Join(root, "ru-RU", "images", "phoneScreenshots")
				writeImage(t, filepath.Join(dir, "01.png"), 1080, 1920, "png")
				writeImage(t, filepath.Join(dir, "02.png"), 1080, 1920, "png")
				writeImage(t, filepath.Join(dir, "03.png"), 1080, 1920, "png")
				if err := os.Rename(filepath.Join(dir, "03.png"), filepath.Join(dir, "03.jpg")); err != nil {
					t.Fatalf("rename: %v", err)
				}
			},
			want: "PNG",
		},
		{
			name: "вложенный каталог",
			setup: func(t *testing.T, root string) {
				dir := filepath.Join(root, "ru-RU", "images", "phoneScreenshots")
				writeImage(t, filepath.Join(dir, "01.png"), 1080, 1920, "png")
				writeImage(t, filepath.Join(dir, "02.png"), 1080, 1920, "png")
				if err := os.MkdirAll(filepath.Join(dir, "03"), 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
			},
			want: "вложенный каталог",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root := writeMetadata(t, map[string]string{
				"ru-RU/changelogs/42.txt": "Изменения",
			})
			test.setup(t, root)

			_, err := app.ReadMetadata(root, 42)
			if err == nil {
				t.Fatal("ожидалась ошибка")
			}
			if kind := exitcode.KindOf(err); kind != exitcode.KindInvalidMetadata {
				t.Errorf("KindOf = %q, want %q", kind, exitcode.KindInvalidMetadata)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("err = %q, want %q", err, test.want)
			}
		})
	}
}
