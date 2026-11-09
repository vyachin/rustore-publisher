package app_test

import (
	"strings"
	"testing"

	"github.com/vyachin/rustore-publisher/internal/app"
	"github.com/vyachin/rustore-publisher/internal/artifact"
	"github.com/vyachin/rustore-publisher/internal/exitcode"
)

func TestCheckIdentity(t *testing.T) {
	t.Parallel()

	identity := artifact.Identity{Package: "com.example.app", VersionCode: 42}

	tests := []struct {
		name        string
		packageName string
		versionCode int64
		wantKind    exitcode.Kind
	}{
		{name: "оба совпадают", packageName: "com.example.app", versionCode: 42},
		{name: "не заданы", packageName: "", versionCode: 0},
		{name: "другой package", packageName: "com.example.other", versionCode: 0, wantKind: exitcode.KindInvalidAAB},
		{name: "другой код", packageName: "", versionCode: 43, wantKind: exitcode.KindInvalidAAB},
		{name: "отрицательный код", packageName: "", versionCode: -1, wantKind: exitcode.KindInvalidArguments},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := app.CheckIdentity(identity, test.packageName, test.versionCode)
			if test.wantKind == "" {
				if err != nil {
					t.Fatalf("неожиданная ошибка: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("ожидалась ошибка")
			}
			if kind := exitcode.KindOf(err); kind != test.wantKind {
				t.Errorf("KindOf = %q, want %q", kind, test.wantKind)
			}
		})
	}
}

func TestCheckArtifactPath(t *testing.T) {
	t.Parallel()

	if err := app.CheckArtifactPath("app.aab"); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}

	err := app.CheckArtifactPath("")
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
	if kind := exitcode.KindOf(err); kind != exitcode.KindInvalidArguments {
		t.Errorf("KindOf = %q", kind)
	}
	if !strings.Contains(exitcode.HintOf(err), "--aab") {
		t.Errorf("hint = %q", exitcode.HintOf(err))
	}
}
