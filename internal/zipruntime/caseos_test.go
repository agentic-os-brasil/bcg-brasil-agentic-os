package zipruntime

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestCaseOSCheckRejectsUntrustedEndpoint(t *testing.T) {
	var out bytes.Buffer
	err := Run([]string{"caseos-check"}, strings.NewReader("{\"endpoint\":\"http://bad.example/path\"}"), &out)
	if err == nil || !strings.Contains(err.Error(), "official HTTPS") {
		t.Fatalf("wrong preflight result: %v", err)
	}
}

type fakeConnector struct {
	called    int
	unhealthy bool
	tools     []RemoteTool
}

func (f *fakeConnector) Discover(context.Context, string) ([]RemoteTool, error) {
	if f.unhealthy {
		return nil, errors.New("offline secret")
	}
	return f.tools, nil
}
func (f *fakeConnector) Call(context.Context, string, string, map[string]any) error {
	f.called++
	return nil
}
func TestCaseOSRoutesOnlyConsentedScopedRead(t *testing.T) {
	for _, scenario := range []string{"ok", "no-consent", "wrong-case", "wrong-workspace", "export", "offline", "unknown-tool", "no-schema", "personal"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			p := CaseOSPolicy{Endpoint: "https://caseos.production.mcp.bcg.com/official-path", Consent: true, Workspace: root, CaseID: "case-fixture", ReadTools: []string{"query"}, CaseParameters: map[string]string{"query": "case_id"}}
			req := CaseOSRequest{Workspace: root, CaseID: "case-fixture", Tool: "query", Arguments: map[string]any{"case_id": "case-fixture", "query": "status"}, CaseArgument: "case_id"}
			f := &fakeConnector{tools: []RemoteTool{{Name: "query", ReadOnly: true, Schema: []byte("{\"type\":\"object\",\"properties\":{\"case_id\":{\"type\":\"string\"},\"query\":{\"type\":\"string\"}},\"required\":[\"case_id\",\"query\"],\"additionalProperties\":false}")}}}
			switch scenario {
			case "no-consent":
				p.Consent = false
			case "wrong-case":
				req.Arguments["case_id"] = "other"
			case "wrong-workspace":
				req.Workspace = t.TempDir()
			case "export":
				req.Export = true
			case "offline":
				f.unhealthy = true
			case "unknown-tool":
				req.Tool = "upload"
			case "no-schema":
				f.tools[0].Schema = nil
			case "personal":
				req.Arguments["owner_memory"] = "private"
			}
			err := RouteCaseOS(context.Background(), p, req, f)
			if scenario == "ok" {
				if err != nil || f.called != 1 {
					t.Fatalf("route failed: %v", err)
				}
			} else if err == nil || f.called != 0 {
				t.Fatalf("unsafe call: %v calls=%d", err, f.called)
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("connector error leaked")
			}
		})
	}
}

func TestCaseOSRequestCannotChooseCaseParameter(t *testing.T) {
	root := t.TempDir()
	p := CaseOSPolicy{Endpoint: "https://caseos.production.mcp.bcg.com/official-path", Consent: true, Workspace: root, CaseID: "allowed", ReadTools: []string{"query"}, CaseParameters: map[string]string{"query": "case_id"}}
	r := CaseOSRequest{Workspace: root, CaseID: "allowed", Tool: "query", CaseArgument: "query", Arguments: map[string]any{"case_id": "foreign", "query": "allowed"}}
	c := &fakeConnector{tools: []RemoteTool{{Name: "query", ReadOnly: true, Schema: []byte("{\"type\":\"object\",\"properties\":{\"case_id\":{\"type\":\"string\"},\"query\":{\"type\":\"string\"}},\"required\":[\"case_id\",\"query\"]}")}}}
	if RouteCaseOS(context.Background(), p, r, c) == nil || c.called != 0 {
		t.Fatal("request-selected case field bypassed enrollment")
	}
}
func TestCaseOSEndpointAndRedirect(t *testing.T) {
	for _, value := range []string{"http://caseos.production.mcp.bcg.com/path", "https://caseos-dev.bcg.com/mcp", "https://caseos.production.mcp.bcg.com.evil/path", "https://user:secret@caseos.production.mcp.bcg.com/path", "https://caseos.production.mcp.bcg.com:444/path", "https://caseos.production.mcp.bcg.com/path?token=secret", "https://caseos.production.mcp.bcg.com/path#fragment"} {
		if ValidateCaseOSEndpoint(value) == nil {
			t.Fatalf("accepted %s", value)
		}
	}
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://evil.example/"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	})}
	if ProbeCaseOSEndpoint(context.Background(), "https://caseos.production.mcp.bcg.com/official-path", client) == nil || calls != 1 {
		t.Fatalf("redirect followed calls=%d", calls)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
