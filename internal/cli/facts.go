package cli

import (
	"encoding/json"

	"github.com/vyachin/rustore-publisher/internal/app"
	"github.com/vyachin/rustore-publisher/internal/rustore"
)

// newVersionFact переводит версию RuStore в поля JSON-контракта.
func newVersionFact(version *rustore.Version, detail *rustore.VersionDetail) *VersionFact {
	if version == nil {
		return nil
	}

	fact := &VersionFact{
		VersionID:        int64Ptr(version.VersionID),
		VersionCode:      int64Ptr(version.VersionCode),
		VersionStatus:    stringPtrOrNil(version.Status),
		TestingType:      stringPtrOrNil(version.Testing.Kind),
		TestingTypeRaw:   version.Testing.Raw,
		PublishType:      publishTypePtr(version.PublishType),
		PartialValue:     version.PartialValue,
		WhatsNew:         version.WhatsNew,
		SendDateForModer: version.SendDateForModer,
		Submitted:        submitted(version),
		Published:        published(version),
	}
	if detail != nil {
		fact.Permissions = decodeRaw(detail.Permissions)
		fact.AutoDetectedSDKs = mainPackageSDKs(detail, version.VersionCode)
	}
	return fact
}

// applyVersionResult заполняет верхние поля результата фактами версии.
func applyVersionResult(result *Result, version *rustore.Version, detail *rustore.VersionDetail) {
	if version == nil {
		return
	}
	result.VersionID = int64Ptr(version.VersionID)
	result.VersionCode = int64Ptr(version.VersionCode)
	result.VersionStatus = stringPtrOrNil(version.Status)
	result.TestingType = stringPtrOrNil(version.Testing.Kind)
	result.TestingTypeRaw = version.Testing.Raw
	result.PublishType = publishTypePtr(version.PublishType)
	result.PartialValue = version.PartialValue
	result.WhatsNew = version.WhatsNew
	result.SendDateForModer = version.SendDateForModer
	result.Submitted = submitted(version)
	result.Published = published(version)
	if detail != nil {
		result.Permissions = decodeRaw(detail.Permissions)
		result.AutoDetectedSDKs = mainPackageSDKs(detail, version.VersionCode)
	}
}

// publishTypePtr приводит publishType RuStore к закрытому перечислению.
func publishTypePtr(value string) *string {
	var normalized string
	switch value {
	case "MANUAL":
		normalized = "manual"
	case "INSTANTLY":
		normalized = "instantly"
	default:
		normalized = "unknown"
	}
	return &normalized
}

// submitted сообщает, известна ли отправка версии на модерацию.
func submitted(version *rustore.Version) *bool {
	value := version.SendDateForModer != nil && *version.SendDateForModer != ""
	return &value
}

// published сообщает, была ли версия опубликована. Для неизвестного статуса
// значение остаётся неопределённым.
func published(version *rustore.Version) *bool {
	value := false
	switch version.Status {
	case rustore.StatusActive, rustore.StatusPartialActive,
		rustore.StatusPreviousActive, rustore.StatusArchived:
		value = true
	case rustore.StatusDraft, rustore.StatusDeletedDraft,
		rustore.StatusReadyForPublication, rustore.StatusAutoCheck,
		rustore.StatusAutoCheckFailed, rustore.StatusTakenForModeration,
		rustore.StatusModeration, rustore.StatusRejectedByModerator,
		rustore.StatusRejectedBySecurity:
		value = false
	default:
		return nil
	}
	return &value
}

// mainPackageSDKs возвращает автоопределённые SDK основного пакета.
func mainPackageSDKs(detail *rustore.VersionDetail, versionCode int64) any {
	return decodeRaw(mainPackageSDKRaw(detail, versionCode))
}

// mainPackageSDKRaw возвращает сырой JSON автоопределённых SDK.
func mainPackageSDKRaw(detail *rustore.VersionDetail, versionCode int64) json.RawMessage {
	pkg := app.MainPackage(detail, versionCode)
	if pkg == nil {
		return nil
	}
	return pkg.AutoDetectedSDKs
}

// decodeRaw декодирует сырой JSON в значение для JSON-результата.
func decodeRaw(raw json.RawMessage) any {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil
	}
	return value
}

// mapWarnings переводит предупреждения приложения в контракт CLI.
func mapWarnings(warnings []app.Warning) []Warning {
	result := make([]Warning, 0, len(warnings))
	for _, warning := range warnings {
		result = append(result, Warning{Code: warning.Code, Message: warning.Message})
	}
	return result
}

func int64Ptr(value int64) *int64 { return &value }

// stringPtrOrNil возвращает указатель для непустого значения.
func stringPtrOrNil(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
