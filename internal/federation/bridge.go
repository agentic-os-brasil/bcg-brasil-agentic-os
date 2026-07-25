package federation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// HTTPBridge is a narrow pilot-device adapter. It speaks only to the
// organization-operated bridge over HTTPS; that bridge, not the device, owns
// GitHub App authentication and GitHub API access.
type HTTPBridge struct {
	endpoint string
	client   *http.Client
}

func NewHTTPBridge(endpoint string, client *http.Client) (*HTTPBridge, error) {
	if err := validateBridgeEndpoint(endpoint); err != nil {
		return nil, err
	}
	if client == nil {
		client = &http.Client{}
	}
	clone := *client
	// A redirect could turn an initially safe endpoint into an uncontrolled
	// destination. The bridge protocol has one stable endpoint instead.
	clone.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &HTTPBridge{endpoint: endpoint, client: &clone}, nil
}

func (bridge *HTTPBridge) Submit(ctx context.Context, batch Batch) error {
	if bridge == nil || bridge.client == nil {
		return errors.New("federation HTTP bridge is not configured")
	}
	if err := batch.Validate(); err != nil {
		return err
	}
	payload, err := json.Marshal(batch)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, bridge.endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-Maestro-Federation-Schema", fmt.Sprintf("%d", SchemaVersion))
	response, err := bridge.client.Do(request)
	if err != nil {
		return errors.New("federation bridge is unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("federation bridge returned status %d", response.StatusCode)
	}
	return nil
}

func parseHTTPSURL(value string) (*url.URL, error) {
	endpoint, err := url.Parse(value)
	if err != nil || endpoint.Scheme != "https" || endpoint.Hostname() == "" || endpoint.Host == "" || endpoint.Opaque != "" {
		return nil, errors.New("federation bridge endpoint must be a credential-free HTTPS URL")
	}
	return endpoint, nil
}
