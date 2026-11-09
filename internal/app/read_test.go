package app_test

import (
	"testing"

	"github.com/vyachin/rustore-publisher/internal/app"
	"github.com/vyachin/rustore-publisher/internal/exitcode"
	"github.com/vyachin/rustore-publisher/internal/rustore"
)

func version(id, code int64, status, testing string) rustore.Version {
	version := rustore.Version{
		VersionID:   id,
		VersionCode: code,
		Status:      status,
		Testing:     rustore.TestingType{Kind: testing, Present: true},
	}
	if testing == rustore.TestingAlpha {
		raw := "alpha"
		version.Testing.Raw = &raw
	}
	return version
}

func TestSelectLatestRelease(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		versions []rustore.Version
		wantID   int64
	}{
		{
			name:     "пусто",
			versions: nil,
			wantID:   0,
		},
		{
			name: "только alpha и черновики",
			versions: []rustore.Version{
				version(1, 10, rustore.StatusDraft, rustore.TestingRelease),
				version(2, 11, rustore.StatusActive, rustore.TestingAlpha),
			},
			wantID: 0,
		},
		{
			name: "максимальный код среди релизов",
			versions: []rustore.Version{
				version(1, 10, rustore.StatusActive, rustore.TestingRelease),
				version(2, 12, rustore.StatusArchived, rustore.TestingRelease),
				version(3, 11, rustore.StatusActive, rustore.TestingRelease),
			},
			wantID: 2,
		},
		{
			name: "равный код — больший id",
			versions: []rustore.Version{
				version(5, 10, rustore.StatusActive, rustore.TestingRelease),
				version(7, 10, rustore.StatusRejectedByModerator, rustore.TestingRelease),
			},
			wantID: 7,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := app.SelectLatestRelease(test.versions)
			if test.wantID == 0 {
				if got != nil {
					t.Fatalf("got %+v, want nil", got)
				}
				return
			}
			if got == nil || got.VersionID != test.wantID {
				t.Fatalf("got %+v, want id %d", got, test.wantID)
			}
		})
	}
}

func TestSelectDraft(t *testing.T) {
	t.Parallel()

	if draft, err := app.SelectDraft(nil); err != nil || draft != nil {
		t.Fatalf("пусто: %v, %v", draft, err)
	}

	draft, err := app.SelectDraft([]rustore.Version{
		version(1, 10, rustore.StatusActive, rustore.TestingRelease),
		version(2, 11, rustore.StatusDraft, rustore.TestingAlpha),
	})
	if err != nil || draft == nil || draft.VersionID != 2 {
		t.Fatalf("один черновик: %+v, %v", draft, err)
	}

	_, err = app.SelectDraft([]rustore.Version{
		version(2, 11, rustore.StatusDraft, rustore.TestingRelease),
		version(3, 12, rustore.StatusDraft, rustore.TestingAlpha),
	})
	if err == nil {
		t.Fatal("ожидался конфликт")
	}
	if kind := exitcode.KindOf(err); kind != exitcode.KindVersionStateConflict {
		t.Errorf("KindOf = %q", kind)
	}
}

func TestSelectActive(t *testing.T) {
	t.Parallel()

	active := app.SelectActive([]rustore.Version{
		version(1, 10, rustore.StatusPreviousActive, rustore.TestingRelease),
		version(2, 11, rustore.StatusPartialActive, rustore.TestingRelease),
		version(3, 12, rustore.StatusActive, rustore.TestingRelease),
	})
	if active == nil || active.VersionID != 3 {
		t.Fatalf("active = %+v", active)
	}

	if got := app.SelectActive([]rustore.Version{
		version(1, 10, rustore.StatusDraft, rustore.TestingRelease),
	}); got != nil {
		t.Errorf("черновик не должен считаться активным: %+v", got)
	}
}

func TestFindVersion(t *testing.T) {
	t.Parallel()

	versions := []rustore.Version{
		version(1, 10, rustore.StatusActive, rustore.TestingRelease),
		version(2, 11, rustore.StatusDraft, rustore.TestingRelease),
	}
	if got := app.FindVersion(versions, 2); got == nil || got.VersionID != 2 {
		t.Fatalf("FindVersion = %+v", got)
	}
	if got := app.FindVersion(versions, 3); got != nil {
		t.Fatalf("FindVersion = %+v, want nil", got)
	}
}

func TestMainPackage(t *testing.T) {
	t.Parallel()

	detail := &rustore.VersionDetail{Packages: []rustore.Package{
		{VerCode: 10, IsMainApk: false},
		{VerCode: 11, IsMainApk: true},
		{VerCode: 12, IsMainApk: true},
	}}

	pkg := app.MainPackage(detail, 11)
	if pkg == nil || pkg.VerCode != 11 {
		t.Fatalf("MainPackage = %+v", pkg)
	}

	pkg = app.MainPackage(detail, 99)
	if pkg == nil || pkg.VerCode != 11 {
		t.Fatalf("fallback MainPackage = %+v", pkg)
	}

	if got := app.MainPackage(nil, 1); got != nil {
		t.Fatalf("MainPackage(nil) = %+v", got)
	}
}
