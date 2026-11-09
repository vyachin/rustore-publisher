// Package exitcode определяет стабильные коды завершения процесса
// и машинные коды ошибок CLI.
package exitcode

import (
	"errors"
	"fmt"
)

// Code — код завершения процесса.
type Code int

// Стабильные коды завершения из docs/design.md.
const (
	OK           Code = 0
	Internal     Code = 1
	Usage        Code = 2
	Auth         Code = 3
	NotFound     Code = 4
	Conflict     Code = 5
	Transient    Code = 6
	Rejected     Code = 7
	ManualAction Code = 8
	Interrupted  Code = 130
)

// Kind — машинный код ошибки в JSON-результате. Значение однозначно
// определяет код завершения процесса.
type Kind string

// Закрытый список error.code версии 0.0.1.
const (
	KindInternal                    Kind = "INTERNAL_ERROR"
	KindAPIResponseInvalid          Kind = "API_RESPONSE_INVALID"
	KindInvalidArguments            Kind = "INVALID_ARGUMENTS"
	KindInvalidCredentials          Kind = "INVALID_CREDENTIALS"
	KindInvalidAAB                  Kind = "INVALID_AAB"
	KindInvalidChangelog            Kind = "INVALID_CHANGELOG"
	KindAuthFailed                  Kind = "AUTH_FAILED"
	KindAccessDenied                Kind = "ACCESS_DENIED"
	KindApplicationNotFound         Kind = "APPLICATION_NOT_FOUND"
	KindVersionNotFound             Kind = "VERSION_NOT_FOUND"
	KindExistingDraft               Kind = "EXISTING_DRAFT"
	KindVersionExists               Kind = "VERSION_EXISTS"
	KindVersionStateConflict        Kind = "VERSION_STATE_CONFLICT"
	KindVersionUnavailable          Kind = "VERSION_UNAVAILABLE"
	KindRemoteStateMismatch         Kind = "REMOTE_STATE_MISMATCH"
	KindAPIUnavailable              Kind = "API_UNAVAILABLE"
	KindCommandTimeout              Kind = "COMMAND_TIMEOUT"
	KindRequestTimeout              Kind = "REQUEST_TIMEOUT"
	KindAABProcessingTimeout        Kind = "AAB_PROCESSING_TIMEOUT"
	KindOutcomeUnknown              Kind = "OUTCOME_UNKNOWN"
	KindVersionRejected             Kind = "VERSION_REJECTED"
	KindBuildRejected               Kind = "BUILD_REJECTED"
	KindAPIRejected                 Kind = "API_REJECTED"
	KindAABSigningRequired          Kind = "AAB_SIGNING_REQUIRED"
	KindPermissionDeclarationNeeded Kind = "PERMISSION_DECLARATION_REQUIRED"
	KindInterrupted                 Kind = "INTERRUPTED"
)

var kindCodes = map[Kind]Code{
	KindInternal:                    Internal,
	KindAPIResponseInvalid:          Internal,
	KindInvalidArguments:            Usage,
	KindInvalidCredentials:          Usage,
	KindInvalidAAB:                  Usage,
	KindInvalidChangelog:            Usage,
	KindAuthFailed:                  Auth,
	KindAccessDenied:                Auth,
	KindApplicationNotFound:         NotFound,
	KindVersionNotFound:             NotFound,
	KindExistingDraft:               Conflict,
	KindVersionExists:               Conflict,
	KindVersionStateConflict:        Conflict,
	KindVersionUnavailable:          Conflict,
	KindRemoteStateMismatch:         Conflict,
	KindAPIUnavailable:              Transient,
	KindCommandTimeout:              Transient,
	KindRequestTimeout:              Transient,
	KindAABProcessingTimeout:        Transient,
	KindOutcomeUnknown:              Transient,
	KindVersionRejected:             Rejected,
	KindBuildRejected:               Rejected,
	KindAPIRejected:                 Rejected,
	KindAABSigningRequired:          ManualAction,
	KindPermissionDeclarationNeeded: ManualAction,
	KindInterrupted:                 Interrupted,
}

// Code возвращает код завершения процесса для машинного кода ошибки.
func (k Kind) Code() Code {
	if code, ok := kindCodes[k]; ok {
		return code
	}
	return Internal
}

// Error — ошибка с машинным кодом и подсказкой пользователю.
type Error struct {
	Kind Kind
	Err  error
	Hint string
}

// Error реализует интерфейс error.
func (e *Error) Error() string { return e.Err.Error() }

// Unwrap открывает исходную ошибку.
func (e *Error) Unwrap() error { return e.Err }

// New создаёт ошибку заданного вида.
func New(kind Kind, err error) error {
	if err == nil {
		return nil
	}
	return &Error{Kind: kind, Err: err}
}

// Errorf создаёт ошибку заданного вида с отформатированным сообщением.
func Errorf(kind Kind, format string, args ...any) error {
	return &Error{Kind: kind, Err: fmt.Errorf(format, args...)}
}

// Wrap привязывает вид к ошибке. Для nil возвращает nil.
func Wrap(kind Kind, err error) error {
	if err == nil {
		return nil
	}
	return &Error{Kind: kind, Err: err}
}

// WithHint возвращает копию ошибки с подсказкой. Ошибка без вида
// становится внутренней.
func WithHint(err error, hint string) error {
	if err == nil {
		return nil
	}

	var coded *Error
	if errors.As(err, &coded) {
		withHint := *coded
		withHint.Hint = hint
		return &withHint
	}
	return &Error{Kind: KindInternal, Err: err, Hint: hint}
}

// From возвращает код завершения для ошибки. Ошибки без вида
// считаются внутренними.
func From(err error) Code {
	if err == nil {
		return OK
	}
	return KindOf(err).Code()
}

// KindOf возвращает машинный код ошибки. Ошибки без вида
// считаются внутренними.
func KindOf(err error) Kind {
	var coded *Error
	if errors.As(err, &coded) {
		return coded.Kind
	}
	return KindInternal
}

// HintOf возвращает подсказку ошибки, если она задана.
func HintOf(err error) string {
	var coded *Error
	if errors.As(err, &coded) {
		return coded.Hint
	}
	return ""
}
