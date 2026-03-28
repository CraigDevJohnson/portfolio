// LPS API HTTP client: endpoint construction and authenticated request execution.
package app

import (
	"context"
	"net/http"

	internallps "portfolio/internal/lps"
)

func (app *App) lpsAPIEndpoint(pathParts ...string) (string, error) {
	baseURL, err := normalizeLPSAPIBaseURL(app.Config.LPSAPIBaseURL)
	if err != nil {
		return "", err
	}
	return internallps.APIEndpoint(baseURL, pathParts...)
}

func (app *App) newLPSAPIRequest(ctx context.Context, method, bearerToken string, pathParts ...string) (*http.Request, error) { //nolint:unparam // method kept general for future POST/PUT support
	baseURL, err := normalizeLPSAPIBaseURL(app.Config.LPSAPIBaseURL)
	if err != nil {
		return nil, err
	}
	return internallps.NewAPIRequest(ctx, baseURL, method, bearerToken, pathParts...)
}

func validateLPSAPIRequest(req *http.Request) error {
	return internallps.ValidateAPIRequest(req)
}

func (app *App) doLPSAPIRequest(req *http.Request) (*http.Response, error) {
	return internallps.DoAPIRequest(app.LPSClient, req)
}
