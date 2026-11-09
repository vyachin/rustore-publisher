package rustore

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/vyachin/rustore-publisher/internal/exitcode"
)

// authTimestampLayout — формат timestamp из документации: миллисекунды
// и числовое смещение часового пояса.
const authTimestampLayout = "2006-01-02T15:04:05.000-07:00"

// tokenRefreshMargin — запас, за который токен обновляется до истечения.
const tokenRefreshMargin = 60 * time.Second

// authRequest — тело запроса на получение токена.
type authRequest struct {
	KeyID     string `json:"keyId"`
	Timestamp string `json:"timestamp"`
	Signature string `json:"signature"`
}

// authBody — тело успешного ответа авторизации.
type authBody struct {
	JWE string `json:"jwe"`
	TTL int    `json:"ttl"`
}

// SignAuth подписывает строку keyId+timestamp алгоритмом SHA512withRSA
// и кодирует подпись в Base64.
func SignAuth(keyID string, key *rsa.PrivateKey, timestamp string) (string, error) {
	sum := sha512.Sum512([]byte(keyID + timestamp))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA512, sum[:])
	if err != nil {
		return "", exitcode.Wrap(exitcode.KindInternal, fmt.Errorf("подписать timestamp: %w", err))
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}

// authorize получает новый токен и возвращает его срок жизни.
func (c *Client) authorize(ctx context.Context) (string, time.Duration, error) {
	timestamp := c.cfg.Now().Format(authTimestampLayout)
	signature, err := SignAuth(c.cfg.Credentials.KeyID, c.cfg.Credentials.Key, timestamp)
	if err != nil {
		return "", 0, err
	}

	payload, err := json.Marshal(authRequest{
		KeyID:     c.cfg.Credentials.KeyID,
		Timestamp: timestamp,
		Signature: signature,
	})
	if err != nil {
		return "", 0, exitcode.Wrap(exitcode.KindInternal, err)
	}

	var lastErr error
	for attempt := 1; attempt <= c.cfg.MaxAttempts; attempt++ {
		env, status, retryAfter, err := c.send(ctx, http.MethodPost, "/public/auth/", nil, payload, "")
		if err == nil {
			var body authBody
			if err := json.Unmarshal(env.Body, &body); err != nil {
				return "", 0, exitcode.WithHint(
					exitcode.Wrap(exitcode.KindAPIResponseInvalid,
						fmt.Errorf("разобрать ответ авторизации: %w", err)),
					"повторите запрос; если ошибка повторяется, проверьте доступность API",
				)
			}
			if body.JWE == "" {
				return "", 0, exitcode.Errorf(exitcode.KindAPIResponseInvalid,
					"ответ авторизации не содержит токен")
			}
			ttl := time.Duration(body.TTL) * time.Second
			if body.TTL <= int(tokenRefreshMargin/time.Second) {
				return "", 0, exitcode.WithHint(
					exitcode.Errorf(exitcode.KindAuthFailed,
						"срок жизни токена %d секунд слишком мал", body.TTL),
					"повторите авторизацию; если ошибка повторяется, обратитесь в поддержку RuStore",
				)
			}
			return body.JWE, ttl, nil
		}

		if status == 401 || status == 400 {
			return "", 0, exitcode.WithHint(
				exitcode.Wrap(exitcode.KindAuthFailed, err),
				"проверьте keyId и приватный ключ, а также системное время",
			)
		}
		lastErr = err
		if attempt == c.cfg.MaxAttempts || !retryable(status, err) {
			break
		}
		if sleepErr := c.cfg.Sleep(ctx, c.backoff(attempt, retryAfter)); sleepErr != nil {
			return "", 0, sleepErr
		}
	}

	if ctx.Err() != nil {
		return "", 0, ctx.Err()
	}
	return "", 0, exitcode.WithHint(
		exitcode.Wrap(exitcode.KindAPIUnavailable, lastErr),
		apiUnavailableHint(lastErr),
	)
}

// Token возвращает действующий токен, получая его при необходимости.
func (c *Client) Token(ctx context.Context) (string, error) {
	return c.token(ctx, false)
}

// RefreshToken принудительно обновляет токен.
func (c *Client) RefreshToken(ctx context.Context) (string, error) {
	return c.token(ctx, true)
}

// token возвращает действующий токен, при необходимости обновляя его.
// force обновляет токен даже при достаточном запасе времени.
func (c *Client) token(ctx context.Context, force bool) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !force && c.cachedToken != "" && c.cfg.Now().Before(c.expires.Add(-tokenRefreshMargin)) {
		return c.cachedToken, nil
	}

	token, ttl, err := c.authorize(ctx)
	if err != nil {
		return "", err
	}
	c.cachedToken = token
	c.expires = c.cfg.Now().Add(ttl)
	return token, nil
}

// send выполняет один HTTP-запрос. Для ответов об ошибке возвращает
// *APIError, для сетевых сбоев — исходную ошибку.
func (c *Client) send(ctx context.Context, method, path string, query map[string]string, body []byte, token string) (*envelope, int, time.Duration, error) {
	requestCtx := ctx
	cancel := func() {}
	if c.cfg.RequestTimeout > 0 {
		requestCtx, cancel = context.WithTimeout(ctx, c.cfg.RequestTimeout)
	}
	defer cancel()

	url := c.cfg.BaseURL + path
	request, err := http.NewRequestWithContext(requestCtx, method, url, bodyReader(body))
	if err != nil {
		return nil, 0, 0, exitcode.Wrap(exitcode.KindInternal, err)
	}
	if len(body) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("Accept", "application/json")
	if token != "" {
		request.Header.Set("Public-Token", token)
	}
	if len(query) > 0 {
		values := request.URL.Query()
		for key, value := range query {
			values.Set(key, value)
		}
		request.URL.RawQuery = values.Encode()
	}

	c.cfg.Logger.DebugContext(ctx, "запрос к RuStore",
		"method", method, "path", path, "attempt_query", len(query))

	response, err := c.cfg.HTTPClient.Do(request)
	if err != nil {
		return nil, 0, 0, err
	}
	defer func() { _ = response.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return nil, response.StatusCode, 0, fmt.Errorf("прочитать ответ: %w", err)
	}

	retryAfter := parseRetryAfter(response.Header.Get("Retry-After"), c.cfg.Now())

	if response.StatusCode >= http.StatusBadRequest {
		apiErr := &APIError{Status: response.StatusCode}
		var parsed envelope
		if json.Unmarshal(data, &parsed) == nil {
			apiErr.Code = parsed.Code
			if parsed.Message != nil {
				apiErr.Message = *parsed.Message
			}
		}
		if apiErr.Message == "" && len(data) > 0 && len(data) < 500 && !looksJSON(data) {
			apiErr.Message = string(data)
		}
		return nil, response.StatusCode, retryAfter, apiErr
	}

	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, response.StatusCode, retryAfter, exitcode.WithHint(
			exitcode.Wrap(exitcode.KindAPIResponseInvalid,
				fmt.Errorf("разобрать ответ %s: %w", path, err)),
			"повторите команду; если ошибка повторяется, проверьте доступность API",
		)
	}
	if env.Code != "OK" {
		apiErr := &APIError{Status: response.StatusCode, Code: env.Code}
		if env.Message != nil {
			apiErr.Message = *env.Message
		}
		return nil, response.StatusCode, retryAfter, apiErr
	}
	return &env, response.StatusCode, retryAfter, nil
}

// retryable сообщает, имеет ли смысл повторить безопасный запрос.
func retryable(status int, err error) bool {
	if status == http.StatusTooManyRequests || status >= http.StatusInternalServerError {
		return true
	}
	if status != 0 {
		return false
	}
	// Сетевые ошибки не имеют HTTP-статуса.
	var apiErr *APIError
	return !errors.As(err, &apiErr)
}

// backoff вычисляет задержку перед повтором с учётом Retry-After.
func (c *Client) backoff(attempt int, retryAfter time.Duration) time.Duration {
	delay := c.cfg.RetryBase << (attempt - 1)
	if delay > maxRetryDelay {
		delay = maxRetryDelay
	}
	if retryAfter > delay {
		delay = retryAfter
	}
	if delay > maxRetryDelay {
		delay = maxRetryDelay
	}
	jitter := time.Duration(randomInt64(int64(delay/2) + 1))
	return delay - jitter
}

// sleepContext ждёт указанное время или отмену контекста.
func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// parseRetryAfter разбирает заголовок Retry-After в секундах или дате.
func parseRetryAfter(value string, now time.Time) time.Duration {
	if value == "" {
		return 0
	}
	var seconds int
	if _, err := fmt.Sscanf(value, "%d", &seconds); err == nil {
		return time.Duration(seconds) * time.Second
	}
	if parsed, err := http.ParseTime(value); err == nil {
		if delay := parsed.Sub(now); delay > 0 {
			return delay
		}
	}
	return 0
}
