package cli_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vyachin/rustore-publisher/internal/artifact/artifacttest"
	"github.com/vyachin/rustore-publisher/internal/cli"
	"github.com/vyachin/rustore-publisher/internal/version"
)

var (
	cliKeyOnce sync.Once
	cliKey     *rsa.PrivateKey
)

func testKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	cliKeyOnce.Do(func() {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic(err)
		}
		cliKey = key
	})
	return cliKey
}

// runAPI запускает команду против подменного сервера RuStore.
func runAPI(t *testing.T, handler http.HandlerFunc, args ...string) (code int, stdout, stderr string) {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	key := testKey(t)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	credentials, err := json.Marshal(map[string]string{
		"keyId":      "key-1",
		"privateKey": base64.StdEncoding.EncodeToString(der),
	})
	if err != nil {
		t.Fatalf("marshal credentials: %v", err)
	}
	credentialsPath := filepath.Join(t.TempDir(), "rustore.json")
	if err := os.WriteFile(credentialsPath, credentials, 0o600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}

	var out, errOut bytes.Buffer
	app := &cli.App{
		Stdout:     &out,
		Stderr:     &errOut,
		Version:    version.Info{Version: "0.0.1", Commit: "abc1234", Date: "2026-11-09T18:00:00Z"},
		APIBaseURL: server.URL,
		Now:        func() time.Time { return time.Date(2026, 11, 9, 18, 0, 0, 0, time.UTC) },
	}

	fullArgs := append([]string{"--credentials", credentialsPath}, args...)
	code = cli.Execute(context.Background(), app, fullArgs)
	return code, out.String(), errOut.String()
}

// storeHandler отвечает на авторизацию и передаёт остальные запросы в api.
func storeHandler(api http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/public/auth/" {
			writeStoreJSON(w, http.StatusOK,
				`{"code":"OK","body":{"jwe":"token-1","ttl":900}}`)
			return
		}
		api(w, r)
	}
}

func writeStoreJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func statusListBody(versions ...string) string {
	return `{"code":"OK","body":{"content":[` + strings.Join(versions, ",") +
		`],"pageNumber":0,"pageSize":100,"totalElements":` +
		strconv.Itoa(len(versions)) + `,"totalPages":1}}`
}

func TestStatusJSON(t *testing.T) {
	t.Parallel()

	handler := storeHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/public/v1/application/com.example.app/version" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Public-Token") != "token-1" {
			t.Errorf("Public-Token = %q", r.Header.Get("Public-Token"))
		}
		writeStoreJSON(w, http.StatusOK, statusListBody(
			`{"versionId":1,"versionCode":10,"versionStatus":"ACTIVE","publishType":"MANUAL","testingType":null,"partialValue":100,"whatsNew":"старое","sendDateForModer":"2026-01-01T00:00:00Z"}`,
			`{"versionId":2,"versionCode":11,"versionStatus":"DRAFT","publishType":"MANUAL","testingType":null,"partialValue":100}`,
			`{"versionId":3,"versionCode":12,"versionStatus":"ACTIVE","publishType":"INSTANTLY","testingType":"alpha"}`,
			`{"versionId":4,"versionCode":9,"versionStatus":"ARCHIVED","publishType":"MANUAL","testingType":null}`,
		))
	})

	code, stdout, stderr := runAPI(t, handler, "--json", "status", "-p", "com.example.app")
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}

	obj := decodeOne(t, stdout)
	assertKeysMatchSchema(t, "result", obj)

	if obj["command"] != "status" || obj["outcome"] != "inspected" || obj["phase"] != "status" {
		t.Errorf("result = %v", obj)
	}
	if obj["version_id"] != nil {
		t.Errorf("без --version-id верхние поля должны быть null: %v", obj["version_id"])
	}

	latest, ok := obj["latest_release"].(map[string]any)
	if !ok || latest["version_id"] != float64(1) || latest["version_status"] != "ACTIVE" {
		t.Errorf("latest_release = %v", obj["latest_release"])
	}
	draft, ok := obj["draft"].(map[string]any)
	if !ok || draft["version_id"] != float64(2) {
		t.Errorf("draft = %v", obj["draft"])
	}
	if latest["published"] != true || latest["submitted"] != true {
		t.Errorf("факты latest_release = %v", latest)
	}
	if draft["submitted"] != false || draft["published"] != false {
		t.Errorf("факты draft = %v", draft)
	}
}

func TestStatusVersionWithDetail(t *testing.T) {
	t.Parallel()

	handler := storeHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/public/v1/"):
			writeStoreJSON(w, http.StatusOK, statusListBody(
				`{"versionId":7,"versionCode":20,"versionStatus":"ACTIVE","publishType":"MANUAL","testingType":null,"partialValue":100,"whatsNew":"новое"}`,
			))
		case r.URL.Path == "/public/v2/application/com.example.app/version/7":
			writeStoreJSON(w, http.StatusOK, `{"code":"OK","body":{
				"versionId":7,"versionStatus":"ACTIVE","statusComment":"комментарий модератора",
				"permissions":{"location":[{"permission":"android.permission.ACCESS_FINE_LOCATION"}]},
				"packages":[{"verCode":20,"isMainApk":true,"autoDetectedSdks":{"ads":["com.example.ads"]}}]}}`)
		default:
			t.Errorf("неожиданный путь %q", r.URL.Path)
		}
	})

	code, stdout, stderr := runAPI(t, handler, "--json", "status", "-p", "com.example.app", "--version-id", "7")
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}

	obj := decodeOne(t, stdout)
	assertKeysMatchSchema(t, "result", obj)
	if obj["version_id"] != float64(7) || obj["version_status"] != "ACTIVE" {
		t.Errorf("result = %v", obj)
	}
	if obj["latest_release"] != nil || obj["draft"] != nil {
		t.Errorf("с --version-id объекты версий должны быть null")
	}
	if _, ok := obj["permissions"].(map[string]any); !ok {
		t.Errorf("permissions = %v", obj["permissions"])
	}
	if _, ok := obj["auto_detected_sdks"].(map[string]any); !ok {
		t.Errorf("auto_detected_sdks = %v", obj["auto_detected_sdks"])
	}

	// В текстовом режиме комментарий модерации выводится отдельной строкой.
	code, stdout, _ = runAPI(t, handler, "status", "-p", "com.example.app", "--version-id", "7")
	if code != 0 {
		t.Fatalf("text code = %d", code)
	}
	if !strings.Contains(stdout, "Комментарий модерации: комментарий модератора") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestStatusVersionNotFound(t *testing.T) {
	t.Parallel()

	handler := storeHandler(func(w http.ResponseWriter, _ *http.Request) {
		writeStoreJSON(w, http.StatusOK, statusListBody(
			`{"versionId":1,"versionCode":10,"versionStatus":"ACTIVE","testingType":null}`,
		))
	})

	code, stdout, _ := runAPI(t, handler, "--json", "status", "-p", "com.example.app", "--version-id", "99")
	if code != 4 {
		t.Fatalf("code = %d, want 4", code)
	}
	obj := decodeOne(t, stdout)
	errObj, _ := obj["error"].(map[string]any)
	if errObj["code"] != "VERSION_NOT_FOUND" {
		t.Errorf("error = %v", obj["error"])
	}
}

func TestStatusMultipleDrafts(t *testing.T) {
	t.Parallel()

	handler := storeHandler(func(w http.ResponseWriter, _ *http.Request) {
		writeStoreJSON(w, http.StatusOK, statusListBody(
			`{"versionId":1,"versionCode":10,"versionStatus":"DRAFT","testingType":null}`,
			`{"versionId":2,"versionCode":11,"versionStatus":"DRAFT","testingType":null}`,
		))
	})

	code, stdout, _ := runAPI(t, handler, "--json", "status", "-p", "com.example.app")
	if code != 5 {
		t.Fatalf("code = %d, want 5", code)
	}
	obj := decodeOne(t, stdout)
	errObj, _ := obj["error"].(map[string]any)
	if errObj["code"] != "VERSION_STATE_CONFLICT" {
		t.Errorf("error = %v", obj["error"])
	}
}

func TestStatusDetailUnavailableKeepsSuccess(t *testing.T) {
	t.Parallel()

	handler := storeHandler(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/public/v2/") {
			writeStoreJSON(w, http.StatusInternalServerError, `{"code":"error","message":"сбой"}`)
			return
		}
		writeStoreJSON(w, http.StatusOK, statusListBody(
			`{"versionId":1,"versionCode":10,"versionStatus":"ACTIVE","testingType":null}`,
		))
	})

	code, stdout, _ := runAPI(t, handler, "--json", "status", "-p", "com.example.app", "--version-id", "1")
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	obj := decodeOne(t, stdout)
	if obj["outcome"] != "inspected" {
		t.Errorf("outcome = %v", obj["outcome"])
	}
	warnings, ok := obj["warnings"].([]any)
	if !ok || len(warnings) != 1 {
		t.Fatalf("warnings = %v", obj["warnings"])
	}
	warning, _ := warnings[0].(map[string]any)
	if warning["code"] != "STATUS_UNAVAILABLE" {
		t.Errorf("warning = %v", warning)
	}
	if obj["version_status"] != "ACTIVE" {
		t.Errorf("version_status = %v", obj["version_status"])
	}
}

func TestDoctorJSON(t *testing.T) {
	t.Parallel()

	handler := storeHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/public/v1/"):
			writeStoreJSON(w, http.StatusOK, statusListBody(
				`{"versionId":5,"versionCode":30,"versionStatus":"ACTIVE","publishType":"MANUAL","testingType":null,"partialValue":100}`,
				`{"versionId":6,"versionCode":31,"versionStatus":"DRAFT","testingType":null}`,
			))
		case r.URL.Path == "/public/v2/application/com.example.app/version/5":
			writeStoreJSON(w, http.StatusOK, `{"code":"OK","body":{
				"versionId":5,"versionStatus":"ACTIVE",
				"permissions":{"camera":[{"permission":"android.permission.CAMERA"}]},
				"packages":[{"verCode":30,"isMainApk":true,"autoDetectedSdks":{"ads":["com.example.ads"]}}]}}`)
		default:
			t.Errorf("неожиданный путь %q", r.URL.Path)
		}
	})

	code, stdout, stderr := runAPI(t, handler, "--json", "doctor", "-p", "com.example.app")
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}

	obj := decodeOne(t, stdout)
	assertKeysMatchSchema(t, "result", obj)
	if obj["command"] != "doctor" || obj["outcome"] != "inspected" {
		t.Errorf("result = %v", obj)
	}
	if obj["version_id"] != float64(5) || obj["version_status"] != "ACTIVE" {
		t.Errorf("version = %v", obj)
	}
	if _, ok := obj["permissions"].(map[string]any); !ok {
		t.Errorf("permissions = %v", obj["permissions"])
	}
	if _, ok := obj["auto_detected_sdks"].(map[string]any); !ok {
		t.Errorf("auto_detected_sdks = %v", obj["auto_detected_sdks"])
	}
}

func TestDoctorNoActiveVersion(t *testing.T) {
	t.Parallel()

	handler := storeHandler(func(w http.ResponseWriter, _ *http.Request) {
		writeStoreJSON(w, http.StatusOK, statusListBody(
			`{"versionId":1,"versionCode":10,"versionStatus":"DRAFT","testingType":null}`,
		))
	})

	code, stdout, stderr := runAPI(t, handler, "doctor", "-p", "com.example.app")
	if code != 0 {
		t.Fatalf("code = %d", code)
	}
	if !strings.Contains(stdout, "Активная версия: не найдена") {
		t.Errorf("stdout = %q, stderr = %q", stdout, stderr)
	}
}

func TestDoctorRejectsMismatchedAAB(t *testing.T) {
	t.Parallel()

	var requests int32
	handler := storeHandler(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&requests, 1)
		writeStoreJSON(w, http.StatusOK, statusListBody())
	})

	aab := writeAAB(t, "com.other.app", 42)
	code, stdout, _ := runAPI(t, handler, "--json", "doctor", "-p", "com.example.app", "-f", aab)
	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	obj := decodeOne(t, stdout)
	errObj, _ := obj["error"].(map[string]any)
	if errObj["code"] != "INVALID_AAB" {
		t.Errorf("error = %v", obj["error"])
	}
	if requests != 0 {
		t.Errorf("сетевых запросов = %d, want 0", requests)
	}
}

func TestStatusMissingPackage(t *testing.T) {
	t.Parallel()

	var requests int32
	handler := storeHandler(func(_ http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&requests, 1)
	})

	code, stdout, _ := runAPI(t, handler, "--json", "status")
	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	obj := decodeOne(t, stdout)
	errObj, _ := obj["error"].(map[string]any)
	if errObj["code"] != "INVALID_ARGUMENTS" {
		t.Errorf("error = %v", obj["error"])
	}
	if requests != 0 {
		t.Errorf("сетевых запросов = %d, want 0", requests)
	}
}

func TestInvalidCredentials(t *testing.T) {
	t.Parallel()

	var out, errOut bytes.Buffer
	app := &cli.App{
		Stdout:  &out,
		Stderr:  &errOut,
		Version: version.Info{Version: "0.0.1"},
	}
	code := cli.Execute(context.Background(), app,
		[]string{
			"--json", "--credentials", filepath.Join(t.TempDir(), "missing.json"),
			"status", "-p", "com.example.app",
		})
	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	obj := decodeOne(t, out.String())
	errObj, _ := obj["error"].(map[string]any)
	if errObj["code"] != "INVALID_CREDENTIALS" {
		t.Errorf("error = %v", obj["error"])
	}
}

// writeAAB собирает минимальный AAB с заданным манифестом.
func writeAAB(t *testing.T, packageName string, versionCode int64) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "app.aab")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create AAB: %v", err)
	}
	writer := zip.NewWriter(file)
	entries := map[string][]byte{
		"BundleConfig.pb":                   []byte("config"),
		"base/manifest/AndroidManifest.xml": artifacttest.Manifest(packageName, versionCode, "1.0"),
	}
	for name, body := range entries {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("zip entry: %v", err)
		}
		if _, err := entry.Write(body); err != nil {
			t.Fatalf("zip write: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return path
}
