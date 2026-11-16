package packaging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestNativeFailedSessionRetainsKnownEstimate(t *testing.T) {
	for _, failure := range []string{"startup", "runner", "known_zero", "unknown_second", "over_budget"} {
		t.Run(failure, func(t *testing.T) {
			in := nativeFixture(t)
			calls := 0
			run := inspectionRunner(t, true, func(_ context.Context, args []string, cwd string) ([]byte, error) {
				calls++
				p := probeFromArguments(t, in, args, cwd)
				stream := syntheticNativeStream(t, p)
				switch failure {
				case "runner":
					return stream, errors.New("PRIVATE_RUNNER_FAILURE")
				case "unknown_second":
					if calls == 1 {
						return stream, nil
					}
					return bytes.ReplaceAll(stream, []byte(`,"total_cost_usd":0.01`), nil), nil
				case "over_budget":
					return bytes.ReplaceAll(stream, []byte(`"total_cost_usd":0.01`), []byte(`"total_cost_usd":0.3`)), nil
				case "known_zero":
					stream = bytes.ReplaceAll(stream, []byte(`"total_cost_usd":0.01`), []byte(`"total_cost_usd":0`))
				}
				return bytes.ReplaceAll(stream, []byte("maestro_native_startup_unchanged:"), []byte("PRIVATE_STARTUP_MISMATCH:")), nil
			})
			review, err := reviewNative(context.Background(), in, run)
			if err != nil {
				t.Fatal(err)
			}
			result, err := qualifyAndBuildNative(context.Background(), in, review.ReviewDigest, true, run, "engineering_fixture")
			if err != nil {
				t.Fatal(err)
			}
			wantCost, wantCalls, complete := 0.01, 1, true
			wantReason := "native_challenge_evidence_incomplete"
			switch failure {
			case "known_zero":
				wantCost = 0
			case "unknown_second":
				wantCalls, complete = 2, false
			case "over_budget":
				wantCost, wantReason = 0.3, "native_estimate_exceeded_reviewed_per_call_limit"
			}
			if result.EstimatedSpendUSD != wantCost || result.EstimatedSpendSessions != 1 || result.EstimatedSpendComplete != complete {
				t.Fatalf("failed session lost estimate availability: %+v", result)
			}
			if calls != wantCalls || result.CallsAttempted != wantCalls || result.State != "unavailable" || result.EvidenceClass != "not_observed" || result.Reason != wantReason {
				t.Fatalf("known cost changed failure/stop authority: %+v", result)
			}
		})
	}
}

func TestNativeStartupFailureDiagnostics(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native owner admission unavailable")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	marker := func(value string) string {
		body, err := json.Marshal(map[string]string{"systemMessage": value})
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	zero, failed := 0, 3
	for _, tc := range []struct {
		name                    string
		starts                  int
		response                bool
		uncorrelated            bool
		outcome, stdout, marker string
		exit                    *int
		matched                 bool
	}{
		{name: "absent"},
		{name: "started_without_response", starts: 1},
		{name: "uncorrelated_response", starts: 1, response: true, uncorrelated: true, outcome: "success", stdout: "{}"},
		{name: "failed", starts: 1, response: true, outcome: "error", stdout: "PRIVATE_STDOUT /private/credential", marker: "malformed", exit: &failed},
		{name: "failed_matching_marker", starts: 1, response: true, outcome: "error", stdout: marker("maestro_native_startup_unchanged:PRIVATE_NONCE"), marker: "startup", exit: &failed, matched: true},
		{name: "cancelled", starts: 1, response: true, outcome: "cancelled", stdout: "{}", marker: "missing"},
		{name: "malformed", starts: 1, response: true, outcome: "success", stdout: "PRIVATE_BAD_JSON", marker: "malformed", exit: &zero},
		{name: "missing_marker", starts: 1, response: true, outcome: "success", stdout: "{}", marker: "missing", exit: &zero},
		{name: "mismatch", starts: 1, response: true, outcome: "success", stdout: marker("maestro_native_startup_unchanged:PRIVATE_WRONG_NONCE"), marker: "startup", exit: &zero},
		{name: "matched", starts: 1, response: true, outcome: "success", stdout: marker("maestro_native_startup_unchanged:PRIVATE_NONCE"), marker: "startup", exit: &zero, matched: true},
		{name: "other_outcome", starts: 1, response: true, outcome: "PRIVATE_OUTCOME", stdout: "{}", marker: "missing"},
		{name: "capped", starts: 40, response: true, outcome: "success", stdout: marker("PRIVATE_WRONG_NONCE"), marker: "other", exit: &zero},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(root, tc.name)
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			p := nativeProbe{root: dir, session: "PRIVATE_SESSION", nonce: "PRIVATE_NONCE"}
			var stream bytes.Buffer
			for i := 0; i < tc.starts; i++ {
				for _, subtype := range []string{"hook_started", "hook_response"} {
					if subtype == "hook_response" && !tc.response {
						continue
					}
					event := map[string]any{"type": "system", "subtype": subtype, "session_id": p.session, "uuid": fmt.Sprintf("private-%d-%s", i, subtype), "hook_id": fmt.Sprint(i), "hook_name": "PRIVATE_HOOK /private/credential", "hook_event": "SessionStart"}
					if subtype == "hook_response" {
						event["outcome"], event["stdout"], event["exit_code"] = tc.outcome, tc.stdout, tc.exit
						if tc.uncorrelated {
							event["hook_id"] = "PRIVATE_UNCORRELATED"
						}
					}
					if err := json.NewEncoder(&stream).Encode(event); err != nil {
						t.Fatal(err)
					}
				}
			}
			var diagnostic nativeFailureDiagnostic
			if _, err := verifyNativeProbeDetailed(stream.Bytes(), p, false, &diagnostic); err == nil {
				t.Fatal("startup metadata alone qualified native enforcement")
			}
			if err := writeNativeFailure(p, diagnostic, 0, nil, nil); err != nil {
				t.Fatal(err)
			}
			body, err := os.ReadFile(filepath.Join(dir, "failure.json"))
			if err != nil {
				t.Fatal(err)
			}
			if len(body) > 4096 || bytes.Contains(body, []byte("PRIVATE_")) || bytes.Contains(body, []byte("/private/")) || bytes.Contains(body, []byte(root)) {
				t.Fatalf("startup diagnostic retained payload or exceeded bounds: %s", body)
			}
			var got struct {
				Startup struct {
					Starts       int  `json:"starts"`
					Responses    int  `json:"responses"`
					CountsCapped bool `json:"capped"`
					Observations []struct {
						Outcome      string `json:"outcome"`
						Marker       string `json:"marker"`
						NonceMatched bool   `json:"nonce_matched"`
						ExitCode     *int   `json:"exit_code"`
					} `json:"observations"`
				} `json:"startup"`
				StartupObserved    bool   `json:"startup_observed"`
				CanonicalFlowCheck string `json:"canonical_flow_check"`
			}
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatal(err)
			}
			starts, responses, observations := tc.starts, 0, 0
			if tc.response && !tc.uncorrelated {
				responses, observations = starts, starts
			}
			capped := starts > 32 || observations > 8
			if starts > 32 {
				starts = 32
			}
			if responses > 32 {
				responses = 32
			}
			if observations > 8 {
				observations = 8
			}
			if got.Startup.Starts != starts || got.Startup.Responses != responses || len(got.Startup.Observations) != observations || got.Startup.CountsCapped != capped || got.StartupObserved != (tc.matched && tc.outcome == "success") || got.CanonicalFlowCheck != "not_reached" {
				t.Fatalf("startup absence/correlation/check status lost: %s", body)
			}
			for _, observed := range got.Startup.Observations {
				outcome := tc.outcome
				if outcome == "PRIVATE_OUTCOME" {
					outcome = "other"
				}
				if observed.Outcome != outcome || observed.Marker != tc.marker || observed.NonceMatched != tc.matched || (observed.ExitCode == nil) != (tc.exit == nil) || (tc.exit != nil && *observed.ExitCode != *tc.exit) {
					t.Fatalf("startup classification differs: %s", body)
				}
			}
		})
	}
}

// These streams contain only deliberately incomplete synthetic controls. The
// real verifier and failure writer remain the seam; no client or model runs.
func allowedReadDiagnosticStream(t *testing.T, p nativeProbe, variants []string, startupCount int) []byte {
	t.Helper()
	var stream bytes.Buffer
	emit := func(value map[string]any) {
		t.Helper()
		if err := json.NewEncoder(&stream).Encode(value); err != nil {
			t.Fatal(err)
		}
	}
	hook := func(id, event, outcome, stdout string) {
		exit := int64(0)
		if event == "SessionStart" {
			exit = -9223372036854775807
		}
		emit(map[string]any{"type": "system", "subtype": "hook_started", "session_id": p.session, "uuid": id + "-start", "hook_id": id, "hook_name": "PRIVATE_HOOK", "hook_event": event})
		emit(map[string]any{"type": "system", "subtype": "hook_response", "session_id": p.session, "uuid": id + "-response", "hook_id": id, "hook_name": "PRIVATE_HOOK", "hook_event": event, "outcome": outcome, "stdout": stdout, "exit_code": exit})
	}
	for i := 0; i < startupCount; i++ {
		hook(fmt.Sprintf("startup-%d", i), "SessionStart", "cancelled", "PRIVATE_MALFORMED_STARTUP")
	}
	for i, variant := range variants {
		id := fmt.Sprintf("PRIVATE_TOOL_%d", i)
		path := filepath.Join(p.challenge, "allowed.txt")
		if variant == "outside" {
			path = p.outside
		}
		emit(map[string]any{"type": "assistant", "message": map[string]any{"content": []any{map[string]any{"type": "tool_use", "id": id, "name": "Read", "input": map[string]any{"file_path": path}}}}})
		if variant != "no_pre" && variant != "attempt_only" {
			decision := ""
			if variant == "denied" {
				decision = "deny"
			}
			output, _ := json.Marshal(map[string]any{"systemMessage": "maestro_native_canonical_hook:" + p.nonce, "hookSpecificOutput": map[string]string{"permissionDecision": decision}})
			hook(id+"-pre", "PreToolUse", "success", string(output))
		}
		if variant != "no_post" && variant != "attempt_only" {
			hook(id+"-post", "PostToolUse", "success", "PRIVATE_POST_BODY")
		}
		if variant != "no_result" && variant != "attempt_only" {
			content := "Synthetic native qualification control. PRIVATE_CONTENT"
			if variant == "no_content" {
				content = "PRIVATE_CONTENT /private/credential"
			}
			emit(map[string]any{"type": "user", "message": map[string]any{"content": []any{map[string]any{"type": "tool_result", "tool_use_id": id, "content": content, "is_error": variant == "error_result"}}}})
		}
	}
	return stream.Bytes()
}

func TestNativeAllowedReadDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name      string
		variants  []string
		observed  bool
		want      []string
		truncated bool
	}{
		{name: "complete", variants: []string{"complete"}, observed: true, want: []string{"canonical_pre post result synthetic_content"}},
		{name: "no_attempt"},
		{name: "no_pre", variants: []string{"no_pre"}, want: []string{"post result synthetic_content"}},
		{name: "no_post", variants: []string{"no_post"}, want: []string{"canonical_pre result synthetic_content"}},
		{name: "no_result", variants: []string{"no_result"}, want: []string{"canonical_pre post"}},
		{name: "no_content", variants: []string{"no_content"}, want: []string{"canonical_pre post result"}},
		{name: "denied", variants: []string{"denied"}, want: []string{"canonical_pre post result synthetic_content denied"}},
		{name: "error_result", variants: []string{"error_result"}, want: []string{"canonical_pre post result error_result"}},
		{name: "outside_not_recorded", variants: []string{"outside"}},
		{name: "cross_call_mix", variants: []string{"no_post", "no_pre"}, want: []string{"canonical_pre result synthetic_content", "post result synthetic_content"}},
		{name: "overflow_keeps_acceptance", variants: []string{"no_post", "no_post", "no_post", "no_post", "no_post", "no_post", "no_post", "no_post", "no_post", "complete"}, observed: true, truncated: true, want: []string{"canonical_pre result synthetic_content", "canonical_pre result synthetic_content", "canonical_pre result synthetic_content", "canonical_pre result synthetic_content", "canonical_pre result synthetic_content", "canonical_pre result synthetic_content", "canonical_pre result synthetic_content", "canonical_pre result synthetic_content"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			p := nativeProbe{root: root, challenge: filepath.Join(root, "PRIVATE_CHALLENGE"), outside: filepath.Join(root, "PRIVATE_OUTSIDE"), session: "PRIVATE_SESSION", nonce: "PRIVATE_NONCE"}
			var d nativeFailureDiagnostic
			if _, err := verifyNativeProbeDetailed(allowedReadDiagnosticStream(t, p, tc.variants, 0), p, true, &d); err == nil {
				t.Fatal("Read diagnostics alone qualified native execution")
			}
			if err := writeNativeFailure(p, d, 0, nil, nil); err != nil {
				t.Fatal(err)
			}
			body, err := os.ReadFile(filepath.Join(root, "failure.json"))
			if err != nil {
				t.Fatal(err)
			}
			if len(body) > 4096 || bytes.Contains(body, []byte("PRIVATE_")) || bytes.Contains(body, []byte("/private/")) || bytes.Contains(body, []byte(root)) {
				t.Fatalf("diagnostic escaped its private-body-free bound: %s", body)
			}
			var got struct {
				Read struct {
					Attempts     int               `json:"attempts"`
					Observations []map[string]bool `json:"observations"`
					Truncated    bool              `json:"truncated"`
				} `json:"read"`
				ReadObserved       bool   `json:"read_observed"`
				CanonicalFlowCheck string `json:"canonical_flow_check"`
			}
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatal(err)
			}
			attempts := len(tc.variants)
			if tc.name == "outside_not_recorded" {
				attempts = 0
			}
			if got.Read.Attempts != attempts || got.Read.Truncated != tc.truncated || len(got.Read.Observations) != len(tc.want) || got.ReadObserved != tc.observed || got.CanonicalFlowCheck != "not_reached" {
				t.Fatalf("read attempts/components or authority changed: %s", body)
			}
			for i, present := range tc.want {
				want := map[string]bool{"canonical_pre": false, "post": false, "result": false, "synthetic_content": false, "denied": false, "error_result": false}
				for _, key := range strings.Fields(present) {
					want[key] = true
				}
				if !reflect.DeepEqual(got.Read.Observations[i], want) {
					t.Fatalf("per-attempt flags differ: got %v want %v", got.Read.Observations[i], want)
				}
			}
		})
	}
}

func TestNativeReadAndStartupDiagnosticsShareExistingBound(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p := nativeProbe{root: root, challenge: filepath.Join(root, "PRIVATE_CHALLENGE"), session: "PRIVATE_SESSION", nonce: "PRIVATE_NONCE"}
	var d nativeFailureDiagnostic
	if _, err := verifyNativeProbeDetailed(allowedReadDiagnosticStream(t, p, []string{"attempt_only", "attempt_only", "attempt_only", "attempt_only", "attempt_only", "attempt_only", "attempt_only", "attempt_only", "attempt_only"}, 40), p, false, &d); err == nil {
		t.Fatal("diagnostics qualified native execution")
	}
	if err := writeNativeFailure(p, d, 0, nil, nil); err != nil {
		t.Fatal("combined maximum closed diagnostics exceed existing writer bound", err)
	}
	body, err := os.ReadFile(filepath.Join(root, "failure.json"))
	if err != nil || len(body) > 4096 || bytes.Contains(body, []byte("PRIVATE_")) {
		t.Fatal("combined diagnostic size/privacy bound", err)
	}
	var got nativeFailureDiagnostic
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Read.Attempts != 9 || len(got.Read.Observations) != 8 || !got.Read.Truncated || len(got.Startup.Observations) != 8 || !got.Startup.Capped {
		t.Fatal("combined diagnostic lost capped independent observations")
	}
	t.Logf("closed_failure_bytes=%d startup_observations=%d read_observations=%d", len(body), len(got.Startup.Observations), len(got.Read.Observations))
}
