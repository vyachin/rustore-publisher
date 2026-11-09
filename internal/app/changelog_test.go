package app_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vyachin/rustore-publisher/internal/app"
	"github.com/vyachin/rustore-publisher/internal/exitcode"
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

func TestReadChangelog(t *testing.T) {
	t.Parallel()

	root := writeMetadata(t, map[string]string{
		"ru-RU/changelogs/42.txt": "Исправлены ошибки.\nУскорен запуск.\n",
	})

	report, err := app.ReadChangelog(root, 42)
	if err != nil {
		t.Fatalf("ReadChangelog: %v", err)
	}
	if report.Text != "Исправлены ошибки.\nУскорен запуск.\n" {
		t.Errorf("text = %q", report.Text)
	}
	if len(report.Ignored) != 0 {
		t.Errorf("ignored = %v, want пусто", report.Ignored)
	}
}

func TestReadChangelogNormalizesNewlines(t *testing.T) {
	t.Parallel()

	root := writeMetadata(t, map[string]string{
		"ru-RU/changelogs/7.txt": "Первая строка\r\nВторая строка\rТретья",
	})

	report, err := app.ReadChangelog(root, 7)
	if err != nil {
		t.Fatalf("ReadChangelog: %v", err)
	}
	if report.Text != "Первая строка\nВторая строка\nТретья" {
		t.Errorf("text = %q", report.Text)
	}
}

func TestReadChangelogIgnoresDotFiles(t *testing.T) {
	t.Parallel()

	root := writeMetadata(t, map[string]string{
		".DS_Store":               "служебный",
		"ru-RU/.DS_Store":         "служебный",
		"ru-RU/changelogs/.keep":  "",
		"ru-RU/changelogs/42.txt": "Изменения",
	})

	report, err := app.ReadChangelog(root, 42)
	if err != nil {
		t.Fatalf("ReadChangelog: %v", err)
	}
	if len(report.Ignored) != 0 {
		t.Errorf("ignored = %v, want пусто", report.Ignored)
	}
}

func TestReadChangelogAcceptsPlayListing(t *testing.T) {
	t.Parallel()

	root := writeMetadata(t, map[string]string{
		"README.md":                   "Как устроены метаданные",
		"developer-contacts.json":     `{"email":"dev@example.com"}`,
		"ru-RU/title.txt":             "Приложение",
		"ru-RU/short_description.txt": "Короткое описание",
		"ru-RU/full_description.txt":  "Полное описание",
		"ru-RU/images/icon.png":       "картинка",
		"ru-RU/changelogs/42.txt":     "Изменения версии 42",
		"ru-RU/changelogs/41.txt":     "Изменения версии 41",
		"ru-RU/changelogs/.DS_Store":  "служебный",
		"ru-RU/changelogs/.gitkeep":   "",
		".circleci/config.yml":        "служебный",
	})

	report, err := app.ReadChangelog(root, 42)
	if err != nil {
		t.Fatalf("ReadChangelog: %v", err)
	}
	if report.Text != "Изменения версии 42" {
		t.Errorf("text = %q", report.Text)
	}
	want := []string{
		"README.md",
		"ru-RU/full_description.txt",
		"ru-RU/images",
		"ru-RU/short_description.txt",
		"ru-RU/title.txt",
	}
	if !slices.Equal(report.Ignored, want) {
		t.Errorf("ignored = %v, want %v", report.Ignored, want)
	}
}

func TestReadChangelogAcceptsFiveThousandRunes(t *testing.T) {
	t.Parallel()

	text := strings.Repeat("я", 5000)
	root := writeMetadata(t, map[string]string{"ru-RU/changelogs/42.txt": text})

	report, err := app.ReadChangelog(root, 42)
	if err != nil {
		t.Fatalf("ReadChangelog: %v", err)
	}
	if len([]rune(report.Text)) != 5000 {
		t.Errorf("длина = %d", len([]rune(report.Text)))
	}
}

func TestReadChangelogErrors(t *testing.T) {
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
			_, err := app.ReadChangelog(root, test.code)
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

func TestReadChangelogMissingDirectory(t *testing.T) {
	t.Parallel()

	_, err := app.ReadChangelog(filepath.Join(t.TempDir(), "missing"), 42)
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
	if kind := exitcode.KindOf(err); kind != exitcode.KindInvalidChangelog {
		t.Errorf("KindOf = %q", kind)
	}
}

func TestReadChangelogAllowsContactsFile(t *testing.T) {
	t.Parallel()

	root := writeMetadata(t, map[string]string{
		"developer-contacts.json": `{"email":"dev@example.com"}`,
		"ru-RU/changelogs/42.txt": "Изменения",
	})

	report, err := app.ReadChangelog(root, 42)
	if err != nil {
		t.Fatalf("ReadChangelog: %v", err)
	}
	if report.Text != "Изменения" {
		t.Errorf("text = %q", report.Text)
	}
	if len(report.Ignored) != 0 {
		t.Errorf("ignored = %v, want пусто", report.Ignored)
	}
}

func TestReadChangelogIgnoresUnknownMetadataFile(t *testing.T) {
	t.Parallel()

	root := writeMetadata(t, map[string]string{
		"notes.txt":               "посторонний",
		"ru-RU/changelogs/42.txt": "Изменения",
	})

	report, err := app.ReadChangelog(root, 42)
	if err != nil {
		t.Fatalf("ReadChangelog: %v", err)
	}
	if !slices.Equal(report.Ignored, []string{"notes.txt"}) {
		t.Errorf("ignored = %v, want [notes.txt]", report.Ignored)
	}
}
