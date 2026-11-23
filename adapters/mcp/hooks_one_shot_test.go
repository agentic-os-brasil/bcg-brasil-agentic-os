package product

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func oneShotHookInput(o Options) string {
	return fmt.Sprintf(`{"session_id":"one-shot","prompt_id":"turn-1","cwd":%q,"hook_event_name":"PreToolUse","tool_name":"Read","tool_use_id":"read-1","tool_input":{"file_path":%q}}`, o.WorkspacePath, filepath.Join(o.WorkspacePath, "synthetic.md"))
}

type hookInputObserver struct {
	io.Reader
	reads int
}

func (r *hookInputObserver) Read(p []byte) (int, error) {
	r.reads++
	return r.Reader.Read(p)
}

func TestClaudeHookOneShotRetainsOwnerReviewOptionChecks(t *testing.T) {
	for _, channel := range []string{"native-first", "model-approved", "LOCAL-BROWSER", "local-browser\n"} {
		t.Run(channel, func(t *testing.T) {
			lookups := 0
			o := Options{OwnerReviewChannel: channel, LookupEnv: func(string) (string, bool) { lookups++; return "", false }}
			_, openErr := Open(o)
			input := &hookInputObserver{Reader: strings.NewReader("PRIVATE_PAYLOAD")}
			var output bytes.Buffer
			err := HandleClaudeHook(o, "PreToolUse", input, &output)
			if openErr == nil || err == nil || openErr.Error() != "owner_review_channel_unavailable" || err.Error() != openErr.Error() || input.reads != 0 || output.Len() != 0 || lookups != 0 {
				t.Fatalf("invalid review option escaped constructor checks: open=%v hook=%v reads=%d output=%q lookups=%d", openErr, err, input.reads, output.String(), lookups)
			}
		})
	}
	for _, channel := range []string{"", "local-browser"} {
		t.Run("accepted/"+channel, func(t *testing.T) {
			b := localReviewFixture(t, channel)
			var output bytes.Buffer
			if err := HandleClaudeHook(b.options, "PreToolUse", strings.NewReader(oneShotHookInput(b.options)), &output); err != nil || strings.TrimSpace(output.String()) != "{}" {
				t.Fatalf("supported review selection refused: %v %q", err, output.String())
			}
		})
	}
}

func TestClaudeHookOneShotRevalidatesInstalledBindingPerCall(t *testing.T) {
	f := newProductSeedFixture(t)
	for call := 1; call <= 2; call++ {
		var output bytes.Buffer
		if err := HandleClaudeHook(f.options, "PreToolUse", strings.NewReader(oneShotHookInput(f.options)), &output); err != nil || strings.TrimSpace(output.String()) != "{}" {
			t.Fatalf("ordinary pre-tool hook refused: %v %q", err, output.String())
		}
		identities, err := os.ReadFile(f.identityCounter)
		if err != nil || strings.Count(string(identities), "identity\n") != call {
			t.Fatalf("one-shot admission was repeated or reused across calls: call=%d identities=%q err=%v", call, identities, err)
		}
	}
	// An earlier admitted invocation cannot authorize this changed origin.
	origin := filepath.Join(f.options.ProductRoot, "trust", "experimental-installed-origin.json")
	body, err := os.ReadFile(origin)
	if err != nil || len(body) == 0 {
		t.Fatal("missing synthetic installed origin", err)
	}
	body[0] ^= 1
	if err := os.WriteFile(origin, body, 0600); err != nil {
		t.Fatal(err)
	}
	input := &hookInputObserver{Reader: strings.NewReader("PRIVATE_PAYLOAD")}
	var output bytes.Buffer
	if err := HandleClaudeHook(f.options, "PreToolUse", input, &output); err == nil || err.Error() != "hook_binding_unavailable" || input.reads != 0 || output.Len() != 0 {
		t.Fatalf("changed installed origin reached event input/effects: err=%v reads=%d output=%q", err, input.reads, output.String())
	}
}

func TestClaudeHookOneShotRetainsPostQualifiedExperimentalFence(t *testing.T) {
	f := newProductSeedFixture(t)
	expired := false
	f.onClock = func() {
		identities, err := os.ReadFile(f.identityCounter)
		if err != nil || strings.Count(string(identities), "identity\n") == 0 {
			return
		}
		f.onClock = nil
		f.now = f.expires
		expired = true
	}
	input := &hookInputObserver{Reader: strings.NewReader("PRIVATE_PAYLOAD")}
	var output bytes.Buffer
	err := HandleClaudeHook(f.options, "PreToolUse", input, &output)
	if !expired || err == nil || err.Error() != "hook_binding_unavailable" || input.reads != 0 || output.Len() != 0 {
		t.Fatalf("expiry after actual qualified work escaped entry fence: expired=%t err=%v reads=%d output=%q", expired, err, input.reads, output.String())
	}
}
