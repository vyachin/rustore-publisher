package rustore_test

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyachin/rustore-publisher/internal/exitcode"
	"github.com/vyachin/rustore-publisher/internal/rustore"
)

func TestListScreenshots(t *testing.T) {
	t.Parallel()

	server, client := clientBase(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s", r.Method)
		}
		if r.URL.Path != "/public/v2/application/com.example.app/version/42/image/screenshot" {
			t.Errorf("path = %q", r.URL.Path)
		}
		switch r.URL.Query().Get("page") {
		case "0":
			writeJSON(w, http.StatusOK, `{"code":"OK","body":{
				"content":[
					{"id":1,"ordinal":0,"type":"SCREENSHOT","orientation":"PORTRAIT","fileUrl":"u1","fileUid":"f1"},
					{"id":2,"ordinal":1,"type":"SCREENSHOT","orientation":"PORTRAIT","fileUrl":"u2","fileUid":"f2"}
				],"pageNumber":0,"pageSize":2,"totalElements":3,"totalPages":2}}`)
		case "1":
			writeJSON(w, http.StatusOK, `{"code":"OK","body":{
				"content":[
					{"id":3,"ordinal":2,"type":"TABLET_SCREENSHOT","orientation":"LANDSCAPE","fileUrl":"u3","fileUid":"f3"}
				],"pageNumber":1,"pageSize":2,"totalElements":3,"totalPages":2}}`)
		default:
			t.Errorf("page = %q", r.URL.Query().Get("page"))
		}
	}, new(int32))
	t.Cleanup(server.Close)

	screenshots, err := client.ListScreenshots(t.Context(), "com.example.app", 42)
	if err != nil {
		t.Fatalf("ListScreenshots: %v", err)
	}
	if len(screenshots) != 3 {
		t.Fatalf("screenshots = %+v", screenshots)
	}
	if screenshots[0].ID != 1 || screenshots[0].Type != rustore.ScreenshotPhone ||
		screenshots[0].Orientation != rustore.OrientationPortrait {
		t.Errorf("first = %+v", screenshots[0])
	}
	if screenshots[2].Type != rustore.ScreenshotTablet ||
		screenshots[2].Orientation != rustore.OrientationLandscape {
		t.Errorf("third = %+v", screenshots[2])
	}
}

func TestUploadScreenshot(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "01.png")
	if err := os.WriteFile(path, []byte("screen"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	server, client := clientBase(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if r.URL.Path != "/public/v2/application/com.example.app/version/42/image/screenshot/PORTRAIT/0/SCREENSHOT" {
			t.Errorf("path = %q", r.URL.Path)
		}
		reader, err := r.MultipartReader()
		if err != nil {
			t.Fatalf("multipart: %v", err)
		}
		part, err := reader.NextPart()
		if err != nil {
			t.Fatalf("part: %v", err)
		}
		if part.FormName() != "file" {
			t.Errorf("field = %q", part.FormName())
		}
		if contentType := part.Header.Get("Content-Type"); contentType != "image/png" {
			t.Errorf("Content-Type = %q", contentType)
		}
		if _, err := io.Copy(io.Discard, part); err != nil {
			t.Fatalf("read: %v", err)
		}
		writeJSON(w, http.StatusOK, `{"code":"OK"}`)
	}, new(int32))
	t.Cleanup(server.Close)

	if err := client.UploadScreenshot(t.Context(), "com.example.app", 42,
		rustore.ScreenshotPhone, rustore.OrientationPortrait, 0, path); err != nil {
		t.Fatalf("UploadScreenshot: %v", err)
	}
}

func TestDeleteScreenshot(t *testing.T) {
	t.Parallel()

	server, client := clientBase(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s", r.Method)
		}
		if r.URL.Path != "/public/v2/application/com.example.app/version/42/image/screenshot/7" {
			t.Errorf("path = %q", r.URL.Path)
		}
		writeJSON(w, http.StatusOK, `{"code":"OK"}`)
	}, new(int32))
	t.Cleanup(server.Close)

	if err := client.DeleteScreenshot(t.Context(), "com.example.app", 42, 7); err != nil {
		t.Fatalf("DeleteScreenshot: %v", err)
	}
}

func TestDeleteScreenshotNotFoundIsSuccess(t *testing.T) {
	t.Parallel()

	server, client := clientBase(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotFound, `{"code":"error","message":"not found"}`)
	}, new(int32))
	t.Cleanup(server.Close)

	if err := client.DeleteScreenshot(t.Context(), "com.example.app", 42, 7); err != nil {
		t.Fatalf("DeleteScreenshot: %v", err)
	}
}

func TestUploadScreenshotRejectsInvalidArguments(t *testing.T) {
	t.Parallel()

	server, client := clientBase(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("запрос не должен выполняться")
	}, new(int32))
	t.Cleanup(server.Close)

	tests := []struct {
		name        string
		typ         rustore.ScreenshotType
		orientation rustore.ScreenshotOrientation
		ordinal     int
	}{
		{name: "тип", typ: "UNKNOWN", orientation: rustore.OrientationPortrait, ordinal: 0},
		{name: "ориентация", typ: rustore.ScreenshotPhone, orientation: "SIDEWAYS", ordinal: 0},
		{name: "ordinal", typ: rustore.ScreenshotPhone, orientation: rustore.OrientationPortrait, ordinal: 10},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := client.UploadScreenshot(t.Context(), "com.example.app", 42,
				test.typ, test.orientation, test.ordinal, "/tmp/x.png")
			if err == nil {
				t.Fatal("ожидалась ошибка")
			}
			if kind := exitcode.KindOf(err); kind != exitcode.KindInvalidArguments {
				t.Errorf("KindOf = %q", kind)
			}
			if !strings.Contains(err.Error(), "скриншота") {
				t.Errorf("err = %q", err)
			}
		})
	}
}
