package rustore

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"sync"
	"time"

	"github.com/vyachin/rustore-publisher/internal/exitcode"
)

const (
	// maxResponseBytes ограничивает тело ответа чтения.
	maxResponseBytes = 8 << 20
	// maxRetryDelay ограничивает паузу между повторами.
	maxRetryDelay = 30 * time.Second
)

// Credentials — ключ API RuStore.
type Credentials struct {
	KeyID string
	Key   *rsa.PrivateKey
}

// Config — зависимости клиента.
type Config struct {
	BaseURL        string
	HTTPClient     *http.Client
	Credentials    Credentials
	RequestTimeout time.Duration
	Now            func() time.Time
	Sleep          func(context.Context, time.Duration) error
	MaxAttempts    int
	RetryBase      time.Duration
	Logger         *slog.Logger
}

// Client — небольшой клиент Public API RuStore.
type Client struct {
	cfg         Config
	mu          sync.Mutex
	cachedToken string
	expires     time.Time
}

// NewClient создаёт клиент с безопасными значениями по умолчанию.
func NewClient(cfg Config) (*Client, error) {
	if cfg.Credentials.Key == nil || cfg.Credentials.KeyID == "" {
		return nil, exitcode.Errorf(exitcode.KindInternal, "клиент RuStore создан без учётных данных")
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{}
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Sleep == nil {
		cfg.Sleep = sleepContext
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.RetryBase <= 0 {
		cfg.RetryBase = 200 * time.Millisecond
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Client{cfg: cfg}, nil
}

// do выполняет запрос к API с обновлением токена и ограниченными повторами.
func (c *Client) do(ctx context.Context, method, path string, query map[string]string, body []byte, op operation) (*envelope, error) {
	token, err := c.token(ctx, false)
	if err != nil {
		return nil, err
	}

	refreshed := false
	var lastErr error
	var lastStatus int

	for attempt := 1; attempt <= c.cfg.MaxAttempts; attempt++ {
		env, status, retryAfter, err := c.send(ctx, method, path, query, body, token)
		if err == nil {
			return env, nil
		}
		lastErr, lastStatus = err, status

		if status == http.StatusUnauthorized && !refreshed {
			refreshed = true
			token, err = c.token(ctx, true)
			if err != nil {
				return nil, err
			}
			continue
		}

		if attempt == c.cfg.MaxAttempts || !retryable(status, err) {
			break
		}
		if sleepErr := c.cfg.Sleep(ctx, c.backoff(attempt, retryAfter)); sleepErr != nil {
			return nil, sleepErr
		}
	}

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if lastStatus == 0 && errors.Is(lastErr, context.DeadlineExceeded) {
		return nil, exitcode.WithHint(
			exitcode.Wrap(exitcode.KindRequestTimeout, lastErr),
			"увеличьте --request-timeout или повторите команду",
		)
	}

	var apiErr *APIError
	if errors.As(lastErr, &apiErr) {
		return nil, classifyAPIError(op, apiErr)
	}
	var coded *exitcode.Error
	if errors.As(lastErr, &coded) {
		return nil, lastErr
	}
	return nil, exitcode.WithHint(
		exitcode.Wrap(exitcode.KindAPIUnavailable, lastErr),
		"повторите команду позже или проверьте доступность API RuStore",
	)
}

// bodyReader возвращает тело запроса или nil для пустого тела.
func bodyReader(body []byte) io.Reader {
	if len(body) == 0 {
		return nil
	}
	return bytes.NewReader(body)
}

// looksJSON сообщает, похоже ли тело на JSON.
func looksJSON(data []byte) bool {
	return json.Valid(data)
}

// randomInt64 возвращает случайное число из [0, n).
func randomInt64(n int64) int64 {
	if n <= 0 {
		return 0
	}
	return rand.Int64N(n)
}
