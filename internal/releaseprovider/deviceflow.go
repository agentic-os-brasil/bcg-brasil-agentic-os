package releaseprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type DeviceFlowClient struct {
	ClientID   string
	BaseURL    string
	HTTPClient *http.Client
	Sleep      func(context.Context, time.Duration) error
	Now        func() time.Time
}

type DeviceAuthorization struct {
	DeviceCode      string
	UserCode        string
	VerificationURI string
	ExpiresAt       time.Time
	Interval        time.Duration
}

type Token struct {
	AccessToken      string    `json:"access_token"`
	RefreshToken     string    `json:"refresh_token"`
	TokenType        string    `json:"token_type"`
	ExpiresAt        time.Time `json:"expires_at"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
}

func (token Token) Redacted() string {
	return fmt.Sprintf("token(type=%s, access=%s, refresh=%s, expires=%s)",
		token.TokenType, presence(token.AccessToken), presence(token.RefreshToken), token.ExpiresAt.UTC().Format(time.RFC3339))
}

func (client DeviceFlowClient) Begin(ctx context.Context) (DeviceAuthorization, error) {
	if strings.TrimSpace(client.ClientID) == "" {
		return DeviceAuthorization{}, errors.New("GitHub App client ID is unavailable")
	}
	var response struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
	}
	if err := client.postForm(ctx, "/login/device/code", url.Values{"client_id": {client.ClientID}}, &response); err != nil {
		return DeviceAuthorization{}, err
	}
	verificationURL, err := url.Parse(response.VerificationURI)
	if err != nil || verificationURL.Scheme != "https" || verificationURL.Host == "" {
		return DeviceAuthorization{}, errors.New("device flow returned an unsafe verification URI")
	}
	if response.DeviceCode == "" || response.UserCode == "" || response.ExpiresIn <= 0 {
		return DeviceAuthorization{}, errors.New("device flow returned an incomplete authorization")
	}
	interval := time.Duration(response.Interval) * time.Second
	if interval < time.Second {
		interval = time.Second
	}
	return DeviceAuthorization{
		DeviceCode: response.DeviceCode, UserCode: response.UserCode,
		VerificationURI: response.VerificationURI, ExpiresAt: client.now().Add(time.Duration(response.ExpiresIn) * time.Second),
		Interval: interval,
	}, nil
}

func (client DeviceFlowClient) Poll(ctx context.Context, authorization DeviceAuthorization) (Token, error) {
	interval := authorization.Interval
	for {
		if !client.now().Before(authorization.ExpiresAt) {
			return Token{}, errors.New("device authorization expired")
		}
		if err := client.sleep(ctx, interval); err != nil {
			return Token{}, err
		}
		var response tokenResponse
		err := client.postForm(ctx, "/login/oauth/access_token", url.Values{
			"client_id":   {client.ClientID},
			"device_code": {authorization.DeviceCode},
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		}, &response)
		if err != nil {
			return Token{}, err
		}
		switch response.Error {
		case "":
			return response.token(client.now())
		case "authorization_pending":
			continue
		case "slow_down":
			interval += 5 * time.Second
			continue
		case "access_denied", "expired_token":
			return Token{}, fmt.Errorf("device authorization failed: %s", response.Error)
		default:
			return Token{}, fmt.Errorf("device authorization failed: %s", response.Error)
		}
	}
}

func (client DeviceFlowClient) Refresh(ctx context.Context, refreshToken string) (Token, error) {
	if refreshToken == "" {
		return Token{}, errors.New("refresh token is unavailable")
	}
	var response tokenResponse
	if err := client.postForm(ctx, "/login/oauth/access_token", url.Values{
		"client_id":     {client.ClientID},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}, &response); err != nil {
		return Token{}, err
	}
	if response.Error != "" {
		return Token{}, fmt.Errorf("token refresh failed: %s", response.Error)
	}
	return response.token(client.now())
}

type tokenResponse struct {
	AccessToken           string `json:"access_token"`
	RefreshToken          string `json:"refresh_token"`
	TokenType             string `json:"token_type"`
	ExpiresIn             int    `json:"expires_in"`
	RefreshTokenExpiresIn int    `json:"refresh_token_expires_in"`
	Error                 string `json:"error"`
}

func (response tokenResponse) token(now time.Time) (Token, error) {
	if response.AccessToken == "" || response.TokenType == "" || response.ExpiresIn <= 0 {
		return Token{}, errors.New("token endpoint returned an incomplete token")
	}
	token := Token{
		AccessToken: response.AccessToken, RefreshToken: response.RefreshToken, TokenType: response.TokenType,
		ExpiresAt: now.Add(time.Duration(response.ExpiresIn) * time.Second),
	}
	if response.RefreshTokenExpiresIn > 0 {
		token.RefreshExpiresAt = now.Add(time.Duration(response.RefreshTokenExpiresIn) * time.Second)
	}
	return token, nil
}

func (client DeviceFlowClient) postForm(ctx context.Context, endpoint string, values url.Values, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(client.baseURL(), "/")+endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.httpClient().Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("device endpoint returned HTTP %d", response.StatusCode)
	}
	decoder := json.NewDecoder(response.Body)
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return nil
}

func (client DeviceFlowClient) httpClient() *http.Client {
	if client.HTTPClient != nil {
		return client.HTTPClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (client DeviceFlowClient) baseURL() string {
	if client.BaseURL != "" {
		return client.BaseURL
	}
	return "https://github.com"
}

func (client DeviceFlowClient) now() time.Time {
	if client.Now != nil {
		return client.Now()
	}
	return time.Now().UTC()
}

func (client DeviceFlowClient) sleep(ctx context.Context, duration time.Duration) error {
	if client.Sleep != nil {
		return client.Sleep(ctx, duration)
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func presence(value string) string {
	if value == "" {
		return "absent"
	}
	return "present"
}
