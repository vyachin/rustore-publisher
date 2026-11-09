package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyachin/rustore-publisher/internal/app"
	"github.com/vyachin/rustore-publisher/internal/exitcode"
)

func writeJSONFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), app.ContactsFileName)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write contacts: %v", err)
	}
	return path
}

func TestReadDeveloperContacts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "email",
			body: `{"email":"dev@example.com"}`,
			want: "dev@example.com",
		},
		{
			name: "все поля и пробелы",
			body: `{
				"email":" dev@example.com ",
				"website":" https://example.com ",
				"vkCommunity":"https://vk.com/example",
				"maxMessenger":"https://max.ru/example"
			}`,
			want: "dev@example.com",
		},
		{
			name: "только сайт",
			body: `{"website":"https://example.com"}`,
			want: "",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			path := writeJSONFile(t, test.body)
			contacts, err := app.ReadDeveloperContacts(path)
			if err != nil {
				t.Fatalf("ReadDeveloperContacts: %v", err)
			}
			if contacts.Email != test.want {
				t.Errorf("email = %q, want %q", contacts.Email, test.want)
			}
		})
	}
}

func TestReadDeveloperContactsErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "пустой объект", body: `{}`, want: "способа связи"},
		{name: "только пробелы", body: `{"email":"   "}`, want: "способа связи"},
		{name: "лишнее поле", body: `{"email":"dev@example.com","phone":"123"}`, want: "unknown field"},
		{name: "плохой email", body: `{"email":"dev@example"}`, want: "email"},
		{name: "email без собаки", body: `{"email":"example.com"}`, want: "email"},
		{name: "относительный сайт", body: `{"website":"example.com"}`, want: "http"},
		{name: "ftp", body: `{"website":"ftp://example.com"}`, want: "http"},
		{name: "битый JSON", body: `{"email":`, want: "разобрать"},
		{name: "два объекта", body: `{"email":"dev@example.com"}{"email":"x@example.com"}`, want: "больше одного"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			path := writeJSONFile(t, test.body)
			_, err := app.ReadDeveloperContacts(path)
			if err == nil {
				t.Fatal("ожидалась ошибка")
			}
			if kind := exitcode.KindOf(err); kind != exitcode.KindInvalidArguments {
				t.Errorf("KindOf = %q", kind)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("err = %q, want %q", err, test.want)
			}
			if strings.TrimSpace(exitcode.HintOf(err)) == "" {
				t.Error("нет подсказки")
			}
		})
	}
}

func TestReadDeveloperContactsMissingFile(t *testing.T) {
	t.Parallel()

	_, err := app.ReadDeveloperContacts(filepath.Join(t.TempDir(), "missing.json"))
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
	if kind := exitcode.KindOf(err); kind != exitcode.KindInvalidArguments {
		t.Errorf("KindOf = %q", kind)
	}
}
