package cli

import (
	"testing"

	"github.com/vyachin/rustore-publisher/internal/exitcode"
	"github.com/vyachin/rustore-publisher/internal/rustore"
)

func TestResolveAPIBaseURL(t *testing.T) {
	// t.Setenv нельзя сочетать с t.Parallel.
	t.Setenv("RUSTORE_API_URL", "")

	tests := []struct {
		name string
		app  string
		flag string
		env  string
		want string
		bad  bool
	}{
		{name: "по умолчанию", want: rustore.DefaultBaseURL},
		{name: "переопределение теста", app: "http://127.0.0.1:1", want: "http://127.0.0.1:1"},
		{name: "флаг", flag: "https://api.example.test/", want: "https://api.example.test"},
		{name: "окружение", env: "https://env.example.test", want: "https://env.example.test"},
		{name: "флаг важнее окружения", flag: "https://flag.example.test", env: "https://env.example.test", want: "https://flag.example.test"},
		{name: "http отклоняется", flag: "http://api.example.test", bad: true},
		{name: "пустой хост отклоняется", flag: "https://", bad: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("RUSTORE_API_URL", test.env)
			app := &App{APIBaseURL: test.app}
			got, err := resolveAPIBaseURL(app, &globalFlags{APIURL: test.flag})
			if test.bad {
				if err == nil {
					t.Fatalf("ожидалась ошибка, получено %q", got)
				}
				if kind := exitcode.KindOf(err); kind != exitcode.KindInvalidArguments {
					t.Errorf("KindOf = %q", kind)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveAPIBaseURL: %v", err)
			}
			if got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestEmbeddedCADisabled(t *testing.T) {
	t.Setenv("RUSTORE_NO_EMBEDDED_CA", "")

	if embeddedCADisabled(&globalFlags{}) {
		t.Error("по умолчанию встроенный CA должен быть включён")
	}
	if !embeddedCADisabled(&globalFlags{NoEmbeddedCA: true}) {
		t.Error("флаг не отключает встроенный CA")
	}

	for _, value := range []string{"1", "true", "yes", "TRUE"} {
		t.Setenv("RUSTORE_NO_EMBEDDED_CA", value)
		if !embeddedCADisabled(&globalFlags{}) {
			t.Errorf("значение %q не отключает встроенный CA", value)
		}
	}
	t.Setenv("RUSTORE_NO_EMBEDDED_CA", "0")
	if embeddedCADisabled(&globalFlags{}) {
		t.Error("значение 0 не должно отключать CA")
	}
}
