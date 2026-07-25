package federation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// GitHubAppTokenSource is central-bridge-only. Implementations exchange the
// organization GitHub App key for a short-lived installation token; pilot
// devices never receive this interface, its key or its token.
type GitHubAppTokenSource interface {
	InstallationToken(context.Context) (string, error)
}

type GitHubAppIssuePublisher struct {
	apiBase string
	owner   string
	repo    string
	tokens  GitHubAppTokenSource
	client  *http.Client
}

func NewGitHubAppIssuePublisher(apiBase, owner, repo string, tokens GitHubAppTokenSource, client *http.Client) (*GitHubAppIssuePublisher, error) {
	parsed, err := url.Parse(apiBase)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("GitHub App issue publisher requires a credential-free HTTPS API base")
	}
	if !repositorySegment(owner) || !repositorySegment(repo) || tokens == nil {
		return nil, errors.New("invalid GitHub App issue publisher configuration")
	}
	if client == nil {
		client = &http.Client{}
	}
	clone := *client
	clone.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &GitHubAppIssuePublisher{apiBase: strings.TrimRight(apiBase, "/"), owner: owner, repo: repo, tokens: tokens, client: &clone}, nil
}

func (publisher *GitHubAppIssuePublisher) Publish(ctx context.Context, issue ProposalIssue) error {
	if publisher == nil || publisher.tokens == nil || publisher.client == nil || strings.TrimSpace(issue.Title) == "" || strings.TrimSpace(issue.Body) == "" {
		return errors.New("invalid GitHub App issue publication")
	}
	token, err := publisher.tokens.InstallationToken(ctx)
	if err != nil || strings.TrimSpace(token) == "" {
		return errors.New("GitHub App installation token unavailable")
	}
	payload, err := json.Marshal(struct {
		Title  string   `json:"title"`
		Body   string   `json:"body"`
		Labels []string `json:"labels"`
	}{Title: issue.Title, Body: issue.Body, Labels: issue.Labels})
	if err != nil {
		return err
	}
	endpoint := publisher.apiBase + "/repos/" + publisher.owner + "/" + publisher.repo + "/issues"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := publisher.client.Do(request)
	if err != nil {
		return errors.New("GitHub App issue publication unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		return fmt.Errorf("GitHub App issue publication returned status %d", response.StatusCode)
	}
	return nil
}

func repositorySegment(value string) bool {
	if value == "" || len(value) > 100 {
		return false
	}
	for _, character := range value {
		if !(character == '-' || character == '_' || character == '.' || (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9')) {
			return false
		}
	}
	return true
}
