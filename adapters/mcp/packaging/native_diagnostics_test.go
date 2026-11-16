package packaging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
