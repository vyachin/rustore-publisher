package app_test

import (
	"os"
	"path/filepath"
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

	text, err := app.ReadChangelog(root, 42)
	if err != nil {
		t.Fatalf("ReadChangelog: %v", err)
	}
	if text != "Исправлены ошибки.\nУскорен запуск.\n" {
		t.Errorf("text = %q", text)
	}
}

func TestReadChangelogNormalizesNewlines(t *testing.T) {
	t.Parallel()

	root := writeMetadata(t, map[string]string{
		"ru-RU/changelogs/7.txt": "Первая строка\r\nВторая строка\rТретья",
	})

	text, err := app.ReadChangelog(root, 7)
	if err != nil {
		t.Fatalf("ReadChangelog: %v", err)
	}
	if text != "Первая строка\nВторая строка\nТретья" {
		t.Errorf("text = %q", text)
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

	if _, err := app.ReadChangelog(root, 42); err != nil {
		t.Fatalf("ReadChangelog: %v", err)
	}
}

func TestReadChangelogAcceptsFiveThousandRunes(t *testing.T) {
	t.Parallel()

	text := strings.Repeat("я", 5000)
	root := writeMetadata(t, map[string]string{"ru-RU/changelogs/42.txt": text})

	got, err := app.ReadChangelog(root, 42)
	if err != nil {
		t.Fatalf("ReadChangelog: %v", err)
	}
	if len([]rune(got)) != 5000 {
		t.Errorf("длина = %d", len([]rune(got)))
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
			name:     "другая локаль",
			metadata: map[string]string{"en-US/changelogs/42.txt": "changes"},
			code:     42,
			want:     "en-US",
			kind:     exitcode.KindInvalidChangelog,
		},
		{
			name:     "файлы карточки",
			metadata: map[string]string{"ru-RU/listings/42.txt": "карточка"},
			code:     42,
			want:     "listings",
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
