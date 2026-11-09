package rustore

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/vyachin/rustore-publisher/internal/exitcode"
)

// maxAABSize — ограничение RuStore на размер AAB.
const maxAABSize = 5 << 30

// UploadAAB потоково загружает AAB в созданный черновик.
//
// Запрос не повторяется: после начала передачи результат может быть
// неизвестен, поэтому вызывающий код обязан проверить состояние версии.
// Короткий таймаут запроса не применяется, действует общий контекст команды.
func (c *Client) UploadAAB(ctx context.Context, packageName string, versionID int64, path string) (UploadResult, error) {
	if packageName == "" || versionID <= 0 {
		return UploadResult{}, exitcode.Errorf(exitcode.KindInvalidArguments,
			"для загрузки AAB нужны package и положительный versionId")
	}

	file, err := os.Open(path)
	if err != nil {
		return UploadResult{}, exitcode.WithHint(
			exitcode.Wrap(exitcode.KindInvalidAAB, fmt.Errorf("открыть %q: %w", path, err)),
			"проверьте путь к AAB",
		)
	}
	defer func() { _ = file.Close() }()

	stat, err := file.Stat()
	if err != nil {
		return UploadResult{}, exitcode.Wrap(exitcode.KindInvalidAAB, err)
	}
	if stat.Size() <= 0 {
		return UploadResult{}, exitcode.Errorf(exitcode.KindInvalidAAB, "файл %q пуст", path)
	}
	if stat.Size() > maxAABSize {
		return UploadResult{}, exitcode.Errorf(exitcode.KindInvalidAAB,
			"файл %q больше допустимых 5 ГБ", path)
	}

	token, err := c.RefreshToken(ctx)
	if err != nil {
		return UploadResult{}, err
	}

	reader, writer := io.Pipe()
	defer func() { _ = reader.Close() }()
	form := multipart.NewWriter(writer)
	go func() {
		writeErr := writeFilePart(form, "file", filepath.Base(path), file)
		if writeErr == nil {
			writeErr = form.Close()
		}
		_ = writer.CloseWithError(writeErr)
	}()

	url := fmt.Sprintf("%s/public/v1/application/%s/version/%d/aab",
		c.cfg.BaseURL, url.PathEscape(packageName), versionID)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, reader)
	if err != nil {
		_ = reader.CloseWithError(err)
		return UploadResult{}, exitcode.Wrap(exitcode.KindInternal, err)
	}
	request.Header.Set("Content-Type", form.FormDataContentType())
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Public-Token", token)
	filename := filepath.Base(path)
	request.ContentLength = uploadLength(stat.Size(), form.Boundary(), filename)

	c.cfg.Logger.DebugContext(ctx, "загрузка AAB",
		"path", path, "size", stat.Size(), "version_id", versionID)

	response, err := c.cfg.HTTPClient.Do(request)
	if err != nil {
		_ = reader.CloseWithError(err)
		if ctx.Err() != nil {
			return UploadResult{}, ctx.Err()
		}
		return UploadResult{}, exitcode.WithHint(
			exitcode.Wrap(exitcode.KindOutcomeUnknown,
				fmt.Errorf("загрузка AAB прервана: %w", err)),
			fmt.Sprintf("проверьте версию %d командой status и в консоли RuStore", versionID),
		)
	}
	defer func() { _ = response.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return UploadResult{}, exitcode.WithHint(
			exitcode.Wrap(exitcode.KindOutcomeUnknown,
				fmt.Errorf("прочитать ответ загрузки: %w", err)),
			fmt.Sprintf("проверьте версию %d командой status", versionID),
		)
	}

	if response.StatusCode >= http.StatusBadRequest {
		return UploadResult{}, classifyAPIError(opUpload, apiErrorFrom(response.StatusCode, data))
	}

	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return UploadResult{}, exitcode.WithHint(
			exitcode.Wrap(exitcode.KindAPIResponseInvalid,
				fmt.Errorf("разобрать ответ загрузки: %w", err)),
			"повторите команду; если ошибка повторяется, проверьте ответ API",
		)
	}
	if env.Code != "OK" {
		return UploadResult{}, classifyAPIError(opUpload, &APIError{
			Status:  response.StatusCode,
			Code:    env.Code,
			Message: messageOf(env.Message),
		})
	}

	var body uploadBody
	if len(env.Body) > 0 && string(env.Body) != "null" {
		if err := json.Unmarshal(env.Body, &body); err != nil {
			return UploadResult{}, exitcode.Wrap(exitcode.KindAPIResponseInvalid,
				fmt.Errorf("разобрать разрешения из ответа загрузки: %w", err))
		}
	}
	return UploadResult(body), nil
}

// writeFilePart копирует файл в multipart-часть.
func writeFilePart(form *multipart.Writer, field, filename string, file *os.File) error {
	part, err := form.CreateFormFile(field, filename)
	if err != nil {
		return err
	}
	_, err = io.Copy(part, file)
	return err
}

// uploadLength вычисляет точную длину multipart-тела, чтобы отправить
// Content-Length вместо chunked. Имя файла экранируется так же, как это
// делает multipart.Writer.
func uploadLength(fileSize int64, boundary, filename string) int64 {
	escaped := strings.NewReplacer("\\", "\\\\", `"`, "\\\"").Replace(filename)
	header := fmt.Sprintf("--%s\r\nContent-Disposition: form-data; name=\"file\"; filename=\"%s\"\r\nContent-Type: application/octet-stream\r\n\r\n",
		boundary, escaped)
	footer := fmt.Sprintf("\r\n--%s--\r\n", boundary)
	return int64(len(header)) + fileSize + int64(len(footer))
}

// apiErrorFrom собирает APIError из тела ответа.
func apiErrorFrom(status int, data []byte) *APIError {
	apiErr := &APIError{Status: status}
	var parsed envelope
	if json.Unmarshal(data, &parsed) == nil {
		apiErr.Code = parsed.Code
		apiErr.Message = messageOf(parsed.Message)
	}
	if apiErr.Message == "" && len(data) > 0 && len(data) < 500 && !json.Valid(data) {
		apiErr.Message = string(data)
	}
	return apiErr
}

func messageOf(message *string) string {
	if message == nil {
		return ""
	}
	return *message
}
