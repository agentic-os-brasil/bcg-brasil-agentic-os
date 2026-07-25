package releaseprovider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestDeviceFlowHandlesPendingWithoutLeakingTokens(t *testing.T) {
	polls := 0
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/login/device/code":
			assertForm(t, request, "client_id", "client-123")
			return testJSONResponse(t, map[string]any{
				"device_code": "device-secret", "user_code": "ABCD-EFGH",
				"verification_uri": "https://github.example/device", "expires_in": 900, "interval": 1,
			}), nil
		case "/login/oauth/access_token":
			polls++
			assertForm(t, request, "device_code", "device-secret")
			if polls == 1 {
				return testJSONResponse(t, map[string]any{"error": "authorization_pending"}), nil
			}
			return testJSONResponse(t, map[string]any{
				"access_token": "access-secret", "refresh_token": "refresh-secret",
				"expires_in": 3600, "refresh_token_expires_in": 15897600, "token_type": "bearer", "scope": "",
			}), nil
		default:
			return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
	})}

	client := DeviceFlowClient{
		ClientID: "client-123", BaseURL: "https://github.example", HTTPClient: httpClient,
		Sleep: func(context.Context, time.Duration) error { return nil },
		Now:   func() time.Time { return time.Unix(1000, 0).UTC() },
	}
	authorization, err := client.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	token, err := client.Poll(context.Background(), authorization)
	if err != nil {
		t.Fatal(err)
	}
	if token.AccessToken != "access-secret" || token.RefreshToken != "refresh-secret" || polls != 2 {
		t.Fatalf("unexpected token result: polls=%d token=%#v", polls, token.Redacted())
	}
	if strings.Contains(token.Redacted(), "secret") {
		t.Fatal("redacted token representation leaked a secret")
	}
}

func TestAuthServiceRequiresSecureStoreBeforeStartingDeviceFlow(t *testing.T) {
	requests := 0
	httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests++
		return testJSONResponse(t, map[string]any{}), nil
	})}
	service := AuthService{
		Flow:  DeviceFlowClient{ClientID: "client", BaseURL: "https://github.example", HTTPClient: httpClient},
		Store: UnavailableStore{},
	}
	if _, err := service.Login(context.Background(), func(DeviceAuthorization) error { return nil }); err == nil {
		t.Fatal("Login() accepted unavailable secure storage")
	}
	if requests != 0 {
		t.Fatalf("device flow started before secure storage was available: %d requests", requests)
	}
}

func TestAssetRedirectDropsAuthorizationOnDifferentHost(t *testing.T) {
	provider := GitHubProvider{HTTPClient: &http.Client{}, Token: "provider-token"}
	redirect := provider.redirectSafeClient().CheckRedirect
	previous, err := http.NewRequest(http.MethodGet, "https://api.github.com/repos/owner/repo/releases/assets/1", nil)
	if err != nil {
		t.Fatal(err)
	}
	next, err := http.NewRequest(http.MethodGet, "https://objects.githubusercontent.com/artifact", nil)
	if err != nil {
		t.Fatal(err)
	}
	next.Header.Set("Authorization", "Bearer provider-token")
	next.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if err := redirect(next, []*http.Request{previous}); err != nil {
		t.Fatal(err)
	}
	if next.Header.Get("Authorization") != "" || next.Header.Get("X-GitHub-Api-Version") != "" {
		t.Fatal("authorization was forwarded to the redirected asset host")
	}
}

func assertForm(t *testing.T, request *http.Request, key, expected string) {
	t.Helper()
	body, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatal(err)
	}
	values, err := url.ParseQuery(string(body))
	if err != nil {
		t.Fatal(err)
	}
	if values.Get(key) != expected {
		t.Fatalf("form %s = %q, want %q", key, values.Get(key), expected)
	}
}

func testJSONResponse(t *testing.T, value any) *http.Response {
	t.Helper()
	var body strings.Builder
	if err := json.NewEncoder(&body).Encode(value); err != nil {
		t.Fatal(err)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body.String())),
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
