package rustore

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/vyachin/rustore-publisher/internal/exitcode"
)

const (
	// defaultPageSize — максимальный размер страницы списка версий.
	defaultPageSize = 100
	// maxPages защищает от бесконечной пагинации при некорректном ответе.
	maxPages = 1000
)

// ListOptions управляет выборкой версий.
type ListOptions struct {
	TestingType string
	IDs         []int64
	PageSize    int
}

// ListVersions читает все страницы списка версий приложения.
func (c *Client) ListVersions(ctx context.Context, packageName string, opts ListOptions) ([]Version, error) {
	if packageName == "" {
		return nil, exitcode.Errorf(exitcode.KindInvalidArguments, "не задан package приложения")
	}

	testingType := opts.TestingType
	if testingType == "" {
		testingType = "ALL"
	}
	query := map[string]string{"filterTestingType": testingType}

	if len(opts.IDs) > 0 {
		ids := ""
		for i, id := range opts.IDs {
			if i > 0 {
				ids += ","
			}
			ids += strconv.FormatInt(id, 10)
		}
		query["ids"] = ids
		return c.readVersionPages(ctx, packageName, query, true)
	}

	return c.readVersionPages(ctx, packageName, query, false)
}

func (c *Client) readVersionPages(ctx context.Context, packageName string, query map[string]string, singlePage bool) ([]Version, error) {
	pageSize := defaultPageSize
	if singlePage {
		pageSize = 0
	}

	var versions []Version
	for pageNumber := 0; ; pageNumber++ {
		if pageNumber >= maxPages {
			return nil, exitcode.WithHint(
				exitcode.Errorf(exitcode.KindAPIResponseInvalid,
					"API вернул больше %d страниц версий", maxPages),
				"повторите команду; если ошибка повторяется, сообщите о ней разработчику",
			)
		}

		pageQuery := make(map[string]string, len(query)+2)
		for key, value := range query {
			pageQuery[key] = value
		}
		if pageSize > 0 {
			pageQuery["page"] = strconv.Itoa(pageNumber)
			pageQuery["size"] = strconv.Itoa(pageSize)
		}

		env, err := c.do(ctx, http.MethodGet, versionListPath(packageName), pageQuery, nil, opListVersions)
		if err != nil {
			return nil, err
		}

		var body page
		if err := json.Unmarshal(env.Body, &body); err != nil {
			return nil, exitcode.WithHint(
				exitcode.Wrap(exitcode.KindAPIResponseInvalid,
					fmt.Errorf("разобрать список версий: %w", err)),
				"повторите команду; если ошибка повторяется, проверьте доступность API",
			)
		}
		if body.TotalPages < 0 || body.PageSize < 0 {
			return nil, exitcode.Errorf(exitcode.KindAPIResponseInvalid,
				"список версий содержит отрицательные размеры страниц")
		}

		for _, wire := range body.Content {
			versions = append(versions, wire.decode())
		}

		if singlePage || pageNumber+1 >= body.TotalPages || len(body.Content) == 0 {
			break
		}
	}

	return versions, nil
}

// VersionDetail читает подробности версии методом v2.
func (c *Client) VersionDetail(ctx context.Context, packageName string, versionID int64) (VersionDetail, error) {
	if packageName == "" || versionID <= 0 {
		return VersionDetail{}, exitcode.Errorf(exitcode.KindInvalidArguments,
			"для подробностей версии нужны package и положительный versionId")
	}

	path := fmt.Sprintf("/public/v2/application/%s/version/%d",
		url.PathEscape(packageName), versionID)

	env, err := c.do(ctx, http.MethodGet, path, nil, nil, opVersionDetail)
	if err != nil {
		return VersionDetail{}, err
	}

	var wire detailWire
	if err := json.Unmarshal(env.Body, &wire); err != nil {
		return VersionDetail{}, exitcode.WithHint(
			exitcode.Wrap(exitcode.KindAPIResponseInvalid,
				fmt.Errorf("разобрать подробности версии: %w", err)),
			"повторите команду; если ошибка повторяется, проверьте ответ API",
		)
	}
	if wire.VersionID != versionID {
		return VersionDetail{}, exitcode.Errorf(exitcode.KindAPIResponseInvalid,
			"API вернул версию %d вместо запрошенной %d", wire.VersionID, versionID)
	}

	return wire.decode(), nil
}

// versionListPath собирает путь списка версий.
func versionListPath(packageName string) string {
	return fmt.Sprintf("/public/v1/application/%s/version", url.PathEscape(packageName))
}
