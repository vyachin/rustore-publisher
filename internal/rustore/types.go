// Package rustore содержит клиент Public API RuStore, авторизацию,
// типы ответов и правила повторных попыток.
package rustore

import (
	"encoding/json"
	"strings"
)

// DefaultBaseURL — адрес Public API RuStore.
const DefaultBaseURL = "https://public-api-m.rustore.ru"

// Нормализованные типы версии.
const (
	TestingRelease = "release"
	TestingAlpha   = "alpha"
	TestingUnknown = "unknown"
)

// Статусы версии, которые использует CLI.
const (
	StatusActive              = "ACTIVE"
	StatusPartialActive       = "PARTIAL_ACTIVE"
	StatusPreviousActive      = "PREVIOUS_ACTIVE"
	StatusReadyForPublication = "READY_FOR_PUBLICATION"
	StatusDraft               = "DRAFT"
	StatusDeletedDraft        = "DELETED_DRAFT"
	StatusArchived            = "ARCHIVED"
	StatusAutoCheck           = "AUTO_CHECK"
	StatusAutoCheckFailed     = "AUTO_CHECK_FAILED"
	StatusTakenForModeration  = "TAKEN_FOR_MODERATION"
	StatusModeration          = "MODERATION"
	StatusRejectedByModerator = "REJECTED_BY_MODERATOR"
	StatusRejectedBySecurity  = "REJECTED_BY_SECURITY"
)

// TestingType — тип версии после нормализации.
//
// Отсутствие поля и явный null различаются: Present=false означает, что поля
// нет в ответе, Present=true и Raw=nil — явный null, который считается
// релизной версией.
type TestingType struct {
	Kind    string
	Raw     *string
	Present bool
}

// IsRelease сообщает, что версия подтверждена как релизная.
func (t TestingType) IsRelease() bool { return t.Kind == TestingRelease }

// NormalizeTestingType приводит поле ответа к закрытому значению.
// Поле может приходить как testingType или filterTestingType; отсутствие
// не приравнивается к null.
func NormalizeTestingType(raw json.RawMessage) TestingType {
	if len(raw) == 0 {
		return TestingType{Kind: TestingUnknown}
	}
	if string(raw) == "null" {
		return TestingType{Kind: TestingRelease, Present: true}
	}

	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return TestingType{Kind: TestingUnknown, Present: true}
	}
	if strings.EqualFold(value, "alpha") {
		return TestingType{Kind: TestingAlpha, Raw: &value, Present: true}
	}
	return TestingType{Kind: TestingUnknown, Raw: &value, Present: true}
}

// Version — версия приложения из списка версий.
type Version struct {
	VersionID        int64
	AppName          string
	AppType          string
	VersionName      string
	VersionCode      int64
	Status           string
	PublishType      string
	Testing          TestingType
	PublishDateTime  *string
	SendDateForModer *string
	PartialValue     *int
	WhatsNew         *string
}

// IsGames сообщает, что приложение относится к играм.
func (v Version) IsGames() bool {
	return v.AppType == "GAME" || v.AppType == "GAMES"
}

// Package — пакет файла внутри версии.
type Package struct {
	VerCode          int64
	IsMainApk        bool
	ServicesType     string
	AutoDetectedSDKs json.RawMessage
}

// VersionDetail — подробности версии из метода v2.
type VersionDetail struct {
	VersionID       int64
	Status          string
	StatusComment   *string
	PublishType     string
	PublishDateTime *string
	PartialValue    *int
	WhatsNew        *string
	Testing         TestingType
	Permissions     json.RawMessage
	Packages        []Package
}

// versionWire описывает ответ списка версий. Поле типа тестирования
// принимается в двух вариантах: документация называет его filterTestingType,
// а примеры ответа содержат testingType.
type versionWire struct {
	VersionID         int64           `json:"versionId"`
	AppName           string          `json:"appName"`
	AppType           string          `json:"appType"`
	VersionName       string          `json:"versionName"`
	VersionCode       int64           `json:"versionCode"`
	VersionStatus     string          `json:"versionStatus"`
	PublishType       string          `json:"publishType"`
	TestingType       json.RawMessage `json:"testingType"`
	FilterTestingType json.RawMessage `json:"filterTestingType"`
	PublishDateTime   *string         `json:"publishDateTime"`
	SendDateForModer  *string         `json:"sendDateForModer"`
	PartialValue      *int            `json:"partialValue"`
	WhatsNew          *string         `json:"whatsNew"`
}

func (w versionWire) decode() Version {
	raw := w.TestingType
	if len(raw) == 0 {
		raw = w.FilterTestingType
	}
	return Version{
		VersionID:        w.VersionID,
		AppName:          w.AppName,
		AppType:          w.AppType,
		VersionName:      w.VersionName,
		VersionCode:      w.VersionCode,
		Status:           w.VersionStatus,
		PublishType:      w.PublishType,
		Testing:          NormalizeTestingType(raw),
		PublishDateTime:  w.PublishDateTime,
		SendDateForModer: w.SendDateForModer,
		PartialValue:     w.PartialValue,
		WhatsNew:         w.WhatsNew,
	}
}

// detailWire описывает ответ подробностей версии.
type detailWire struct {
	VersionID       int64           `json:"versionId"`
	VersionStatus   string          `json:"versionStatus"`
	StatusComment   *string         `json:"statusComment"`
	PublishType     string          `json:"publishType"`
	PublishDateTime *string         `json:"publishDateTime"`
	PartialValue    *int            `json:"partialValue"`
	WhatsNew        *string         `json:"whatsNew"`
	TestingType     json.RawMessage `json:"testingType"`
	Permissions     json.RawMessage `json:"permissions"`
	Packages        []struct {
		VerCode          int64           `json:"verCode"`
		IsMainApk        bool            `json:"isMainApk"`
		ServicesType     string          `json:"servicesType"`
		AutoDetectedSDKs json.RawMessage `json:"autoDetectedSdks"`
	} `json:"packages"`
}

func (w detailWire) decode() VersionDetail {
	detail := VersionDetail{
		VersionID:       w.VersionID,
		Status:          w.VersionStatus,
		StatusComment:   w.StatusComment,
		PublishType:     w.PublishType,
		PublishDateTime: w.PublishDateTime,
		PartialValue:    w.PartialValue,
		WhatsNew:        w.WhatsNew,
		Testing:         NormalizeTestingType(w.TestingType),
		Permissions:     w.Permissions,
	}
	for _, pkg := range w.Packages {
		detail.Packages = append(detail.Packages, Package{
			VerCode:          pkg.VerCode,
			IsMainApk:        pkg.IsMainApk,
			ServicesType:     pkg.ServicesType,
			AutoDetectedSDKs: pkg.AutoDetectedSDKs,
		})
	}
	return detail
}

// page — страница списка версий.
type page struct {
	Content       []versionWire `json:"content"`
	PageNumber    int           `json:"pageNumber"`
	PageSize      int           `json:"pageSize"`
	TotalElements int           `json:"totalElements"`
	TotalPages    int           `json:"totalPages"`
}

// envelope — общий конверт ответов RuStore.
type envelope struct {
	Code      string          `json:"code"`
	Message   *string         `json:"message"`
	Body      json.RawMessage `json:"body"`
	Timestamp string          `json:"timestamp"`
}
