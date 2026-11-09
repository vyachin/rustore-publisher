package artifact_test

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyachin/rustore-publisher/internal/artifact"
	"github.com/vyachin/rustore-publisher/internal/artifact/artifacttest"
	"github.com/vyachin/rustore-publisher/internal/exitcode"
)

// writeBundle собирает zip-файл с заданными элементами и возвращает путь.
func writeBundle(t *testing.T, files map[string][]byte) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "app-release.aab")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("создать AAB: %v", err)
	}

	writer := zip.NewWriter(file)
	for name, body := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("создать элемент %s: %v", name, err)
		}
		if _, err := entry.Write(body); err != nil {
			t.Fatalf("записать элемент %s: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("закрыть zip: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("закрыть файл: %v", err)
	}
	return path
}

// validManifest возвращает корректный манифест базового модуля.
func validManifest() []byte {
	return artifacttest.Manifest("com.example.app", 42, "1.2.3")
}

func TestInspectComputesSizeAndChecksum(t *testing.T) {
	t.Parallel()

	path := writeBundle(t, map[string][]byte{
		"BundleConfig.pb":                   []byte("bundle-config"),
		"base/manifest/AndroidManifest.xml": validManifest(),
	})

	info, err := artifact.Inspect(path)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("прочитать AAB: %v", err)
	}
	sum := sha256.Sum256(data)

	if info.Path != path {
		t.Errorf("Path = %q, want %q", info.Path, path)
	}
	if info.Size != int64(len(data)) {
		t.Errorf("Size = %d, want %d", info.Size, len(data))
	}
	if info.SHA256 != hex.EncodeToString(sum[:]) {
		t.Errorf("SHA256 = %q, want %q", info.SHA256, hex.EncodeToString(sum[:]))
	}
	if info.Signed {
		t.Error("неподписанный AAB отмечен как подписанный")
	}
}

func TestInspectDetectsSignature(t *testing.T) {
	t.Parallel()

	path := writeBundle(t, map[string][]byte{
		"BundleConfig.pb":   []byte("bundle-config"),
		"META-INF/CERT.RSA": []byte("signature"),
	})

	info, err := artifact.Inspect(path)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if !info.Signed {
		t.Error("подпись JAR не найдена")
	}
}

func TestOpenParsesIdentity(t *testing.T) {
	t.Parallel()

	path := writeBundle(t, map[string][]byte{
		"BundleConfig.pb":                   []byte("bundle-config"),
		"base/manifest/AndroidManifest.xml": validManifest(),
	})

	bundle, err := artifact.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() {
		if err := bundle.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	if bundle.Identity.Package != "com.example.app" {
		t.Errorf("Package = %q", bundle.Identity.Package)
	}
	if bundle.Identity.VersionCode != 42 {
		t.Errorf("VersionCode = %d", bundle.Identity.VersionCode)
	}
	if bundle.Identity.VersionName != "1.2.3" {
		t.Errorf("VersionName = %q", bundle.Identity.VersionName)
	}
	if err := bundle.Rewind(); err != nil {
		t.Fatalf("Rewind: %v", err)
	}
	if err := bundle.Close(); err != nil {
		t.Fatalf("повторный Close: %v", err)
	}
}

func TestInspectRejectsInvalidBundles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		files map[string][]byte
		want  string
	}{
		{
			name:  "нет BundleConfig.pb",
			files: map[string][]byte{"base/manifest/AndroidManifest.xml": validManifest()},
			want:  "не является Android App Bundle",
		},
		{
			name:  "нет манифеста",
			files: map[string][]byte{"BundleConfig.pb": []byte("config")},
			want:  "AndroidManifest.xml",
		},
		{
			name: "пустой манифест",
			files: map[string][]byte{
				"BundleConfig.pb":                   []byte("config"),
				"base/manifest/AndroidManifest.xml": {},
			},
			want: "корнев",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			path := writeBundle(t, test.files)

			_, err := artifact.Open(path)
			if err == nil {
				t.Fatal("ожидалась ошибка")
			}
			if kind := exitcode.KindOf(err); kind != exitcode.KindInvalidAAB {
				t.Errorf("KindOf = %q, want %q", kind, exitcode.KindInvalidAAB)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("err = %q, want %q", err, test.want)
			}
		})
	}
}

func TestInspectRejectsEmptyAndNonZipFiles(t *testing.T) {
	t.Parallel()

	empty := filepath.Join(t.TempDir(), "empty.aab")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatalf("записать пустой файл: %v", err)
	}

	plain := filepath.Join(t.TempDir(), "plain.aab")
	if err := os.WriteFile(plain, []byte("не zip"), 0o600); err != nil {
		t.Fatalf("записать текстовый файл: %v", err)
	}

	for _, path := range []string{empty, plain, filepath.Join(t.TempDir(), "missing.aab")} {
		_, err := artifact.Inspect(path)
		if err == nil {
			t.Fatalf("%s: ожидалась ошибка", path)
		}
		if kind := exitcode.KindOf(err); kind != exitcode.KindInvalidAAB {
			t.Errorf("%s: KindOf = %q, want %q", path, kind, exitcode.KindInvalidAAB)
		}
	}
}
