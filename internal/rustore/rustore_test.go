package rustore_test

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vyachin/rustore-publisher/internal/exitcode"
	"github.com/vyachin/rustore-publisher/internal/rustore"
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

// clientBase собирает сервер: /public/auth/ отвечает токеном, остальное
// передаётся в api.
func clientBase(t *testing.T, api http.HandlerFunc, authCalls *int32) (*httptest.Server, *rustore.Client) {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/public/auth/", func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(authCalls, 1)
		writeJSON(w, http.StatusOK, `{"code":"OK","message":null,"body":{"jwe":"token-1","ttl":900},"timestamp":"2026-11-09T18:00:00Z"}`)
	})
	mux.HandleFunc("/", api)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	now := time.Date(2026, 11, 9, 18, 0, 0, 0, time.UTC)
	client := mustClient(t, server.URL, func() time.Time { return now }, nil)
	return server, client
}

func mustClient(t *testing.T, baseURL string, now func() time.Time, key *rsa.PrivateKey) *rustore.Client {
	t.Helper()
	if key == nil {
		key = rsaKey(t)
	}
	client, err := rustore.NewClient(rustore.Config{
		BaseURL:     baseURL,
		Credentials: rustore.Credentials{KeyID: "key-1", Key: key},
		Now:         now,
		Sleep:       func(context.Context, time.Duration) error { return nil },
		RetryBase:   time.Millisecond,
		MaxAttempts: 3,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func TestTokenSignsAndCaches(t *testing.T) {
	t.Parallel()

	key := rsaKey(t)
	now := time.Date(2026, 11, 9, 18, 0, 0, 0, time.UTC)
	var calls int32
	var gotTimestamp string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/public/auth/" || r.Method != http.MethodPost {
			t.Errorf("неожиданный запрос %s %s", r.Method, r.URL.Path)
		}
		atomic.AddInt32(&calls, 1)

		var req struct {
			KeyID     string `json:"keyId"`
			Timestamp string `json:"timestamp"`
			Signature string `json:"signature"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode auth: %v", err)
		}
		gotTimestamp = req.Timestamp
		if req.KeyID != "key-1" {
			t.Errorf("keyId = %q", req.KeyID)
		}
		if _, err := time.Parse("2006-01-02T15:04:05.000-07:00", req.Timestamp); err != nil {
			t.Errorf("timestamp %q: %v", req.Timestamp, err)
		}

		signature, err := base64.StdEncoding.DecodeString(req.Signature)
		if err != nil {
			t.Fatalf("signature base64: %v", err)
		}
		sum := sha512.Sum512([]byte(req.KeyID + req.Timestamp))
		if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA512, sum[:], signature); err != nil {
			t.Fatalf("подпись неверна: %v", err)
		}

		writeJSON(w, http.StatusOK,
			`{"code":"OK","body":{"jwe":"token-1","ttl":900},"timestamp":"2026-11-09T18:00:00Z"}`)
	}))
	t.Cleanup(server.Close)

	client := mustClient(t, server.URL, func() time.Time { return now }, key)

	token, err := client.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if token != "token-1" {
		t.Errorf("token = %q", token)
	}

	if _, err := client.Token(context.Background()); err != nil {
		t.Fatalf("повторный Token: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("запросов авторизации = %d, want 1", got)
	}

	// Запас 60 секунд: за 800 секунд токен ещё действует.
	now = now.Add(800 * time.Second)
	if _, err := client.Token(context.Background()); err != nil {
		t.Fatalf("Token после сдвига: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("токен обновлён слишком рано: запросов %d", got)
	}

	// За 61 секунду до истечения токен обновляется.
	now = now.Add(61 * time.Second)
	if _, err := client.Token(context.Background()); err != nil {
		t.Fatalf("Token при истечении: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("запросов авторизации = %d, want 2", got)
	}
	if gotTimestamp == "" {
		t.Error("timestamp не передан")
	}
}

func TestTokenRejectsShortTTL(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, `{"code":"OK","body":{"jwe":"token","ttl":60}}`)
	}))
	t.Cleanup(server.Close)

	client := mustClient(t, server.URL, time.Now, rsaKey(t))
	_, err := client.Token(context.Background())
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
	if kind := exitcode.KindOf(err); kind != exitcode.KindAuthFailed {
		t.Errorf("KindOf = %q, want %q", kind, exitcode.KindAuthFailed)
	}
}

func TestTokenRetriesTransientFailure(t *testing.T) {
	t.Parallel()

	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			writeJSON(w, http.StatusServiceUnavailable, `{"code":"error","message":"временно"}`)
			return
		}
		writeJSON(w, http.StatusOK, `{"code":"OK","body":{"jwe":"token-2","ttl":900}}`)
	}))
	t.Cleanup(server.Close)

	client := mustClient(t, server.URL, time.Now, rsaKey(t))
	token, err := client.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if token != "token-2" || calls != 2 {
		t.Errorf("token = %q, calls = %d", token, calls)
	}
}

func TestTokenAuthErrorKeepsAPIMessage(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusBadRequest,
			`{"code":"error","message":"Range timestamp not valid"}`)
	}))
	t.Cleanup(server.Close)

	client := mustClient(t, server.URL, time.Now, rsaKey(t))
	_, err := client.Token(context.Background())
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
	if kind := exitcode.KindOf(err); kind != exitcode.KindAuthFailed {
		t.Errorf("KindOf = %q", kind)
	}
	if !strings.Contains(err.Error(), "Range timestamp not valid") {
		t.Errorf("err = %q", err)
	}
}

func TestListVersionsPaginates(t *testing.T) {
	t.Parallel()

	var calls int32
	server, client := clientBase(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.Header.Get("Public-Token") != "token-1" {
			t.Errorf("Public-Token = %q", r.Header.Get("Public-Token"))
		}
		if r.URL.Path != "/public/v1/application/com.example.app/version" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("filterTestingType"); got != "ALL" {
			t.Errorf("filterTestingType = %q", got)
		}
		if got := r.URL.Query().Get("size"); got != "100" {
			t.Errorf("size = %q", got)
		}

		switch r.URL.Query().Get("page") {
		case "0":
			writeJSON(w, http.StatusOK, `{"code":"OK","body":{
				"content":[
					{"versionId":1,"appName":"App","appType":"MAIN","versionName":"1.0","versionCode":10,"versionStatus":"ACTIVE","publishType":"MANUAL","testingType":null,"partialValue":100},
					{"versionId":2,"appName":"App","appType":"GAME","versionName":"1.1","versionCode":11,"versionStatus":"DRAFT","publishType":"MANUAL","testingType":"alpha"}
				],"pageNumber":0,"pageSize":2,"totalElements":3,"totalPages":2}}`)
		case "1":
			writeJSON(w, http.StatusOK, `{"code":"OK","body":{
				"content":[
					{"versionId":3,"appName":"App","appType":"GAMES","versionName":"1.2","versionCode":12,"versionStatus":"ARCHIVED","publishType":"INSTANTLY"}
				],"pageNumber":1,"pageSize":2,"totalElements":3,"totalPages":2}}`)
		default:
			t.Errorf("неожиданная страница %q", r.URL.Query().Get("page"))
		}
	}, new(int32))
	t.Cleanup(server.Close)

	versions, err := client.ListVersions(context.Background(), "com.example.app", rustore.ListOptions{})
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 3 {
		t.Fatalf("versions = %d, want 3", len(versions))
	}
	if calls != 2 {
		t.Errorf("запросов = %d, want 2", calls)
	}

	if versions[0].Testing.Kind != rustore.TestingRelease || !versions[0].Testing.Present {
		t.Errorf("null testingType = %+v", versions[0].Testing)
	}
	if versions[1].Testing.Kind != rustore.TestingAlpha {
		t.Errorf("alpha testingType = %+v", versions[1].Testing)
	}
	if versions[2].Testing.Kind != rustore.TestingUnknown || versions[2].Testing.Present {
		t.Errorf("отсутствующий testingType = %+v", versions[2].Testing)
	}
	if versions[1].Testing.Raw == nil || *versions[1].Testing.Raw != "alpha" {
		t.Errorf("raw = %v", versions[1].Testing.Raw)
	}
	if !versions[1].IsGames() || !versions[2].IsGames() || versions[0].IsGames() {
		t.Error("appType GAME/GAMES распознан неверно")
	}
}

func TestListVersionsByIDs(t *testing.T) {
	t.Parallel()

	server, client := clientBase(t, func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if got := query.Get("ids"); got != "7,9" {
			t.Errorf("ids = %q", got)
		}
		if query.Get("page") != "" || query.Get("size") != "" {
			t.Errorf("ids не должны сочетаться с пагинацией: %v", query)
		}
		writeJSON(w, http.StatusOK, `{"code":"OK","body":{"content":[
			{"versionId":7,"versionCode":1,"versionStatus":"ACTIVE","testingType":null}
		],"pageNumber":0,"pageSize":1,"totalElements":1,"totalPages":1}}`)
	}, new(int32))
	t.Cleanup(server.Close)

	versions, err := client.ListVersions(context.Background(), "com.example.app",
		rustore.ListOptions{IDs: []int64{7, 9}})
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 1 || versions[0].VersionID != 7 {
		t.Errorf("versions = %+v", versions)
	}
}

func TestListVersionsRetriesTransient(t *testing.T) {
	t.Parallel()

	var calls int32
	server, client := clientBase(t, func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Header().Set("Retry-After", "0")
			writeJSON(w, http.StatusTooManyRequests, `{"code":"error","message":"лимит"}`)
			return
		}
		writeJSON(w, http.StatusOK, `{"code":"OK","body":{"content":[],"totalPages":1}}`)
	}, new(int32))
	t.Cleanup(server.Close)

	if _, err := client.ListVersions(context.Background(), "com.example.app", rustore.ListOptions{}); err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if calls != 2 {
		t.Errorf("запросов = %d, want 2", calls)
	}
}

func TestListVersionsErrorMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status int
		body   string
		want   exitcode.Kind
	}{
		{name: "403", status: http.StatusForbidden, body: `{"code":"error","message":"нет доступа"}`, want: exitcode.KindAccessDenied},
		{name: "404", status: http.StatusNotFound, body: `{"code":"error","message":"not found"}`, want: exitcode.KindAccessDenied},
		{name: "400", status: http.StatusBadRequest, body: `{"code":"error","message":"bad package"}`, want: exitcode.KindInvalidArguments},
		{name: "битый JSON", status: http.StatusOK, body: `не json`, want: exitcode.KindAPIResponseInvalid},
		{name: "code error", status: http.StatusOK, body: `{"code":"error","message":"сбой"}`, want: exitcode.KindAPIRejected},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			server, client := clientBase(t, func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(w, test.status, test.body)
			}, new(int32))
			defer server.Close()

			_, err := client.ListVersions(context.Background(), "com.example.app", rustore.ListOptions{})
			if err == nil {
				t.Fatal("ожидалась ошибка")
			}
			if kind := exitcode.KindOf(err); kind != test.want {
				t.Errorf("KindOf = %q, want %q", kind, test.want)
			}
		})
	}
}

func TestTokenRefreshedOn401(t *testing.T) {
	t.Parallel()

	var authCalls, apiCalls int32
	mux := http.NewServeMux()
	mux.HandleFunc("/public/auth/", func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&authCalls, 1)
		writeJSON(w, http.StatusOK,
			fmt.Sprintf(`{"code":"OK","body":{"jwe":"token-%d","ttl":900}}`, n))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&apiCalls, 1)
		if n == 1 {
			if got := r.Header.Get("Public-Token"); got != "token-1" {
				t.Errorf("первый запрос с токеном %q", got)
			}
			writeJSON(w, http.StatusUnauthorized, `{"code":"error","message":"token expired"}`)
			return
		}
		if got := r.Header.Get("Public-Token"); got != "token-2" {
			t.Errorf("повтор с токеном %q", got)
		}
		writeJSON(w, http.StatusOK, `{"code":"OK","body":{"content":[],"totalPages":1}}`)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	client := mustClient(t, server.URL, time.Now, rsaKey(t))
	if _, err := client.ListVersions(context.Background(), "com.example.app", rustore.ListOptions{}); err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if authCalls != 2 || apiCalls != 2 {
		t.Errorf("authCalls = %d, apiCalls = %d", authCalls, apiCalls)
	}
}

func TestVersionDetail(t *testing.T) {
	t.Parallel()

	server, client := clientBase(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/public/v2/application/com.example.app/version/42" {
			t.Errorf("path = %q", r.URL.Path)
		}
		writeJSON(w, http.StatusOK, `{"code":"OK","body":{
			"versionId":42,
			"versionStatus":"DRAFT",
			"statusComment":"нужны декларации",
			"publishType":"MANUAL",
			"partialValue":100,
			"whatsNew":"Изменения",
			"testingType":"PRODUCTION",
			"permissions":{"location":[{"permission":"android.permission.ACCESS_FINE_LOCATION"}]},
			"packages":[
				{"verCode":42,"isMainApk":true,"servicesType":"Unknown","autoDetectedSdks":{"ads":["com.example.ads"]}}
			]}}`)
	}, new(int32))
	t.Cleanup(server.Close)

	detail, err := client.VersionDetail(context.Background(), "com.example.app", 42)
	if err != nil {
		t.Fatalf("VersionDetail: %v", err)
	}
	if detail.VersionID != 42 || detail.Status != "DRAFT" {
		t.Errorf("detail = %+v", detail)
	}
	if detail.StatusComment == nil || *detail.StatusComment != "нужны декларации" {
		t.Errorf("StatusComment = %v", detail.StatusComment)
	}
	if detail.Testing.Kind != rustore.TestingUnknown || detail.Testing.Raw == nil || *detail.Testing.Raw != "PRODUCTION" {
		t.Errorf("Testing = %+v", detail.Testing)
	}
	if len(detail.Packages) != 1 || !detail.Packages[0].IsMainApk || detail.Packages[0].VerCode != 42 {
		t.Errorf("Packages = %+v", detail.Packages)
	}
	if len(detail.Permissions) == 0 || len(detail.Packages[0].AutoDetectedSDKs) == 0 {
		t.Error("permissions или автоопределённые SDK потеряны")
	}
}

func TestVersionDetailNotFound(t *testing.T) {
	t.Parallel()

	server, client := clientBase(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotFound, `{"code":"error","message":"version not found"}`)
	}, new(int32))
	t.Cleanup(server.Close)

	_, err := client.VersionDetail(context.Background(), "com.example.app", 42)
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
	if kind := exitcode.KindOf(err); kind != exitcode.KindVersionNotFound {
		t.Errorf("KindOf = %q", kind)
	}
}
