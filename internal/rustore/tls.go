package rustore

import (
	"crypto/tls"
	"crypto/x509"
	"embed"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/vyachin/rustore-publisher/internal/exitcode"
)

// Сертификаты Минцифры встроены в бинарный файл: пользователю не нужно
// передавать SSL_CERT_FILE или устанавливать CA в систему.
//
//go:embed certs/*.crt
var embeddedCerts embed.FS

var (
	rootCAsOnce sync.Once
	rootCAsPool *x509.CertPool
	rootCAsErr  error
)

// rootCAs возвращает системные корни вместе с официальными сертификатами
// Минцифры. Пул используется только HTTP-клиентом RuStore.
func rootCAs() (*x509.CertPool, error) {
	rootCAsOnce.Do(func() {
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}

		entries, err := embeddedCerts.ReadDir("certs")
		if err != nil {
			rootCAsErr = fmt.Errorf("прочитать встроенные сертификаты: %w", err)
			return
		}

		added := 0
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".crt") {
				continue
			}
			data, err := embeddedCerts.ReadFile("certs/" + entry.Name())
			if err != nil {
				rootCAsErr = fmt.Errorf("прочитать %s: %w", entry.Name(), err)
				return
			}
			if !pool.AppendCertsFromPEM(data) {
				rootCAsErr = fmt.Errorf("сертификат %s не разобран", entry.Name())
				return
			}
			added++
		}
		if added == 0 {
			rootCAsErr = fmt.Errorf("встроенные сертификаты не найдены")
			return
		}
		rootCAsPool = pool
	})
	return rootCAsPool, rootCAsErr
}

// newHTTPClient создаёт HTTP-клиент RuStore с встроенными сертификатами.
// Редиректы запрещены, чтобы доверие Минцифры не распространялось на другие
// хосты, куда мог бы перенаправить ответ.
func newHTTPClient() (*http.Client, error) {
	pool, err := rootCAs()
	if err != nil {
		return nil, exitcode.Wrap(exitcode.KindInternal, err)
	}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout: 10 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 2 * time.Minute,
		ForceAttemptHTTP2:     true,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			RootCAs:    pool,
		},
	}

	return &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return fmt.Errorf("редиректы API не поддерживаются")
		},
	}, nil
}
