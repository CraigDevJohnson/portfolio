// LPS API HTTP client: endpoint construction and authenticated request execution.
package app

import (
	"context"
	"net/http"

	internallps "portfolio/internal/lps"
)

func lpsAPIEndpoint(pathParts ...string) (string, error) {
	baseURL, err := normalizeLPSAPIBaseURL(configData.LPSAPIBaseURL)
	if err != nil {
		return "", err
	}
	return internallps.APIEndpoint(baseURL, pathParts...)
}

func newLPSAPIRequest(ctx context.Context, method, bearerToken string, pathParts ...string) (*http.Request, error) { //nolint:unparam // method kept general for future POST/PUT support
	baseURL, err := normalizeLPSAPIBaseURL(configData.LPSAPIBaseURL)
	if err != nil {
		return nil, err
	}
	return internallps.NewAPIRequest(ctx, baseURL, method, bearerToken, pathParts...)
}

func validateLPSAPIRequest(req *http.Request) error {
	return internallps.ValidateAPIRequest(req)
}

func doLPSAPIRequest(req *http.Request) (*http.Response, error) {
	return internallps.DoAPIRequest(lpsHTTPClient, req)
}
