package cli

import (
	"encoding/json"
	"io"

	"github.com/vyachin/rustore-publisher/internal/exitcode"
)

// SchemaVersion — версия машинного контракта JSON.
const SchemaVersion = 1

// Фазы выполнения команды из закрытого списка.
const (
	PhasePreflight  = "preflight"
	PhaseAuth       = "auth"
	PhaseCreate     = "create"
	PhaseUpload     = "upload"
	PhaseProcessing = "processing"
	PhaseCommit     = "commit"
	PhaseStatus     = "status"
)

// Итоги выполнения команды из закрытого списка.
const (
	OutcomeSubmitted = "submitted"
	OutcomeSkipped   = "skipped"
	OutcomeDryRun    = "dry_run"
	OutcomeInspected = "inspected"
	OutcomeFailed    = "failed"
	OutcomeUnknown   = "unknown"
)

// Warning — предупреждение CLI. Текст сообщения не используется для ветвления.
type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ErrorDetail — известная ошибка команды с подсказкой.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
}

// VersionFact — сведения об одной версии для status.
type VersionFact struct {
	VersionID        *int64  `json:"version_id"`
	VersionCode      *int64  `json:"version_code"`
	VersionStatus    *string `json:"version_status"`
	TestingType      *string `json:"testing_type"`
	TestingTypeRaw   *string `json:"testing_type_raw"`
	PublishType      *string `json:"publish_type"`
	PartialValue     *int    `json:"partial_value"`
	WhatsNew         *string `json:"whats_new"`
	SendDateForModer *string `json:"send_date_for_moder"`
	Submitted        *bool   `json:"submitted"`
	Published        *bool   `json:"published"`
	Permissions      any     `json:"permissions"`
	AutoDetectedSDKs any     `json:"auto_detected_sdks"`
}

// Result — общее содержимое результата publish, doctor, status и ошибок
// version или completion.
type Result struct {
	SchemaVersion    int          `json:"schema_version"`
	Command          string       `json:"command"`
	Package          *string      `json:"package"`
	VersionCode      *int64       `json:"version_code"`
	VersionID        *int64       `json:"version_id"`
	Phase            string       `json:"phase"`
	Outcome          string       `json:"outcome"`
	VersionStatus    *string      `json:"version_status"`
	TestingType      *string      `json:"testing_type"`
	TestingTypeRaw   *string      `json:"testing_type_raw"`
	PublishType      *string      `json:"publish_type"`
	PartialValue     *int         `json:"partial_value"`
	WhatsNew         *string      `json:"whats_new"`
	SendDateForModer *string      `json:"send_date_for_moder"`
	Submitted        *bool        `json:"submitted"`
	Published        *bool        `json:"published"`
	Skipped          bool         `json:"skipped"`
	ManualAction     bool         `json:"manual_action"`
	OutcomeUnknown   bool         `json:"outcome_unknown"`
	Permissions      any          `json:"permissions"`
	AutoDetectedSDKs any          `json:"auto_detected_sdks"`
	Warnings         []Warning    `json:"warnings"`
	Error            *ErrorDetail `json:"error"`
}

// StatusResult добавляет к результату сведения о версиях.
type StatusResult struct {
	Result
	LatestRelease *VersionFact `json:"latest_release"`
	Draft         *VersionFact `json:"draft"`
}

// VersionResult — успешный результат version --json.
type VersionResult struct {
	SchemaVersion int    `json:"schema_version"`
	Command       string `json:"command"`
	Version       string `json:"version"`
	Commit        string `json:"commit"`
	BuildDate     string `json:"build_date"`
}

// NewResult создаёт результат команды с фазой preflight и итогом failed.
// Успешные команды меняют фазу и итог по мере выполнения.
func NewResult(command string) *Result {
	return &Result{
		SchemaVersion: SchemaVersion,
		Command:       command,
		Phase:         PhasePreflight,
		Outcome:       OutcomeFailed,
		Warnings:      []Warning{},
	}
}

// NewErrorDetail описывает ошибку для JSON-результата.
func NewErrorDetail(err error) *ErrorDetail {
	if err == nil {
		return nil
	}
	return &ErrorDetail{
		Code:    string(exitcode.KindOf(err)),
		Message: err.Error(),
		Hint:    exitcode.HintOf(err),
	}
}

// EncodeJSON печатает один объект JSON с отключённым HTML-экранированием,
// чтобы тексты замечаний с символами <, > и & оставались читаемыми.
func EncodeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return exitcode.Wrap(exitcode.KindInternal, err)
	}
	return nil
}

// renderedError помечает ошибку, результат которой уже напечатан в stdout,
// чтобы Execute не выводил второй объект.
type renderedError struct {
	err error
}

func (e *renderedError) Error() string { return e.err.Error() }

func (e *renderedError) Unwrap() error { return e.err }

// writeRenderedError используется командами, которые сами печатают JSON
// с накопленными фактами.
func writeRenderedError(w io.Writer, result any, err error) error {
	if writeErr := EncodeJSON(w, result); writeErr != nil {
		return writeErr
	}
	return &renderedError{err: err}
}
