package rustore

import (
	"fmt"

	"github.com/vyachin/rustore-publisher/internal/exitcode"
)

// APIError — отказ API RuStore с HTTP-статусом и сообщением.
type APIError struct {
	Status  int
	Code    string
	Message string
}

// Error реализует интерфейс error.
func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("API RuStore вернул %d: %s", e.Status, e.Message)
	}
	if e.Code != "" {
		return fmt.Sprintf("API RuStore вернул %d (код %s)", e.Status, e.Code)
	}
	return fmt.Sprintf("API RuStore вернул %d", e.Status)
}

// operation — вид запроса, от которого зависит классификация ошибки.
type operation int

const (
	opAuth operation = iota
	opListVersions
	opVersionDetail
	opCreate
	opUpload
	opUploadIcon
	opCommit
)

// classifyAPIError переводит ответ API в категорию ошибки CLI.
func classifyAPIError(op operation, apiErr *APIError) error {
	kind := exitcode.KindAPIRejected
	hint := ""

	switch {
	case apiErr.Status == 401:
		kind = exitcode.KindAuthFailed
		hint = "проверьте keyId и приватный ключ в файле учётных данных"
	case apiErr.Status == 403:
		kind = exitcode.KindAccessDenied
		hint = "проверьте, что ключ API выдан для этого приложения и нужных методов"
	case apiErr.Status == 404 && (op == opVersionDetail || op == opUpload || op == opUploadIcon || op == opCommit):
		kind = exitcode.KindVersionNotFound
		hint = "проверьте идентификатор версии"
	case apiErr.Status == 404 && op == opCreate:
		kind = exitcode.KindApplicationNotFound
		hint = "проверьте имя пакета и права ключа API"
	case apiErr.Status == 404:
		// Права и отсутствие приложения различаются не всегда, поэтому
		// не выдаём 404 за подтверждённое отсутствие.
		kind = exitcode.KindAccessDenied
		hint = "проверьте имя пакета и права ключа; отсутствие приложения не подтверждено"
	case apiErr.Status == 409 && op == opCreate:
		kind = exitcode.KindExistingDraft
		hint = "завершите или удалите существующий черновик в консоли RuStore"
	case (op == opCreate || op == opUpload || op == opUploadIcon || op == opCommit) && apiErr.Status >= 500:
		// Ответ 5xx на запись не доказывает, что изменение не применилось.
		kind = exitcode.KindOutcomeUnknown
		hint = "проверьте состояние версии командой status и в консоли RuStore"
	case apiErr.Status == 400 || apiErr.Status == 422:
		switch op {
		case opListVersions, opVersionDetail:
			kind = exitcode.KindInvalidArguments
			hint = "проверьте имя пакета и параметры запроса"
		case opCreate:
			hint = "заполните обязательные поля карточки приложения (например, контакты разработчика) в консоли RuStore и повторите"
		case opUploadIcon:
			hint = "проверьте формат иконки: PNG или JPEG 512×512, до 3 МБ"
		case opCommit:
			hint = "проверьте обязательные поля и разрешения версии в консоли RuStore"
		}
	}

	err := exitcode.Wrap(kind, apiErr)
	if hint != "" {
		return exitcode.WithHint(err, hint)
	}
	return err
}
