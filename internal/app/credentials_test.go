package app_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/vyachin/rustore-publisher/internal/app"
	"github.com/vyachin/rustore-publisher/internal/exitcode"
)

var (
	keyOnce sync.Once
	testKey *rsa.PrivateKey
)

func rsaKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()

	keyOnce.Do(func() {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic(err)
		}
		testKey = key
	})
	return testKey
}

func writeCredentials(t *testing.T, value map[string]any) string {
	t.Helper()

	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	path := filepath.Join(t.TempDir(), "rustore.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func pkcs8Base64(t *testing.T, key *rsa.PrivateKey) string {
	t.Helper()

	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal pkcs8: %v", err)
	}
	return base64.StdEncoding.EncodeToString(der)
}

func TestLoadCredentials(t *testing.T) {
	t.Parallel()

	path := writeCredentials(t, map[string]any{
		"keyId":      "key-1",
		"privateKey": pkcs8Base64(t, rsaKey(t)),
	})

	credentials, err := app.LoadCredentials(path)
	if err != nil {
		t.Fatalf("LoadCredentials: %v", err)
	}
	if credentials.KeyID != "key-1" {
		t.Errorf("KeyID = %q", credentials.KeyID)
	}
	if credentials.Key == nil || credentials.Key.N.Cmp(rsaKey(t).N) != 0 {
		t.Error("ключ RSA не совпадает")
	}
}

func TestLoadCredentialsErrors(t *testing.T) {
	t.Parallel()

	pkcs1 := func() string {
		der := x509.MarshalPKCS1PrivateKey(rsaKey(t))
		return base64.StdEncoding.EncodeToString(der)
	}()
	pemKey := func() string {
		der, err := x509.MarshalPKCS8PrivateKey(rsaKey(t))
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	}()
	edKey := func() string {
		_, key, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("ed25519: %v", err)
		}
		der, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			t.Fatalf("marshal ed25519: %v", err)
		}
		return base64.StdEncoding.EncodeToString(der)
	}()

	tests := []struct {
		name  string
		value map[string]any
	}{
		{name: "нет keyId", value: map[string]any{"privateKey": pkcs8Base64(t, rsaKey(t))}},
		{name: "нет privateKey", value: map[string]any{"keyId": "key-1"}},
		{name: "лишнее поле", value: map[string]any{
			"keyId": "key-1", "privateKey": pkcs8Base64(t, rsaKey(t)), "type": "rsa",
		}},
		{name: "битый base64", value: map[string]any{"keyId": "key-1", "privateKey": "не base64!"}},
		{name: "PKCS#1", value: map[string]any{"keyId": "key-1", "privateKey": pkcs1}},
		{name: "PEM", value: map[string]any{"keyId": "key-1", "privateKey": pemKey}},
		{name: "не RSA", value: map[string]any{"keyId": "key-1", "privateKey": edKey}},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			path := writeCredentials(t, test.value)
			_, err := app.LoadCredentials(path)
			if err == nil {
				t.Fatal("ожидалась ошибка")
			}
			if kind := exitcode.KindOf(err); kind != exitcode.KindInvalidCredentials {
				t.Errorf("KindOf = %q, want %q", kind, exitcode.KindInvalidCredentials)
			}
			if strings.TrimSpace(exitcode.HintOf(err)) == "" {
				t.Error("нет подсказки")
			}
		})
	}
}

func TestLoadCredentialsMissingFile(t *testing.T) {
	t.Parallel()

	_, err := app.LoadCredentials(filepath.Join(t.TempDir(), "missing.json"))
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
	if kind := exitcode.KindOf(err); kind != exitcode.KindInvalidCredentials {
		t.Errorf("KindOf = %q", kind)
	}
}

func TestResolveCredentialsPath(t *testing.T) {
	t.Setenv(app.CredentialsEnv, "/env/creds.json")

	if got := app.ResolveCredentialsPath("/flag/creds.json"); got != "/flag/creds.json" {
		t.Errorf("флаг не имеет приоритета: %q", got)
	}
	if got := app.ResolveCredentialsPath(""); got != "/env/creds.json" {
		t.Errorf("переменная окружения не используется: %q", got)
	}
}
