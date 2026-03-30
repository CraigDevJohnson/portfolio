package lps

import (
	"context"
	"errors"
	"net/http"
	"net/url"
)

func apiEndpoint(baseURL string, pathParts ...string) (string, error) {
	return url.JoinPath(baseURL, pathParts...)
}

func newAPIRequest(ctx context.Context, baseURL, bearerToken string, pathParts ...string) (*http.Request, error) {
	endpoint, err := apiEndpoint(baseURL, pathParts...)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if err := validateAPIRequest(request); err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json, text/plain, */*")
	if bearerToken != "" {
		request.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	return request, nil
}

func validateAPIRequest(request *http.Request) error {
	if request == nil || request.URL == nil {
		return errors.New("LPS API request URL is required")
	}
	if request.URL.RawQuery != "" || request.URL.Fragment != "" {
		return errors.New("LPS API requests cannot include query or fragment")
	}
	return nil
}

func doAPIRequest(client *http.Client, request *http.Request) (*http.Response, error) {
	if err := validateAPIRequest(request); err != nil {
		return nil, err
	}
	return client.Do(request) //nolint:gosec // Request URLs are rebuilt from a validated base URL and revalidated here.
}
