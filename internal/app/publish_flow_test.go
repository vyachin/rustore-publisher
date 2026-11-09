package app_test

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vyachin/rustore-publisher/internal/app"
	"github.com/vyachin/rustore-publisher/internal/artifact"
	"github.com/vyachin/rustore-publisher/internal/exitcode"
	"github.com/vyachin/rustore-publisher/internal/rustore"
)

// flowStore — сценарий хранилища для проверки повторов publish.
type flowStore struct {
	t *testing.T

	versionID     int64
	packagesReady bool
	status        string
	sendDate      *string

	commitFailures int
	uploadStatus   int

	createCalls int
	uploadCalls int
	commitCalls int
}

func (s *flowStore) handler(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/public/auth/":
		writeFlowJSON(w, http.StatusOK, `{"code":"OK","body":{"jwe":"token","ttl":900}}`)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/version"):
		s.createCalls++
		s.versionID = 100
		s.status = "DRAFT"
		writeFlowJSON(w, http.StatusOK, `{"code":"OK","body":100}`)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/aab"):
		s.uploadCalls++
		_, _ = io.Copy(io.Discard, r.Body)
		if s.uploadStatus >= 400 {
			writeFlowJSON(w, s.uploadStatus, `{"code":"error","message":"сбой"}`)
			return
		}
		writeFlowJSON(w, http.StatusOK, `{"code":"OK","body":{"permissions":{}}}`)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/commit"):
		s.commitCalls++
		if s.commitFailures > 0 {
			s.commitFailures--
			writeFlowJSON(w, http.StatusBadRequest,
				`{"code":"error","message":"Packages for version with id = 100 Is not found"}`)
			return
		}
		if s.uploadStatus == http.StatusInternalServerError {
			writeFlowJSON(w, http.StatusInternalServerError, `{"code":"error","message":"сбой"}`)
			return
		}
		s.status = "TAKEN_FOR_MODERATION"
		date := "2026-11-09T18:05:00Z"
		s.sendDate = &date
		writeFlowJSON(w, http.StatusOK, `{"code":"OK"}`)
	case strings.Contains(r.URL.Path, "/public/v2/"):
		s.detail(w)
	case strings.Contains(r.URL.Path, "/public/v1/"):
		s.list(w)
	default:
		s.t.Errorf("неожиданный запрос %s %s", r.Method, r.URL.Path)
	}
}

func (s *flowStore) detail(w http.ResponseWriter) {
	packages := "[]"
	if s.packagesReady {
		packages = `[{"verCode":42,"isMainApk":true,"servicesType":"Unknown"}]`
	}
	writeFlowJSON(w, http.StatusOK, fmt.Sprintf(`{"code":"OK","body":{
		"versionId":%d,"versionStatus":"%s","publishType":"MANUAL",
		"partialValue":100,"whatsNew":"Заметки","packages":%s}}`,
		s.versionID, s.status, packages))
}

func (s *flowStore) list(w http.ResponseWriter) {
	list := []json.RawMessage{
		json.RawMessage(`{"versionId":1,"versionCode":10,"versionStatus":"ACTIVE","publishType":"MANUAL","testingType":null}`),
	}
	if s.versionID > 0 {
		date := ""
		if s.sendDate != nil {
			date = fmt.Sprintf(`,"sendDateForModer":%q`, *s.sendDate)
		}
		list = append(list, json.RawMessage(fmt.Sprintf(
			`{"versionId":%d,"versionCode":42,"versionStatus":%q,"publishType":"MANUAL","testingType":null%s}`,
			s.versionID, s.status, date)))
	}
	body, err := json.Marshal(list)
	if err != nil {
		s.t.Fatalf("marshal list: %v", err)
	}
	writeFlowJSON(w, http.StatusOK, fmt.Sprintf(
		`{"code":"OK","body":{"content":%s,"pageNumber":0,"pageSize":100,"totalElements":%d,"totalPages":1}}`,
		body, len(list)))
}

func writeFlowJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func newFlowClient(t *testing.T, baseURL string, key *rsa.PrivateKey) *rustore.Client {
	t.Helper()
	client, err := rustore.NewClient(rustore.Config{
		BaseURL:     baseURL,
		Credentials: rustore.Credentials{KeyID: "key-1", Key: key},
		Sleep:       func(context.Context, time.Duration) error { return nil },
		RetryBase:   time.Millisecond,
		MaxAttempts: 3,
	})
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	return client
}

// newBundle создаёт файл AAB и минимальный Bundle для upload.
func newBundle(t *testing.T) *artifact.Bundle {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.aab")
	if err := os.WriteFile(path, []byte("aab-content"), 0o600); err != nil {
		t.Fatalf("write AAB: %v", err)
	}
	return &artifact.Bundle{
		Info:     artifact.Info{Path: path},
		Identity: artifact.Identity{Package: "com.example.app", VersionCode: 42},
	}
}

func publishInput(t *testing.T, bundle *artifact.Bundle, changelog *string) app.PublishInput {
	t.Helper()
	return app.PublishInput{
		Package:           "com.example.app",
		Bundle:            bundle,
		Changelog:         changelog,
		PublishType:       "MANUAL",
		IfExists:          "fail",
		ProcessingTimeout: 200 * time.Millisecond,
		PollInterval:      time.Millisecond,
		Sleep:             func(context.Context, time.Duration) error { return nil },
	}
}

func TestPublishRetriesCommitAfterPackagesNotFound(t *testing.T) {
	t.Parallel()

	store := &flowStore{packagesReady: true, commitFailures: 1}
	server := httptest.NewServer(http.HandlerFunc(store.handler))
	t.Cleanup(server.Close)

	bundle := newBundle(t)
	changelog := "Заметки"
	report, err := app.Publish(context.Background(), newFlowClient(t, server.URL, rsaKey(t)),
		publishInput(t, bundle, &changelog), nil)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if report.Outcome != app.OutcomeSubmitted {
		t.Errorf("outcome = %q", report.Outcome)
	}
	if store.commitCalls != 2 {
		t.Errorf("commitCalls = %d, want 2", store.commitCalls)
	}
	if store.uploadCalls != 1 || store.createCalls != 1 {
		t.Errorf("create/upload = %d/%d", store.createCalls, store.uploadCalls)
	}
}

func TestPublishRecoversUploadAfterServerError(t *testing.T) {
	t.Parallel()

	store := &flowStore{packagesReady: true}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/aab") {
			// Сервер прочитал файл, но ответил ошибкой; состояние показывает,
			// что загрузка применилась.
			store.uploadCalls++
			_, _ = io.Copy(io.Discard, r.Body)
			writeFlowJSON(w, http.StatusInternalServerError, `{"code":"error","message":"сбой"}`)
			return
		}
		store.handler(w, r)
	}))
	t.Cleanup(server.Close)

	bundle := newBundle(t)
	changelog := "Заметки"
	report, err := app.Publish(context.Background(), newFlowClient(t, server.URL, rsaKey(t)),
		publishInput(t, bundle, &changelog), nil)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if report.Outcome != app.OutcomeSubmitted {
		t.Errorf("outcome = %q", report.Outcome)
	}
	if store.uploadCalls != 1 {
		t.Errorf("uploadCalls = %d", store.uploadCalls)
	}
}

func TestPublishCommitUnknownOutcome(t *testing.T) {
	t.Parallel()

	store := &flowStore{packagesReady: true}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/commit") {
			store.commitCalls++
			writeFlowJSON(w, http.StatusInternalServerError, `{"code":"error","message":"сбой"}`)
			return
		}
		store.handler(w, r)
	}))
	t.Cleanup(server.Close)

	bundle := newBundle(t)
	changelog := "Заметки"
	report, err := app.Publish(context.Background(), newFlowClient(t, server.URL, rsaKey(t)),
		publishInput(t, bundle, &changelog), nil)
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
	if kind := exitcode.KindOf(err); kind != exitcode.KindOutcomeUnknown {
		t.Errorf("KindOf = %q", kind)
	}
	if !report.OutcomeUnknown {
		t.Error("report.OutcomeUnknown = false")
	}
	if report.VersionID != 100 {
		t.Errorf("VersionID = %d", report.VersionID)
	}
}

func TestPublishProcessingTimeout(t *testing.T) {
	t.Parallel()

	store := &flowStore{commitFailures: 1000}
	server := httptest.NewServer(http.HandlerFunc(store.handler))
	t.Cleanup(server.Close)

	bundle := newBundle(t)
	changelog := "Заметки"
	in := publishInput(t, bundle, &changelog)
	in.ProcessingTimeout = 20 * time.Millisecond
	report, err := app.Publish(context.Background(), newFlowClient(t, server.URL, rsaKey(t)), in, nil)
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
	if kind := exitcode.KindOf(err); kind != exitcode.KindAABProcessingTimeout {
		t.Errorf("KindOf = %q", kind)
	}
	if report.OutcomeUnknown {
		t.Error("таймаут обработки не должен помечать результат неизвестным")
	}
}
