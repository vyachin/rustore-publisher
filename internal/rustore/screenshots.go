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

// ScreenshotType — тип устройства для скриншота в API v2.
type ScreenshotType string

// Допустимые типы скриншотов.
const (
	ScreenshotPhone  ScreenshotType = "SCREENSHOT"
	ScreenshotTablet ScreenshotType = "TABLET_SCREENSHOT"
	ScreenshotTV     ScreenshotType = "TV_SCREENSHOT"
)

// ScreenshotOrientation — ориентация скриншота.
type ScreenshotOrientation string

// Допустимые ориентации скриншотов.
const (
	OrientationPortrait  ScreenshotOrientation = "PORTRAIT"
	OrientationLandscape ScreenshotOrientation = "LANDSCAPE"
)

// maxScreenshotSize — ограничение RuStore на размер скриншота.
const maxScreenshotSize = 5 << 20

// screenshotPageSize — размер страницы списка скриншотов.
const screenshotPageSize = 20

// Screenshot — скриншот черновика из списка API.
type Screenshot struct {
	ID          int64
	Ordinal     int
	Type        ScreenshotType
	Orientation ScreenshotOrientation
	FileURL     string
	FileUID     string
}

// ListScreenshots читает все страницы списка скриншотов черновика.
func (c *Client) ListScreenshots(ctx context.Context, packageName string, versionID int64) ([]Screenshot, error) {
	if packageName == "" || versionID <= 0 {
		return nil, exitcode.Errorf(exitcode.KindInvalidArguments,
			"для списка скриншотов нужны package и положительный versionId")
	}

	path := fmt.Sprintf("/public/v2/application/%s/version/%d/image/screenshot",
		url.PathEscape(packageName), versionID)

	var screenshots []Screenshot
	for page := 0; ; page++ {
		if page >= maxPages {
			return nil, exitcode.WithHint(
				exitcode.Errorf(exitcode.KindAPIResponseInvalid,
					"API вернул больше %d страниц скриншотов", maxPages),
				"повторите команду; если ошибка повторяется, сообщите о ней разработчику",
			)
		}

		env, err := c.do(ctx, http.MethodGet, path, map[string]string{
			"page": strconv.Itoa(page),
			"size": strconv.Itoa(screenshotPageSize),
		}, nil, opListScreenshots)
		if err != nil {
			return nil, err
		}

		var body screenshotPage
		if err := json.Unmarshal(env.Body, &body); err != nil {
			return nil, exitcode.WithHint(
				exitcode.Wrap(exitcode.KindAPIResponseInvalid,
					fmt.Errorf("разобрать список скриншотов: %w", err)),
				"повторите команду; если ошибка повторяется, проверьте ответ API",
			)
		}
		for _, wire := range body.Content {
			screenshots = append(screenshots, wire.decode())
		}
		if page+1 >= body.TotalPages || len(body.Content) == 0 {
			break
		}
	}
	return screenshots, nil
}

// UploadScreenshot загружает один скриншот в черновик.
func (c *Client) UploadScreenshot(ctx context.Context, packageName string, versionID int64,
	screenshotType ScreenshotType, orientation ScreenshotOrientation, ordinal int, path string,
) error {
	if packageName == "" || versionID <= 0 {
		return exitcode.Errorf(exitcode.KindInvalidArguments,
			"для загрузки скриншота нужны package и положительный versionId")
	}
	if !validScreenshotType(screenshotType) {
		return exitcode.Errorf(exitcode.KindInvalidArguments,
			"неизвестный тип скриншота %q", screenshotType)
	}
	if !validOrientation(orientation) {
		return exitcode.Errorf(exitcode.KindInvalidArguments,
			"неизвестная ориентация скриншота %q", orientation)
	}
	if ordinal < 0 || ordinal > 9 {
		return exitcode.Errorf(exitcode.KindInvalidArguments,
			"ordinal скриншота должен быть от 0 до 9, получен %d", ordinal)
	}

	endpoint := fmt.Sprintf("/public/v2/application/%s/version/%d/image/screenshot/%s/%d/%s",
		url.PathEscape(packageName), versionID, orientation, ordinal, screenshotType)
	return c.uploadImage(ctx, endpoint, path, maxScreenshotSize, opUploadScreenshot)
}

// DeleteScreenshot удаляет скриншот черновика по идентификатору.
// Ответ 404 считается успехом: скриншота уже нет.
func (c *Client) DeleteScreenshot(ctx context.Context, packageName string, versionID, screenshotID int64) error {
	if packageName == "" || versionID <= 0 || screenshotID <= 0 {
		return exitcode.Errorf(exitcode.KindInvalidArguments,
			"для удаления скриншота нужны package, versionId и screenshotId")
	}

	path := fmt.Sprintf("/public/v2/application/%s/version/%d/image/screenshot/%d",
		url.PathEscape(packageName), versionID, screenshotID)
	env, err := c.do(ctx, http.MethodDelete, path, nil, nil, opDeleteScreenshot)
	if err != nil {
		if exitcode.KindOf(err) == exitcode.KindVersionNotFound {
			return nil
		}
		return err
	}
	if env.Code != "OK" {
		return classifyAPIError(opDeleteScreenshot, &APIError{
			Status:  http.StatusOK,
			Code:    env.Code,
			Message: messageOf(env.Message),
		})
	}
	return nil
}

// screenshotPage — страница списка скриншотов.
type screenshotPage struct {
	Content    []screenshotWire `json:"content"`
	TotalPages int              `json:"totalPages"`
}

type screenshotWire struct {
	ID          int64  `json:"id"`
	Ordinal     int    `json:"ordinal"`
	Type        string `json:"type"`
	Orientation string `json:"orientation"`
	FileURL     string `json:"fileUrl"`
	FileUID     string `json:"fileUid"`
}

func (w screenshotWire) decode() Screenshot {
	return Screenshot{
		ID:          w.ID,
		Ordinal:     w.Ordinal,
		Type:        ScreenshotType(w.Type),
		Orientation: ScreenshotOrientation(w.Orientation),
		FileURL:     w.FileURL,
		FileUID:     w.FileUID,
	}
}

func validScreenshotType(value ScreenshotType) bool {
	switch value {
	case ScreenshotPhone, ScreenshotTablet, ScreenshotTV:
		return true
	}
	return false
}

func validOrientation(value ScreenshotOrientation) bool {
	switch value {
	case OrientationPortrait, OrientationLandscape:
		return true
	}
	return false
}
