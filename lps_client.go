// LPS API HTTP client: endpoint construction and authenticated request execution.
package main

import (
	"context"
	"errors"
	"net/http"
	"net/url"
)

func lpsAPIEndpoint(pathParts ...string) (string, error) {
	baseURL, err := normalizeLPSAPIBaseURL(configData.LPSAPIBaseURL)
	if err != nil {
		return "", err
	}
	return url.JoinPath(baseURL, pathParts...)
}

func newLPSAPIRequest(ctx context.Context, method, bearerToken string, pathParts ...string) (*http.Request, error) { //nolint:unparam // method kept general for future POST/PUT support
	endpoint, err := lpsAPIEndpoint(pathParts...)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if err := validateLPSAPIRequest(req); err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	return req, nil
}

func validateLPSAPIRequest(req *http.Request) error {
	if req == nil || req.URL == nil {
		return errors.New("LPS API request URL is required")
	}

	if req.URL.RawQuery != "" || req.URL.Fragment != "" {
		return errors.New("LPS API requests cannot include query or fragment")
	}
	return nil
}

func doLPSAPIRequest(req *http.Request) (*http.Response, error) {
	if err := validateLPSAPIRequest(req); err != nil {
		return nil, err
	}
	return lpsHTTPClient.Do(req) //nolint:gosec // Request URLs are rebuilt from normalizeLPSAPIBaseURL and revalidated here.
}
