package rustore

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/vyachin/rustore-publisher/internal/exitcode"
)

// CreateDraft создаёт черновик версии и возвращает его versionId.
func (c *Client) CreateDraft(ctx context.Context, packageName string, request CreateDraftRequest) (int64, error) {
	if packageName == "" {
		return 0, exitcode.Errorf(exitcode.KindInvalidArguments, "не задан package приложения")
	}

	body, err := json.Marshal(request)
	if err != nil {
		return 0, exitcode.Wrap(exitcode.KindInternal, err)
	}

	env, err := c.do(ctx, http.MethodPost, versionListPath(packageName), nil, body, opCreate)
	if err != nil {
		return 0, err
	}

	versionID, err := parseCreatedVersionID(env.Body)
	if err != nil {
		return 0, exitcode.WithHint(
			exitcode.Wrap(exitcode.KindAPIResponseInvalid, err),
			"повторите команду; если ошибка повторяется, проверьте ответ API",
		)
	}
	return versionID, nil
}

// parseCreatedVersionID принимает ответ создания в известных формах:
// число, объект с versionId или объект с content.versionId.
func parseCreatedVersionID(raw json.RawMessage) (int64, error) {
	if len(raw) == 0 {
		return 0, fmt.Errorf("ответ создания черновика пуст")
	}

	var direct int64
	if err := json.Unmarshal(raw, &direct); err == nil {
		if direct <= 0 {
			return 0, fmt.Errorf("ответ создания содержит некорректный versionId %d", direct)
		}
		return direct, nil
	}

	var object struct {
		VersionID int64 `json:"versionId"`
		Content   *struct {
			VersionID int64 `json:"versionId"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &object); err != nil {
		return 0, fmt.Errorf("разобрать ответ создания черновика: %w", err)
	}
	if object.VersionID > 0 {
		return object.VersionID, nil
	}
	if object.Content != nil && object.Content.VersionID > 0 {
		return object.Content.VersionID, nil
	}
	return 0, fmt.Errorf("ответ создания черновика не содержит versionId")
}

// Commit отправляет черновик на модерацию с минимальным приоритетом.
func (c *Client) Commit(ctx context.Context, packageName string, versionID int64) error {
	if packageName == "" || versionID <= 0 {
		return exitcode.Errorf(exitcode.KindInvalidArguments,
			"для отправки на модерацию нужны package и положительный versionId")
	}

	path := fmt.Sprintf("/public/v1/application/%s/version/%d/commit",
		url.PathEscape(packageName), versionID)
	_, err := c.do(ctx, http.MethodPost, path,
		map[string]string{"priorityUpdate": "0"}, nil, opCommit)
	return err
}
