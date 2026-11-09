package cli

import (
	"github.com/vyachin/rustore-publisher/internal/app"
	"github.com/vyachin/rustore-publisher/internal/rustore"
)

// newRustoreClient загружает учётные данные и собирает клиент API.
func newRustoreClient(a *App, g *globalFlags) (*rustore.Client, error) {
	baseURL := a.APIBaseURL
	if baseURL == "" {
		baseURL = rustore.DefaultBaseURL
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
		RequestTimeout: g.RequestTimeout,
		Now:            a.Now,
		Logger:         a.Logger,
	})
}
