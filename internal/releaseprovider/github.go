package releaseprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type GitHubProvider struct {
	APIBase    string
	Owner      string
	Repository string
	Token      string
	HTTPClient *http.Client
}

type Release struct {
	ID         int64   `json:"id"`
	TagName    string  `json:"tag_name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`
}

type Asset struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	APIURL string `json:"url"`
	Size   int64  `json:"size"`
}

func (provider GitHubProvider) ListReleases(ctx context.Context) ([]Release, error) {
	if provider.Owner == "" || provider.Repository == "" {
		return nil, errors.New("private release repository is not configured")
	}
	endpoint := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=30",
		strings.TrimRight(provider.apiBase(), "/"), url.PathEscape(provider.Owner), url.PathEscape(provider.Repository))
	request, err := provider.request(ctx, http.MethodGet, endpoint)
	if err != nil {
		return nil, err
	}
	response, err := provider.httpClient().Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("private release provider returned HTTP %d", response.StatusCode)
	}
	var releases []Release
	decoder := json.NewDecoder(io.LimitReader(response.Body, 4<<20))
	if err := decoder.Decode(&releases); err != nil {
		return nil, err
	}
	return releases, nil
}

func (provider GitHubProvider) FetchAsset(ctx context.Context, asset Asset, destination string) error {
	if asset.Name == "" || asset.Name != filepath.Base(asset.Name) || strings.ContainsAny(asset.Name, `/\`) {
		return errors.New("provider asset name is unsafe")
	}
	assetURL, err := url.Parse(asset.APIURL)
	if err != nil || (assetURL.Scheme != "https" && !isLoopbackHTTP(assetURL)) {
		return errors.New("provider asset URL is unsafe")
	}
	apiURL, err := url.Parse(provider.apiBase())
	if err != nil || assetURL.Host != apiURL.Host {
		return errors.New("provider asset API URL is outside the configured API host")
	}
	if asset.Size > 1<<30 {
		return errors.New("provider asset exceeds 1 GiB")
	}
	request, err := provider.request(ctx, http.MethodGet, asset.APIURL)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/octet-stream")
	client := provider.redirectSafeClient()
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("provider asset returned HTTP %d", response.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	success := false
	defer func() {
		if !success {
			_ = os.Remove(destination)
		}
	}()
	written, copyErr := io.Copy(file, io.LimitReader(response.Body, 1<<30+1))
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if written > 1<<30 {
		return errors.New("provider asset exceeds 1 GiB")
	}
	if asset.Size > 0 && written != asset.Size {
		return fmt.Errorf("provider asset size mismatch for %s", asset.Name)
	}
	if closeErr != nil {
		return closeErr
	}
	success = true
	return nil
}

func (provider GitHubProvider) request(ctx context.Context, method, endpoint string) (*http.Request, error) {
	if provider.Token == "" {
		return nil, errors.New("provider access token is unavailable")
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Authorization", "Bearer "+provider.Token)
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	return request, nil
}

func (provider GitHubProvider) redirectSafeClient() *http.Client {
	base := provider.httpClient()
	copyClient := *base
	original := base.CheckRedirect
	copyClient.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("too many provider redirects")
		}
		if len(via) > 0 && request.URL.Host != via[0].URL.Host {
			request.Header.Del("Authorization")
			request.Header.Del("X-GitHub-Api-Version")
		}
		if original != nil {
			return original(request, via)
		}
		return nil
	}
	return &copyClient
}

func (provider GitHubProvider) httpClient() *http.Client {
	if provider.HTTPClient != nil {
		return provider.HTTPClient
	}
	return &http.Client{Timeout: 2 * time.Minute}
}

func (provider GitHubProvider) apiBase() string {
	if provider.APIBase != "" {
		return provider.APIBase
	}
	return "https://api.github.com"
}

func isLoopbackHTTP(value *url.URL) bool {
	return value.Scheme == "http" && (value.Hostname() == "127.0.0.1" || value.Hostname() == "localhost" || value.Hostname() == "::1")
}
