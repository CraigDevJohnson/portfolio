package lps

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"

	"portfolio/internal/config"
)

func newAPIRequest(ctx context.Context, baseURL, bearerToken string, pathParts ...string) (*http.Request, error) {
	endpoint, err := url.JoinPath(baseURL, pathParts...)
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
	return client.Do(request)
}

// statusErrorKind maps a non-standard LPS status code to the appropriate ErrorKind for the resource type.
type statusErrorKind struct {
	codes []int
	kind  ErrorKind
}

// executeAPIRequest sends an LPS API request, reads the response body, and
// classifies non-2xx status codes into a *FetchError. Callers pass resource-specific
// status mappings; 401→Unauthorized, 403→Forbidden, and remaining non-2xx→Upstream
// are always applied as fallbacks.
func executeAPIRequest(client *http.Client, req *http.Request, resourceID int, resourceMappings ...statusErrorKind) ([]byte, error) {
	resp, err := doAPIRequest(client, req)
	if err != nil {
		return nil, NewFetchError(ErrorUpstream, resourceID, http.StatusBadGateway, "could not reach Let's Play Soccer: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, config.MaxLPSResponseBodySize))
	if err != nil {
		return nil, NewFetchError(ErrorUpstream, resourceID, http.StatusBadGateway, "could not read the LPS response: %w", err)
	}

	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return body, nil
	}

	for _, mapping := range resourceMappings {
		for _, code := range mapping.codes {
			if resp.StatusCode == code {
				return nil, NewFetchError(mapping.kind, resourceID, resp.StatusCode, "Let's Play Soccer returned status %d", resp.StatusCode)
			}
		}
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return nil, NewFetchError(ErrorUnauthorized, resourceID, resp.StatusCode, "Let's Play Soccer rejected the imported token with status %d", resp.StatusCode)
	case http.StatusForbidden:
		return nil, NewFetchError(ErrorForbidden, resourceID, resp.StatusCode, "Let's Play Soccer denied access with status %d", resp.StatusCode)
	default:
		return nil, NewFetchError(ErrorUpstream, resourceID, resp.StatusCode, "Let's Play Soccer returned status %d", resp.StatusCode)
	}
}
