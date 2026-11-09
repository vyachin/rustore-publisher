package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyachin/rustore-publisher/internal/cli"
	"github.com/vyachin/rustore-publisher/internal/version"
)

func run(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()

	var out, errOut bytes.Buffer
	app := &cli.App{
		Stdout: &out,
		Stderr: &errOut,
		Version: version.Info{
			Version: "0.0.1",
			Commit:  "abc1234",
			Date:    "2026-11-09T18:00:00Z",
		},
	}

	code = cli.Execute(context.Background(), app, args)
	return code, out.String(), errOut.String()
}

func TestVersionText(t *testing.T) {
	t.Parallel()

	code, stdout, stderr := run(t, "version")
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	for _, want := range []string{"rustore-publisher 0.0.1", "abc1234", "2026-11-09T18:00:00Z"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
	}
}

func TestVersionJSONMatchesSchema(t *testing.T) {
	t.Parallel()

	code, stdout, stderr := run(t, "--json", "version")
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}

	obj := decodeOne(t, stdout)
	assertKeysMatchSchema(t, "version_success", obj)

	if obj["command"] != "version" || obj["version"] != "0.0.1" || obj["commit"] != "abc1234" {
		t.Errorf("result = %v", obj)
	}
}

func TestCompletionBash(t *testing.T) {
	t.Parallel()

	code, stdout, stderr := run(t, "completion", "bash")
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "rustore-publisher") {
		t.Errorf("скрипт не содержит имя команды: %q", stdout)
	}
}

func TestCompletionJSONIsError(t *testing.T) {
	t.Parallel()

	code, stdout, stderr := run(t, "--json", "completion", "bash")
	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if strings.TrimSpace(stderr) == "" {
		t.Error("stderr пуст")
	}

	obj := decodeOne(t, stdout)
	assertKeysMatchSchema(t, "result", obj)

	if obj["command"] != "completion" || obj["phase"] != "preflight" || obj["outcome"] != "failed" {
		t.Errorf("result = %v", obj)
	}
	errObj, ok := obj["error"].(map[string]any)
	if !ok {
		t.Fatalf("error = %v", obj["error"])
	}
	if errObj["code"] != "INVALID_ARGUMENTS" {
		t.Errorf("error.code = %v", errObj["code"])
	}
	if strings.Contains(stdout, "complete") && strings.Contains(stdout, "# bash") {
		t.Error("в JSON попал фрагмент скрипта")
	}
}

func TestCompletionUnknownShell(t *testing.T) {
	t.Parallel()

	code, _, stderr := run(t, "completion", "tcsh")
	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "неизвестная оболочка") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestUnknownCommandIsUsageError(t *testing.T) {
	t.Parallel()

	code, stdout, stderr := run(t, "--json", "publish")
	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "неизвестная команда") {
		t.Errorf("stderr = %q", stderr)
	}

	obj := decodeOne(t, stdout)
	assertKeysMatchSchema(t, "result", obj)
	if obj["command"] != "rustore-publisher" {
		t.Errorf("command = %v", obj["command"])
	}
}

func TestNegativeTimeoutIsUsageError(t *testing.T) {
	t.Parallel()

	code, stdout, _ := run(t, "--json", "--timeout=-1s", "version")
	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}

	obj := decodeOne(t, stdout)
	if obj["outcome"] != "failed" {
		t.Errorf("outcome = %v", obj["outcome"])
	}
}

func TestZeroTimeoutDisablesLimit(t *testing.T) {
	t.Parallel()

	code, _, stderr := run(t, "--timeout=0", "version")
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
}

func TestZeroRequestTimeoutIsUsageError(t *testing.T) {
	t.Parallel()

	code, _, _ := run(t, "--request-timeout=0", "version")
	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
}

func TestHelp(t *testing.T) {
	t.Parallel()

	code, stdout, stderr := run(t, "--help")
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	for _, want := range []string{"rustore-publisher публикует", "Android App Bundle", "completion", "version", "--json", "--timeout"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("help не содержит %q", want)
		}
	}
}

func TestHelpIsRussian(t *testing.T) {
	t.Parallel()

	_, stdout, _ := run(t, "--help")
	for _, want := range []string{"Использование:", "Доступные команды:", "Флаги:", "показать справку"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("help не содержит %q", want)
		}
	}
	for _, unwanted := range []string{"Available Commands:", "Usage:", "help for"} {
		if strings.Contains(stdout, unwanted) {
			t.Errorf("help содержит английский фрагмент %q", unwanted)
		}
	}
}

func TestHelpCommand(t *testing.T) {
	t.Parallel()

	code, stdout, stderr := run(t, "help", "version")
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "Показать версию") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestUnknownFlagIsUsageError(t *testing.T) {
	t.Parallel()

	code, _, stderr := run(t, "--no-such-flag", "version")
	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "подсказка:") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestJSONDoesNotEscapeHTML(t *testing.T) {
	t.Parallel()

	code, stdout, _ := run(t, "--json", "foo<bar>")
	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if !strings.Contains(stdout, "foo<bar>") {
		t.Errorf("stdout = %q, ожидался символ < без экранирования", stdout)
	}
}

// decodeOne декодирует ровно один JSON-объект и проверяет, что после него
// в stdout ничего нет.
func decodeOne(t *testing.T, stdout string) map[string]any {
	t.Helper()

	decoder := json.NewDecoder(strings.NewReader(stdout))
	var obj map[string]any
	if err := decoder.Decode(&obj); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, stdout)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("в stdout больше одного JSON-объекта: %v", err)
	}
	return obj
}

// assertKeysMatchSchema сверяет поля объекта со схемой: лишние поля
// запрещены, обязательные должны присутствовать.
func assertKeysMatchSchema(t *testing.T, def string, obj map[string]any) {
	t.Helper()

	path := filepath.Join("..", "..", "docs", "schema", "result.schema.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}

	var doc struct {
		Defs map[string]struct {
			Required   []string                   `json:"required"`
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}

	definition, ok := doc.Defs[def]
	if !ok {
		t.Fatalf("в схеме нет $defs.%s", def)
	}

	for key := range obj {
		if _, ok := definition.Properties[key]; !ok {
			t.Errorf("лишнее поле %q, которого нет в свойствах схемы", key)
		}
	}
	for _, key := range definition.Required {
		if _, ok := obj[key]; !ok {
			t.Errorf("нет обязательного поля %q", key)
		}
	}
}
