package rustore_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/vyachin/rustore-publisher/internal/exitcode"
	"github.com/vyachin/rustore-publisher/internal/rustore"
)

func TestCreateDraft(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want int64
	}{
		{name: "число", body: `{"code":"OK","body":243242}`, want: 243242},
		{name: "объект versionId", body: `{"code":"OK","body":{"versionId":17}}`, want: 17},
		{name: "объект content", body: `{"code":"OK","body":{"content":{"versionId":19}}}`, want: 19},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			server, client := clientBase(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/public/v1/application/com.example.app/version" {
					t.Errorf("запрос %s %s", r.Method, r.URL.Path)
				}
				var request struct {
					PublishType  string  `json:"publishType"`
					PartialValue int     `json:"partialValue"`
					WhatsNew     *string `json:"whatsNew"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if request.PublishType != "MANUAL" || request.PartialValue != 100 {
					t.Errorf("request = %+v", request)
				}
				if request.WhatsNew == nil || *request.WhatsNew != "Изменения" {
					t.Errorf("whatsNew = %v", request.WhatsNew)
				}
				writeJSON(w, http.StatusOK, test.body)
			}, new(int32))
			t.Cleanup(server.Close)

			whatsNew := "Изменения"
			id, err := client.CreateDraft(context.Background(), "com.example.app", rustore.CreateDraftRequest{
				PublishType:  "MANUAL",
				PartialValue: 100,
				WhatsNew:     &whatsNew,
			})
			if err != nil {
				t.Fatalf("CreateDraft: %v", err)
			}
			if id != test.want {
				t.Errorf("id = %d, want %d", id, test.want)
			}
		})
	}
}

func TestCreateDraftInvalidResponse(t *testing.T) {
	t.Parallel()

	server, client := clientBase(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, `{"code":"OK","body":"строка"}`)
	}, new(int32))
	t.Cleanup(server.Close)

	_, err := client.CreateDraft(context.Background(), "com.example.app",
		rustore.CreateDraftRequest{PublishType: "MANUAL", PartialValue: 100})
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
	if kind := exitcode.KindOf(err); kind != exitcode.KindAPIResponseInvalid {
		t.Errorf("KindOf = %q", kind)
	}
}

func TestCreateDraftConflict(t *testing.T) {
	t.Parallel()

	server, client := clientBase(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusConflict, `{"code":"error","message":"draft exists"}`)
	}, new(int32))
	t.Cleanup(server.Close)

	_, err := client.CreateDraft(context.Background(), "com.example.app",
		rustore.CreateDraftRequest{PublishType: "MANUAL", PartialValue: 100})
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
	if kind := exitcode.KindOf(err); kind != exitcode.KindExistingDraft {
		t.Errorf("KindOf = %q", kind)
	}
}

func TestCommit(t *testing.T) {
	t.Parallel()

	server, client := clientBase(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if r.URL.Path != "/public/v1/application/com.example.app/version/42/commit" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("priorityUpdate"); got != "0" {
			t.Errorf("priorityUpdate = %q", got)
		}
		if r.Header.Get("Public-Token") != "token-1" {
			t.Errorf("Public-Token = %q", r.Header.Get("Public-Token"))
		}
		writeJSON(w, http.StatusOK, `{"code":"OK","message":null}`)
	}, new(int32))
	t.Cleanup(server.Close)

	if err := client.Commit(context.Background(), "com.example.app", 42); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

func TestUploadAAB(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "app.aab")
	content := bytes.Repeat([]byte("aab-data"), 1000)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write AAB: %v", err)
	}

	var received string
	server, client := clientBase(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/public/v1/application/com.example.app/version/42/aab" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Public-Token") != "token-1" {
			t.Errorf("Public-Token = %q", r.Header.Get("Public-Token"))
		}
		if r.ContentLength <= 0 {
			t.Errorf("ContentLength = %d", r.ContentLength)
		}

		reader, err := r.MultipartReader()
		if err != nil {
			t.Fatalf("multipart: %v", err)
		}
		part, err := reader.NextPart()
		if err != nil {
			t.Fatalf("next part: %v", err)
		}
		if part.FormName() != "file" {
			t.Errorf("field = %q", part.FormName())
		}
		data, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("read part: %v", err)
		}
		received = string(data)

		writeJSON(w, http.StatusOK, `{"code":"OK","body":{
			"permissions":{"location":[{"permission":"android.permission.ACCESS_FINE_LOCATION"}]},
			"autoDetectedSdks":{"ads":["com.example.ads"]}}}`)
	}, new(int32))
	t.Cleanup(server.Close)

	result, err := client.UploadAAB(context.Background(), "com.example.app", 42, path)
	if err != nil {
		t.Fatalf("UploadAAB: %v", err)
	}
	if received != string(content) {
		t.Errorf("загружено %d байт, ожидалось %d", len(received), len(content))
	}
	if len(result.Permissions) == 0 || len(result.AutoDetectedSDKs) == 0 {
		t.Errorf("result = %+v", result)
	}
	if !strings.Contains(string(result.AutoDetectedSDKs), "com.example.ads") {
		t.Errorf("SDK = %s", result.AutoDetectedSDKs)
	}
}

func TestUploadAABServerError(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "app.aab")
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	server, client := clientBase(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		writeJSON(w, http.StatusInternalServerError, `{"code":"error","message":"сбой"}`)
	}, new(int32))
	t.Cleanup(server.Close)

	_, err := client.UploadAAB(context.Background(), "com.example.app", 42, path)
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
	if kind := exitcode.KindOf(err); kind != exitcode.KindOutcomeUnknown {
		t.Errorf("KindOf = %q", kind)
	}
}

func TestCreateDraftIsNotRetried(t *testing.T) {
	t.Parallel()

	var calls int32
	server, client := clientBase(t, func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		writeJSON(w, http.StatusServiceUnavailable, `{"code":"error","message":"сбой"}`)
	}, new(int32))
	t.Cleanup(server.Close)

	_, err := client.CreateDraft(context.Background(), "com.example.app",
		rustore.CreateDraftRequest{PublishType: "MANUAL", PartialValue: 100})
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
	if calls != 1 {
		t.Errorf("запросов = %d, want 1", calls)
	}
	if kind := exitcode.KindOf(err); kind != exitcode.KindOutcomeUnknown {
		t.Errorf("KindOf = %q", kind)
	}
}

func TestCreateDraftValidationHint(t *testing.T) {
	t.Parallel()

	server, client := clientBase(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusBadRequest,
			`{"code":"error","message":"Developer contacts are not filled in"}`)
	}, new(int32))
	t.Cleanup(server.Close)

	_, err := client.CreateDraft(context.Background(), "com.example.app",
		rustore.CreateDraftRequest{PublishType: "MANUAL", PartialValue: 100})
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
	if kind := exitcode.KindOf(err); kind != exitcode.KindAPIRejected {
		t.Errorf("KindOf = %q", kind)
	}
	if hint := exitcode.HintOf(err); !strings.Contains(hint, "консоли") {
		t.Errorf("hint = %q", hint)
	}
}
