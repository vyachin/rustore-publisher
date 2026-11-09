package cli

import (
	"net/url"
	"os"
	"strings"

	"github.com/vyachin/rustore-publisher/internal/app"
	"github.com/vyachin/rustore-publisher/internal/exitcode"
	"github.com/vyachin/rustore-publisher/internal/rustore"
)

// newRustoreClient загружает учётные данные и собирает клиент API.
func newRustoreClient(a *App, g *globalFlags) (*rustore.Client, error) {
	baseURL, err := resolveAPIBaseURL(a, g)
	if err != nil {
		return nil, err
	}

	path := app.ResolveCredentialsPath(g.Credentials)
	credentials, err := app.LoadCredentials(path)
	if err != nil {
		return nil, err
	}

	return rustore.NewClient(rustore.Config{
		BaseURL: baseURL,
		Credentials: rustore.Credentials{
			KeyID: credentials.KeyID,
			Key:   credentials.Key,
		},
		RequestTimeout:    g.RequestTimeout,
		Now:               a.Now,
		Logger:            a.Logger,
		DisableEmbeddedCA: embeddedCADisabled(g),
	})
}

// resolveAPIBaseURL выбирает адрес API: тестовое переопределение, флаг,
// переменная окружения или старый домен с публичным сертификатом.
func resolveAPIBaseURL(a *App, g *globalFlags) (string, error) {
	if a.APIBaseURL != "" {
		return a.APIBaseURL, nil
	}

	value := g.APIURL
	if value == "" {
		value = os.Getenv("RUSTORE_API_URL")
	}
	if value == "" {
		return rustore.DefaultBaseURL, nil
	}

	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", exitcode.Errorf(exitcode.KindInvalidArguments,
			"--api-url должен быть HTTPS-адресом, получено %q", value)
	}
	return strings.TrimRight(value, "/"), nil
}

// embeddedCADisabled учитывает флаг и переменную окружения.
func embeddedCADisabled(g *globalFlags) bool {
	if g.NoEmbeddedCA {
		return true
	}
	value := strings.ToLower(strings.TrimSpace(os.Getenv("RUSTORE_NO_EMBEDDED_CA")))
	return value == "1" || value == "true" || value == "yes"
}
