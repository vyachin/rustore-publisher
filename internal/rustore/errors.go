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
	case apiErr.Status == 404 && op == opVersionDetail:
		kind = exitcode.KindVersionNotFound
		hint = "проверьте идентификатор версии"
	case apiErr.Status == 404:
		// Права и отсутствие приложения различаются не всегда, поэтому
		// не выдаём 404 за подтверждённое отсутствие.
		kind = exitcode.KindAccessDenied
		hint = "проверьте имя пакета и права ключа; отсутствие приложения не подтверждено"
	case apiErr.Status == 400 || apiErr.Status == 422:
		if op == opListVersions || op == opVersionDetail {
			kind = exitcode.KindInvalidArguments
			hint = "проверьте имя пакета и параметры запроса"
		}
	}

	err := exitcode.Wrap(kind, apiErr)
	if hint != "" {
		return exitcode.WithHint(err, hint)
	}
	return err
}
