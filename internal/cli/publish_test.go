package cli_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// publishStore — подменный RuStore для сценариев publish.
type publishStore struct {
	t *testing.T

	active        map[string]any
	versions      []map[string]any
	created       map[string]any
	versionID     int64
	uploaded      bool
	status        string
	sendDate      *string
	publishType   string
	partialValue  int
	whatsNew      string
	contactsEmail string
	mismatch      bool

	appName          *string
	shortDescription *string
	fullDescription  *string
	mismatchListing  bool
	icon             []byte
	iconUID          string
	iconNotStored    bool

	screenshots         []storeScreenshot
	screenshotUploads   int
	screenshotDeletes   int
	screenshotNotStored bool

	createCalls int
	iconCalls   int
	uploadCalls int
	commitCalls int
}

type storeScreenshot struct {
	ID          int64  `json:"id"`
	Ordinal     int    `json:"ordinal"`
	Type        string `json:"type"`
	Orientation string `json:"orientation"`
}

func newPublishStore(t *testing.T, activeCode int64) *publishStore {
	t.Helper()
	active := map[string]any{
		"versionId":     1,
		"versionCode":   activeCode,
		"versionStatus": "ACTIVE",
		"publishType":   "MANUAL",
		"testingType":   nil,
		"partialValue":  100,
	}
	return &publishStore{
		t:            t,
		active:       active,
		versions:     []map[string]any{active},
		partialValue: 100,
		publishType:  "MANUAL",
	}
}

func (s *publishStore) addVersion(version map[string]any) {
	s.versions = append(s.versions, version)
}

func (s *publishStore) handler(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/public/auth/":
		writeStoreJSON(w, http.StatusOK, `{"code":"OK","body":{"jwe":"token-1","ttl":900}}`)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/image/icon"):
		s.uploadIcon(w, r)
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/image/screenshot"):
		s.listScreenshots(w)
	case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/image/screenshot/"):
		s.uploadScreenshot(w, r)
	case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/image/screenshot/"):
		s.deleteScreenshot(w)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/aab"):
		s.upload(w, r)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/commit"):
		s.commit(w, r)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/version"):
		s.create(w, r)
	case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/public/v2/"):
		s.detail(w)
	case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/public/v1/"):
		s.list(w)
	default:
		s.t.Errorf("неожиданный запрос %s %s", r.Method, r.URL.Path)
		writeStoreJSON(w, http.StatusNotFound, `{"code":"error"}`)
	}
}

func (s *publishStore) create(w http.ResponseWriter, r *http.Request) {
	s.createCalls++
	var request struct {
		AppName           *string `json:"appName"`
		ShortDescription  *string `json:"shortDescription"`
		FullDescription   *string `json:"fullDescription"`
		PublishType       string  `json:"publishType"`
		PartialValue      int     `json:"partialValue"`
		WhatsNew          *string `json:"whatsNew"`
		DeveloperContacts *struct {
			Email string `json:"email"`
		} `json:"developerContacts"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.t.Fatalf("create body: %v", err)
	}
	s.appName = request.AppName
	s.shortDescription = request.ShortDescription
	s.fullDescription = request.FullDescription
	s.publishType = request.PublishType
	s.partialValue = request.PartialValue
	if request.WhatsNew != nil {
		s.whatsNew = *request.WhatsNew
	}
	if request.DeveloperContacts != nil {
		s.contactsEmail = request.DeveloperContacts.Email
	}
	s.versionID = 100
	s.status = "DRAFT"
	if s.mismatch {
		request.PublishType = "INSTANTLY"
		request.PartialValue = 5
	}
	s.created = map[string]any{
		"versionId":     s.versionID,
		"versionCode":   int64(42),
		"versionStatus": "DRAFT",
		"publishType":   request.PublishType,
		"testingType":   nil,
		"partialValue":  request.PartialValue,
	}
	if request.WhatsNew != nil {
		s.created["whatsNew"] = *request.WhatsNew
	}
	if request.AppName != nil {
		s.created["appName"] = *request.AppName
	}
	if request.ShortDescription != nil {
		s.created["shortDescription"] = *request.ShortDescription
	}
	if request.FullDescription != nil {
		s.created["fullDescription"] = *request.FullDescription
	}
	s.versions = append(s.versions, s.created)
	writeStoreJSON(w, http.StatusOK, fmt.Sprintf(`{"code":"OK","body":%d}`, s.versionID))
}

func (s *publishStore) uploadIcon(w http.ResponseWriter, r *http.Request) {
	s.iconCalls++
	reader, err := r.MultipartReader()
	if err != nil {
		s.t.Fatalf("multipart: %v", err)
	}
	part, err := reader.NextPart()
	if err != nil {
		s.t.Fatalf("part: %v", err)
	}
	if part.FormName() != "file" {
		s.t.Errorf("field = %q", part.FormName())
	}
	data, err := io.ReadAll(part)
	if err != nil {
		s.t.Fatalf("read icon: %v", err)
	}
	s.icon = data
	s.iconUID = "icon-1"
	writeStoreJSON(w, http.StatusOK, `{"code":"OK"}`)
}

func (s *publishStore) upload(w http.ResponseWriter, r *http.Request) {
	s.uploadCalls++
	reader, err := r.MultipartReader()
	if err != nil {
		s.t.Fatalf("multipart: %v", err)
	}
	part, err := reader.NextPart()
	if err != nil {
		s.t.Fatalf("part: %v", err)
	}
	if part.FormName() != "file" {
		s.t.Errorf("field = %q", part.FormName())
	}
	if _, err := io.Copy(io.Discard, part); err != nil {
		s.t.Fatalf("read file: %v", err)
	}
	s.uploaded = true
	writeStoreJSON(w, http.StatusOK, `{"code":"OK","body":{
		"permissions":{"camera":[{"permission":"android.permission.CAMERA"}]},
		"autoDetectedSdks":{"ads":["com.example.ads"]}}}`)
}

func (s *publishStore) commit(w http.ResponseWriter, r *http.Request) {
	s.commitCalls++
	if got := r.URL.Query().Get("priorityUpdate"); got != "0" {
		s.t.Errorf("priorityUpdate = %q", got)
	}
	s.status = "TAKEN_FOR_MODERATION"
	date := "2026-11-09T18:05:00Z"
	s.sendDate = &date
	s.created["versionStatus"] = s.status
	s.created["sendDateForModer"] = date
	writeStoreJSON(w, http.StatusOK, `{"code":"OK","message":null}`)
}

func (s *publishStore) detail(w http.ResponseWriter) {
	if s.created == nil {
		writeStoreJSON(w, http.StatusNotFound, `{"code":"error","message":"not found"}`)
		return
	}
	publishType := s.publishType
	packages := `[]`
	if s.uploaded {
		packages = `[{"verCode":42,"isMainApk":true,"servicesType":"Unknown"}]`
	}
	appName := optionalJSON(s.appName)
	if s.mismatchListing && s.appName != nil {
		appName = `"Другое название"`
	}
	iconUID := "null"
	iconURL := "null"
	if s.iconUID != "" && !s.iconNotStored {
		iconUID = fmt.Sprintf("%q", s.iconUID)
		iconURL = fmt.Sprintf("%q", "https://example.com/icon.png")
	}
	writeStoreJSON(w, http.StatusOK, fmt.Sprintf(`{"code":"OK","body":{
		"versionId":%d,"versionStatus":"%s","publishType":"%s",
		"appName":%s,"shortDescription":%s,"fullDescription":%s,
		"iconUid":%s,"iconUrl":%s,
		"partialValue":-1,"whatsNew":%q,"statusComment":null,
		"permissions":null,"packages":%s}}`,
		s.versionID, s.status, publishType,
		appName, optionalJSON(s.shortDescription), optionalJSON(s.fullDescription),
		iconUID, iconURL, s.whatsNew, packages))
}

// optionalJSON кодирует необязательную строку для ответа подменного сервера.
func optionalJSON(value *string) string {
	if value == nil {
		return "null"
	}
	return fmt.Sprintf("%q", *value)
}

func (s *publishStore) list(w http.ResponseWriter) {
	items := make([]string, 0, len(s.versions))
	for _, version := range s.versions {
		data, err := json.Marshal(version)
		if err != nil {
			s.t.Fatalf("marshal version: %v", err)
		}
		items = append(items, string(data))
	}
	writeStoreJSON(w, http.StatusOK, statusListBody(items...))
}

func writeChangelog(t *testing.T, versionCode int64, text string) (string, string) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "ru-RU", "changelogs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	file := filepath.Join(dir, fmt.Sprintf("%d.txt", versionCode))
	if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
		t.Fatalf("write changelog: %v", err)
	}
	return root, text
}

func writeMetadataFile(t *testing.T, root, name, body string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func writePNGImage(t *testing.T, path string, width, height int) {
	t.Helper()

	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, width, height))); err != nil {
		t.Fatalf("encode image: %v", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write image: %v", err)
	}
}

func writePNGIcon(t *testing.T, root string) {
	t.Helper()
	writePNGImage(t, filepath.Join(root, "ru-RU", "images", "icon.png"), 512, 512)
}

func writePhoneScreenshots(t *testing.T, root string, count int) {
	t.Helper()
	for i := 1; i <= count; i++ {
		writePNGImage(t,
			filepath.Join(root, "ru-RU", "images", "phoneScreenshots", fmt.Sprintf("%02d.png", i)),
			1080, 1920)
	}
}

func TestPublishHappyPath(t *testing.T) {
	t.Parallel()

	store := newPublishStore(t, 10)
	metadata, text := writeChangelog(t, 42, "Заметки о выпуске")
	aab := writeAAB(t, "com.example.app", 42)

	code, stdout, stderr := runAPI(t, store.handler, "--json", "publish",
		"-f", aab, "--metadata-dir", metadata)
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}

	obj := decodeOne(t, stdout)
	assertKeysMatchSchema(t, "result", obj)

	if obj["outcome"] != "submitted" || obj["phase"] != "status" {
		t.Errorf("result = %v", obj)
	}
	if obj["version_id"] != float64(100) {
		t.Errorf("version_id = %v", obj["version_id"])
	}
	if obj["version_status"] != "TAKEN_FOR_MODERATION" {
		t.Errorf("version_status = %v", obj["version_status"])
	}
	if obj["submitted"] != true || obj["skipped"] != false {
		t.Errorf("факты = %v", obj)
	}
	if _, ok := obj["permissions"].(map[string]any); !ok {
		t.Errorf("permissions = %v", obj["permissions"])
	}
	if _, ok := obj["auto_detected_sdks"].(map[string]any); !ok {
		t.Errorf("auto_detected_sdks = %v", obj["auto_detected_sdks"])
	}

	if store.createCalls != 1 || store.uploadCalls != 1 || store.commitCalls != 1 {
		t.Errorf("вызовы: create=%d upload=%d commit=%d",
			store.createCalls, store.uploadCalls, store.commitCalls)
	}
	if store.publishType != "MANUAL" || store.partialValue != 100 {
		t.Errorf("настройки черновика: %s %d", store.publishType, store.partialValue)
	}
	if store.whatsNew != text {
		t.Errorf("whatsNew = %q", store.whatsNew)
	}
	if store.appName != nil || store.shortDescription != nil || store.fullDescription != nil {
		t.Errorf("без файлов тексты не должны передаваться: %v/%v/%v",
			store.appName, store.shortDescription, store.fullDescription)
	}
}

func TestPublishAcceptsPlayMetadataLayout(t *testing.T) {
	t.Parallel()

	store := newPublishStore(t, 10)
	metadata, text := writeChangelog(t, 42, "Заметки о выпуске")

	listing := map[string]string{
		"README.md":                       "документация каталога",
		"ru-RU/title.txt":                 "Приложение",
		"ru-RU/full_description.txt":      "Подробное описание",
		"ru-RU/images/featureGraphic.png": "картинка",
	}
	for name, body := range listing {
		path := filepath.Join(metadata, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	aab := writeAAB(t, "com.example.app", 42)

	code, stdout, stderr := runAPI(t, store.handler, "--json", "publish",
		"-f", aab, "--metadata-dir", metadata)
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}

	obj := decodeOne(t, stdout)
	warnings, ok := obj["warnings"].([]any)
	if !ok || len(warnings) == 0 {
		t.Fatalf("warnings = %v", obj["warnings"])
	}
	found := false
	for _, raw := range warnings {
		warning, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if warning["code"] != "METADATA_IGNORED" {
			continue
		}
		found = true
		message, _ := warning["message"].(string)
		if !strings.Contains(message, "ru-RU/images") {
			t.Errorf("message = %q", message)
		}
		if strings.Contains(message, "title.txt") {
			t.Errorf("title.txt попал в игнорируемые: %q", message)
		}
	}
	if !found {
		t.Errorf("нет предупреждения METADATA_IGNORED: %v", warnings)
	}
	if store.whatsNew != text {
		t.Errorf("whatsNew = %q", store.whatsNew)
	}
	if store.appName == nil || *store.appName != "Приложение" {
		t.Errorf("appName = %v", store.appName)
	}
	if store.fullDescription == nil || *store.fullDescription != "Подробное описание" {
		t.Errorf("fullDescription = %v", store.fullDescription)
	}
	if store.shortDescription != nil {
		t.Errorf("shortDescription = %v, want nil", store.shortDescription)
	}
}

func TestPublishSkipExisting(t *testing.T) {
	t.Parallel()

	date := "2026-01-01T00:00:00Z"
	store := newPublishStore(t, 42)
	store.versions[0]["versionCode"] = int64(42)
	store.versions[0]["sendDateForModer"] = date
	store.versions[0]["versionStatus"] = "ACTIVE"
	aab := writeAAB(t, "com.example.app", 42)

	code, stdout, stderr := runAPI(t, store.handler, "--json", "publish",
		"-f", aab, "--if-exists=skip")
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	obj := decodeOne(t, stdout)
	if obj["outcome"] != "skipped" || obj["skipped"] != true {
		t.Errorf("result = %v", obj)
	}
	if store.createCalls != 0 || store.uploadCalls != 0 || store.commitCalls != 0 {
		t.Errorf("записи при skip: %d/%d/%d",
			store.createCalls, store.uploadCalls, store.commitCalls)
	}
}

func TestPublishFailsOnExistingVersion(t *testing.T) {
	t.Parallel()

	date := "2026-01-01T00:00:00Z"
	store := newPublishStore(t, 42)
	store.versions[0]["sendDateForModer"] = date
	aab := writeAAB(t, "com.example.app", 42)

	code, stdout, _ := runAPI(t, store.handler, "--json", "publish", "-f", aab)
	if code != 5 {
		t.Fatalf("code = %d, want 5", code)
	}
	obj := decodeOne(t, stdout)
	errObj, _ := obj["error"].(map[string]any)
	if errObj["code"] != "VERSION_EXISTS" {
		t.Errorf("error = %v", obj["error"])
	}
	if store.createCalls != 0 {
		t.Errorf("create вызван %d раз", store.createCalls)
	}
}

func TestPublishExistingDraft(t *testing.T) {
	t.Parallel()

	store := newPublishStore(t, 10)
	store.addVersion(map[string]any{
		"versionId": 2, "versionCode": 41, "versionStatus": "DRAFT", "testingType": nil,
	})
	aab := writeAAB(t, "com.example.app", 42)

	code, stdout, _ := runAPI(t, store.handler, "--json", "publish", "-f", aab)
	if code != 5 {
		t.Fatalf("code = %d, want 5", code)
	}
	obj := decodeOne(t, stdout)
	errObj, _ := obj["error"].(map[string]any)
	if errObj["code"] != "EXISTING_DRAFT" {
		t.Errorf("error = %v", obj["error"])
	}
	if store.createCalls != 0 {
		t.Errorf("create вызван %d раз", store.createCalls)
	}
}

func TestPublishDryRun(t *testing.T) {
	t.Parallel()

	store := newPublishStore(t, 10)
	aab := writeAAB(t, "com.example.app", 42)

	code, stdout, stderr := runAPI(t, store.handler, "--json", "publish",
		"-f", aab, "--dry-run", "--metadata-dir", mustMetadata(t, 42))
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	obj := decodeOne(t, stdout)
	if obj["outcome"] != "dry_run" {
		t.Errorf("outcome = %v", obj["outcome"])
	}
	if store.createCalls != 0 || store.uploadCalls != 0 || store.commitCalls != 0 {
		t.Errorf("dry-run изменил магазин")
	}
}

func TestPublishSettingsMismatch(t *testing.T) {
	t.Parallel()

	store := newPublishStore(t, 10)
	store.mismatch = true
	aab := writeAAB(t, "com.example.app", 42)

	code, stdout, _ := runAPI(t, store.handler, "--json", "publish", "-f", aab)
	if code != 5 {
		t.Fatalf("code = %d, want 5", code)
	}
	obj := decodeOne(t, stdout)
	errObj, _ := obj["error"].(map[string]any)
	if errObj["code"] != "REMOTE_STATE_MISMATCH" {
		t.Errorf("error = %v", obj["error"])
	}
	if store.uploadCalls != 0 {
		t.Errorf("загрузка началась при несовпадении настроек")
	}
}

func TestPublishInstantlyWithoutConfirmation(t *testing.T) {
	t.Parallel()

	store := newPublishStore(t, 10)
	aab := writeAAB(t, "com.example.app", 42)

	code, stdout, stderr := runAPI(t, store.handler, "--json", "publish",
		"-f", aab, "--publish-type", "instantly")
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	obj := decodeOne(t, stdout)
	if obj["outcome"] != "submitted" || obj["publish_type"] != "instantly" {
		t.Errorf("result = %v", obj)
	}
	if store.publishType != "INSTANTLY" {
		t.Errorf("publishType = %q", store.publishType)
	}
}

func TestPublishSendsListingFromMetadata(t *testing.T) {
	t.Parallel()

	store := newPublishStore(t, 10)
	metadata, _ := writeChangelog(t, 42, "Заметки")
	writeMetadataFile(t, metadata, "ru-RU/title.txt", "Приложение")
	writeMetadataFile(t, metadata, "ru-RU/short_description.txt", "Короткое описание")
	writeMetadataFile(t, metadata, "ru-RU/full_description.txt", "Полное описание\nВторая строка")
	aab := writeAAB(t, "com.example.app", 42)

	code, stdout, stderr := runAPI(t, store.handler, "--json", "publish",
		"-f", aab, "--metadata-dir", metadata)
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if store.appName == nil || *store.appName != "Приложение" {
		t.Errorf("appName = %v", store.appName)
	}
	if store.shortDescription == nil || *store.shortDescription != "Короткое описание" {
		t.Errorf("shortDescription = %v", store.shortDescription)
	}
	if store.fullDescription == nil || *store.fullDescription != "Полное описание\nВторая строка" {
		t.Errorf("fullDescription = %v", store.fullDescription)
	}

	obj := decodeOne(t, stdout)
	if obj["outcome"] != "submitted" {
		t.Errorf("result = %v", obj)
	}
}

func TestPublishSendsIconFromMetadata(t *testing.T) {
	t.Parallel()

	store := newPublishStore(t, 10)
	metadata, _ := writeChangelog(t, 42, "Заметки")
	writePNGIcon(t, metadata)
	aab := writeAAB(t, "com.example.app", 42)

	code, stdout, stderr := runAPI(t, store.handler, "--json", "publish",
		"-f", aab, "--metadata-dir", metadata)
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if store.iconCalls != 1 || len(store.icon) == 0 {
		t.Errorf("iconCalls = %d, bytes = %d", store.iconCalls, len(store.icon))
	}
	if store.uploadCalls != 1 {
		t.Errorf("uploadCalls = %d", store.uploadCalls)
	}
	obj := decodeOne(t, stdout)
	if obj["outcome"] != "submitted" {
		t.Errorf("result = %v", obj)
	}
}

func TestPublishIconNotStored(t *testing.T) {
	t.Parallel()

	store := newPublishStore(t, 10)
	store.iconNotStored = true
	metadata, _ := writeChangelog(t, 42, "Заметки")
	writePNGIcon(t, metadata)
	aab := writeAAB(t, "com.example.app", 42)

	code, stdout, _ := runAPI(t, store.handler, "--json", "publish",
		"-f", aab, "--metadata-dir", metadata)
	if code != 5 {
		t.Fatalf("code = %d, want 5", code)
	}
	obj := decodeOne(t, stdout)
	errObj, _ := obj["error"].(map[string]any)
	if errObj["code"] != "REMOTE_STATE_MISMATCH" {
		t.Errorf("error = %v", obj["error"])
	}
	if store.iconCalls != 1 || store.uploadCalls != 0 {
		t.Errorf("iconCalls = %d, uploadCalls = %d", store.iconCalls, store.uploadCalls)
	}
}

func TestPublishInvalidIconStopsBeforeNetwork(t *testing.T) {
	t.Parallel()

	store := newPublishStore(t, 10)
	metadata, _ := writeChangelog(t, 42, "Заметки")
	writeMetadataFile(t, metadata, "ru-RU/images/icon.png", "не картинка")
	aab := writeAAB(t, "com.example.app", 42)

	code, stdout, _ := runAPI(t, store.handler, "--json", "publish",
		"-f", aab, "--metadata-dir", metadata)
	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	obj := decodeOne(t, stdout)
	errObj, _ := obj["error"].(map[string]any)
	if errObj["code"] != "INVALID_METADATA" {
		t.Errorf("error = %v", obj["error"])
	}
	if store.createCalls != 0 || store.iconCalls != 0 || store.uploadCalls != 0 {
		t.Errorf("сеть вызвана до проверки иконки")
	}
}

func TestPublishInvalidListingStopsBeforeNetwork(t *testing.T) {
	t.Parallel()

	store := newPublishStore(t, 10)
	metadata, _ := writeChangelog(t, 42, "Заметки")
	writeMetadataFile(t, metadata, "ru-RU/title.txt", strings.Repeat("я", 51))
	aab := writeAAB(t, "com.example.app", 42)

	code, stdout, _ := runAPI(t, store.handler, "--json", "publish",
		"-f", aab, "--metadata-dir", metadata)
	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	obj := decodeOne(t, stdout)
	errObj, _ := obj["error"].(map[string]any)
	if errObj["code"] != "INVALID_METADATA" {
		t.Errorf("error = %v", obj["error"])
	}
	if store.createCalls != 0 || store.uploadCalls != 0 || store.commitCalls != 0 {
		t.Errorf("сеть вызвана до проверки текстов")
	}
}

func TestPublishListingMismatch(t *testing.T) {
	t.Parallel()

	store := newPublishStore(t, 10)
	store.mismatchListing = true
	metadata, _ := writeChangelog(t, 42, "Заметки")
	writeMetadataFile(t, metadata, "ru-RU/title.txt", "Приложение")
	aab := writeAAB(t, "com.example.app", 42)

	code, stdout, _ := runAPI(t, store.handler, "--json", "publish",
		"-f", aab, "--metadata-dir", metadata)
	if code != 5 {
		t.Fatalf("code = %d, want 5", code)
	}
	obj := decodeOne(t, stdout)
	errObj, _ := obj["error"].(map[string]any)
	if errObj["code"] != "REMOTE_STATE_MISMATCH" {
		t.Errorf("error = %v", obj["error"])
	}
	if store.uploadCalls != 0 {
		t.Errorf("загрузка началась при несовпадении текстов")
	}
}

// mustMetadata создаёт каталог с changelog для теста.
func mustMetadata(t *testing.T, versionCode int64) string {
	t.Helper()
	root, _ := writeChangelog(t, versionCode, "Изменения")
	return root
}

func TestPublishSendsDeveloperContacts(t *testing.T) {
	t.Parallel()

	store := newPublishStore(t, 10)
	metadata, _ := writeChangelog(t, 42, "Заметки")
	contacts := filepath.Join(metadata, "developer-contacts.json")
	if err := os.WriteFile(contacts, []byte(`{"email":"dev@example.com","website":"https://example.com"}`), 0o600); err != nil {
		t.Fatalf("write contacts: %v", err)
	}
	aab := writeAAB(t, "com.example.app", 42)

	code, _, stderr := runAPI(t, store.handler, "publish",
		"-f", aab, "--metadata-dir", metadata)
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if store.contactsEmail != "dev@example.com" {
		t.Errorf("developerContacts.email = %q", store.contactsEmail)
	}
}

func TestPublishExplicitContactsFile(t *testing.T) {
	t.Parallel()

	store := newPublishStore(t, 10)
	aab := writeAAB(t, "com.example.app", 42)
	contacts := filepath.Join(t.TempDir(), "contacts.json")
	if err := os.WriteFile(contacts, []byte(`{"website":"https://example.com"}`), 0o600); err != nil {
		t.Fatalf("write contacts: %v", err)
	}

	code, _, stderr := runAPI(t, store.handler, "publish",
		"-f", aab, "--contacts-file", contacts)
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if store.createCalls != 1 {
		t.Errorf("createCalls = %d", store.createCalls)
	}
}

func TestPublishInvalidContactsStopsBeforeNetwork(t *testing.T) {
	t.Parallel()

	store := newPublishStore(t, 10)
	aab := writeAAB(t, "com.example.app", 42)
	contacts := filepath.Join(t.TempDir(), "contacts.json")
	if err := os.WriteFile(contacts, []byte(`{"email":"bad"}`), 0o600); err != nil {
		t.Fatalf("write contacts: %v", err)
	}

	code, stdout, _ := runAPI(t, store.handler, "--json", "publish",
		"-f", aab, "--contacts-file", contacts)
	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	obj := decodeOne(t, stdout)
	errObj, _ := obj["error"].(map[string]any)
	if errObj["code"] != "INVALID_ARGUMENTS" {
		t.Errorf("error = %v", obj["error"])
	}
	if store.createCalls != 0 || store.uploadCalls != 0 {
		t.Errorf("сеть вызвана до проверки контактов")
	}
}

func (s *publishStore) listScreenshots(w http.ResponseWriter) {
	items := s.screenshots
	if s.screenshotNotStored {
		items = nil
	}
	data, err := json.Marshal(items)
	if err != nil {
		s.t.Fatalf("marshal screenshots: %v", err)
	}
	writeStoreJSON(w, http.StatusOK, fmt.Sprintf(
		`{"code":"OK","body":{"content":%s,"pageNumber":0,"pageSize":20,"totalElements":%d,"totalPages":1}}`,
		data, len(items)))
}

func (s *publishStore) uploadScreenshot(w http.ResponseWriter, r *http.Request) {
	s.screenshotUploads++
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		s.t.Fatalf("path = %q", r.URL.Path)
	}
	orientation := parts[len(parts)-3]
	ordinal, err := strconv.Atoi(parts[len(parts)-2])
	if err != nil {
		s.t.Fatalf("ordinal: %v", err)
	}
	typ := parts[len(parts)-1]

	reader, err := r.MultipartReader()
	if err != nil {
		s.t.Fatalf("multipart: %v", err)
	}
	part, err := reader.NextPart()
	if err != nil {
		s.t.Fatalf("part: %v", err)
	}
	if part.FormName() != "file" {
		s.t.Errorf("field = %q", part.FormName())
	}
	if _, err := io.Copy(io.Discard, part); err != nil {
		s.t.Fatalf("read screenshot: %v", err)
	}
	if !s.screenshotNotStored {
		s.screenshots = append(s.screenshots, storeScreenshot{
			ID:          int64(len(s.screenshots) + 1000),
			Ordinal:     ordinal,
			Type:        typ,
			Orientation: orientation,
		})
	}
	writeStoreJSON(w, http.StatusOK, `{"code":"OK"}`)
}

func (s *publishStore) deleteScreenshot(w http.ResponseWriter) {
	s.screenshotDeletes++
	if len(s.screenshots) == 0 {
		writeStoreJSON(w, http.StatusNotFound, `{"code":"error","message":"not found"}`)
		return
	}
	s.screenshots = s.screenshots[1:]
	writeStoreJSON(w, http.StatusOK, `{"code":"OK"}`)
}

func TestPublishSendsScreenshots(t *testing.T) {
	t.Parallel()

	store := newPublishStore(t, 10)
	store.screenshots = []storeScreenshot{
		{ID: 1, Ordinal: 0, Type: "SCREENSHOT", Orientation: "PORTRAIT"},
		{ID: 2, Ordinal: 1, Type: "SCREENSHOT", Orientation: "PORTRAIT"},
	}
	metadata, _ := writeChangelog(t, 42, "Заметки")
	writePhoneScreenshots(t, metadata, 3)
	aab := writeAAB(t, "com.example.app", 42)

	code, stdout, stderr := runAPI(t, store.handler, "--json", "publish",
		"-f", aab, "--metadata-dir", metadata)
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if store.screenshotDeletes != 2 {
		t.Errorf("screenshotDeletes = %d, want 2", store.screenshotDeletes)
	}
	if store.screenshotUploads != 3 {
		t.Errorf("screenshotUploads = %d, want 3", store.screenshotUploads)
	}
	if len(store.screenshots) != 3 {
		t.Errorf("screenshots = %v", store.screenshots)
	}
	obj := decodeOne(t, stdout)
	if obj["outcome"] != "submitted" {
		t.Errorf("result = %v", obj)
	}
}

func TestPublishScreenshotMismatch(t *testing.T) {
	t.Parallel()

	store := newPublishStore(t, 10)
	store.screenshotNotStored = true
	metadata, _ := writeChangelog(t, 42, "Заметки")
	writePhoneScreenshots(t, metadata, 3)
	aab := writeAAB(t, "com.example.app", 42)

	code, stdout, _ := runAPI(t, store.handler, "--json", "publish",
		"-f", aab, "--metadata-dir", metadata)
	if code != 5 {
		t.Fatalf("code = %d, want 5", code)
	}
	obj := decodeOne(t, stdout)
	errObj, _ := obj["error"].(map[string]any)
	if errObj["code"] != "REMOTE_STATE_MISMATCH" {
		t.Errorf("error = %v", obj["error"])
	}
	if store.uploadCalls != 0 {
		t.Errorf("AAB загружен при несовпадении скриншотов")
	}
}

func TestPublishInvalidScreenshotsStopsBeforeNetwork(t *testing.T) {
	t.Parallel()

	store := newPublishStore(t, 10)
	metadata, _ := writeChangelog(t, 42, "Заметки")
	writePhoneScreenshots(t, metadata, 2)
	aab := writeAAB(t, "com.example.app", 42)

	code, stdout, _ := runAPI(t, store.handler, "--json", "publish",
		"-f", aab, "--metadata-dir", metadata)
	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	obj := decodeOne(t, stdout)
	errObj, _ := obj["error"].(map[string]any)
	if errObj["code"] != "INVALID_METADATA" {
		t.Errorf("error = %v", obj["error"])
	}
	if store.createCalls != 0 || store.screenshotUploads != 0 {
		t.Errorf("сеть вызвана до проверки скриншотов")
	}
}
