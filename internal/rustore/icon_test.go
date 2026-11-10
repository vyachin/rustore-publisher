package rustore_test

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyachin/rustore-publisher/internal/exitcode"
)

func TestUploadIcon(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "icon.png")
	content := []byte("icon-content")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write icon: %v", err)
	}

	var received string
	server, client := clientBase(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if r.URL.Path != "/public/v1/application/com.example.app/version/42/image/icon" {
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
		if part.FileName() != "icon.png" {
			t.Errorf("filename = %q", part.FileName())
		}
		if contentType := part.Header.Get("Content-Type"); contentType != "image/png" {
			t.Errorf("Content-Type = %q", contentType)
		}
		data, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("read part: %v", err)
		}
		received = string(data)
		writeJSON(w, http.StatusOK, `{"code":"OK"}`)
	}, new(int32))
	t.Cleanup(server.Close)

	if err := client.UploadIcon(t.Context(), "com.example.app", 42, path); err != nil {
		t.Fatalf("UploadIcon: %v", err)
	}
	if received != string(content) {
		t.Errorf("получено %q, ожидалось %q", received, content)
	}
}

func TestUploadIconTooLarge(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "icon.png")
	if err := os.WriteFile(path, make([]byte, 3<<20+1), 0o600); err != nil {
		t.Fatalf("write icon: %v", err)
	}

	server, client := clientBase(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("запрос не должен выполняться")
	}, new(int32))
	t.Cleanup(server.Close)

	err := client.UploadIcon(t.Context(), "com.example.app", 42, path)
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
	if kind := exitcode.KindOf(err); kind != exitcode.KindInvalidMetadata {
		t.Errorf("KindOf = %q", kind)
	}
}

func TestUploadIconAPIRejected(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "icon.png")
	if err := os.WriteFile(path, []byte("icon"), 0o600); err != nil {
		t.Fatalf("write icon: %v", err)
	}

	server, client := clientBase(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusBadRequest, `{"code":"error","message":"bad image"}`)
	}, new(int32))
	t.Cleanup(server.Close)

	err := client.UploadIcon(t.Context(), "com.example.app", 42, path)
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
	if kind := exitcode.KindOf(err); kind != exitcode.KindAPIRejected {
		t.Errorf("KindOf = %q", kind)
	}
	if hint := exitcode.HintOf(err); !strings.Contains(hint, "формат иконки") {
		t.Errorf("hint = %q", hint)
	}
}

func TestUploadIconServerError(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "icon.png")
	if err := os.WriteFile(path, []byte("icon"), 0o600); err != nil {
		t.Fatalf("write icon: %v", err)
	}

	server, client := clientBase(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusInternalServerError, `{"code":"error","message":"сбой"}`)
	}, new(int32))
	t.Cleanup(server.Close)

	err := client.UploadIcon(t.Context(), "com.example.app", 42, path)
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
	if kind := exitcode.KindOf(err); kind != exitcode.KindOutcomeUnknown {
		t.Errorf("KindOf = %q", kind)
	}
}
