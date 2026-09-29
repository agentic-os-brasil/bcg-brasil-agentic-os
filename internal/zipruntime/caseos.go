package zipruntime

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// These owner-local policies are never shipped with case IDs or credentials.
type CaseOSPolicy struct {
	Endpoint, Workspace, CaseID string
	Consent                     bool
	ReadTools                   []string
	// CaseParameters is enrolled alongside each reviewed read tool/schema.
	CaseParameters map[string]string
}
type CaseOSRequest struct {
	Workspace, CaseID, Tool, CaseArgument string
	Arguments                             map[string]any
	Export                                bool
}
type RemoteTool struct {
	Name     string
	ReadOnly bool
	Schema   []byte
}

// Discover must perform a fresh, authenticated host-native MCP health/discovery
// handshake against the exact endpoint, with TLS validation and no redirects.
// Implementations must never read OAuth credentials from repository files.
type CaseOSConnector interface {
	Discover(context.Context, string) ([]RemoteTool, error)
	Call(context.Context, string, string, map[string]any) error
}

func ValidateCaseOSEndpoint(value string) error {
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "https" || u.Host != "caseos.production.mcp.bcg.com" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return errors.New("caseOS requires the official HTTPS endpoint on the approved production host; no credentials, query or fragment")
	}
	return nil
}

// ProbeCaseOSEndpoint verifies a supplied endpoint before configuration. It
// never persists, guesses a path, follows redirects, or supplies credentials.
// A 401 is unavailable, not a successful MCP handshake. Authentication belongs
// to the native host; an attended authenticated host probe can complete setup.
func ProbeCaseOSEndpoint(ctx context.Context, endpoint string, client *http.Client) error {
	if err := ValidateCaseOSEndpoint(endpoint); err != nil {
		return err
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	bounded := *client
	bounded.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	bounded.Timeout = 10 * time.Second
	body := "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{\"protocolVersion\":\"2025-03-26\",\"capabilities\":{},\"clientInfo\":{\"name\":\"maestro-preflight\",\"version\":\"0.2.0\"}}}"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(body))
	if err != nil {
		return errors.New("caseOS request unavailable")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := bounded.Do(req)
	if err != nil {
		return errors.New("caseOS TLS or connection check failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return errors.New("caseOS MCP handshake unavailable; use host authentication and approved endpoint verification")
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 65537))
	if err != nil || len(raw) > 65536 {
		return errors.New("caseOS handshake exceeds bounds")
	}
	var reply struct {
		JSONRPC string
		ID      int
		Result  struct {
			ProtocolVersion string
			ServerInfo      map[string]any
			Capabilities    map[string]any
		}
		Error any
	}
	if json.Unmarshal(raw, &reply) != nil || reply.JSONRPC != "2.0" || reply.ID != 1 || reply.Error != nil || reply.Result.ProtocolVersion == "" || reply.Result.ServerInfo == nil || reply.Result.Capabilities == nil {
		return errors.New("caseOS MCP handshake not verified")
	}
	return nil
}

// RouteCaseOS is a read-only connector boundary. Export is deliberately absent:
// it requires a separately reviewed, content-specific publication contract.
func RouteCaseOS(ctx context.Context, p CaseOSPolicy, r CaseOSRequest, c CaseOSConnector) error {
	deny := errors.New("caseOS unavailable: consent, scope, health or read-only schema boundary not satisfied")
	if ValidateCaseOSEndpoint(p.Endpoint) != nil || !p.Consent || p.Workspace == "" || p.CaseID == "" || r.Export || c == nil || r.CaseID != p.CaseID || !slices.Contains(p.ReadTools, r.Tool) {
		return deny
	}
	a, e := filepath.EvalSymlinks(p.Workspace)
	if e != nil {
		return deny
	}
	b, e := filepath.EvalSymlinks(r.Workspace)
	if e != nil || a != b {
		return deny
	}
	// Enrollment, not the request, binds the tool to its case parameter.
	caseParameter := p.CaseParameters[r.Tool]
	if caseParameter == "" || (r.CaseArgument != "" && r.CaseArgument != caseParameter) || r.Arguments[caseParameter] != p.CaseID {
		return deny
	}
	tools, err := c.Discover(ctx, p.Endpoint)
	if err != nil {
		return deny
	}
	for _, tool := range tools {
		if tool.Name == r.Tool && tool.ReadOnly {
			if len(tool.Schema) == 0 || len(tool.Schema) > 65536 {
				return deny
			}
			var schema any
			if json.Unmarshal(tool.Schema, &schema) != nil || hasReference(schema) {
				return deny
			}
			object, ok := schema.(map[string]any)
			if !ok || object["type"] != "object" {
				return deny
			}
			properties, ok := object["properties"].(map[string]any)
			if !ok {
				return deny
			}
			parameter, ok := properties[caseParameter].(map[string]any)
			if !ok || parameter["type"] != "string" {
				return deny
			}
			required, _ := object["required"].([]any)
			boundRequired := false
			for _, field := range required {
				if field == caseParameter {
					boundRequired = true
				}
			}
			if !boundRequired {
				return deny
			}
			compiler := jsonschema.NewCompiler()
			if compiler.AddResource("urn:maestro:caseos-tool", schema) != nil {
				return deny
			}
			compiled, err := compiler.Compile("urn:maestro:caseos-tool")
			if err != nil {
				return deny
			}
			raw, err := json.Marshal(r.Arguments)
			if err != nil || len(raw) > 16384 {
				return deny
			}
			var args any
			if json.Unmarshal(raw, &args) != nil || compiled.Validate(args) != nil {
				return deny
			}
			if err = c.Call(ctx, p.Endpoint, r.Tool, r.Arguments); err != nil {
				return errors.New("caseOS call failed; no content retained")
			}
			return nil
		}
	}
	return deny
}

// Discovered schemas cannot initiate network resolution or import resources.
func hasReference(v any) bool {
	switch x := v.(type) {
	case map[string]any:
		for k, v := range x {
			if k == "$ref" || k == "$dynamicRef" || hasReference(v) {
				return true
			}
		}
	case []any:
		for _, v := range x {
			if hasReference(v) {
				return true
			}
		}
	}
	return false
}
