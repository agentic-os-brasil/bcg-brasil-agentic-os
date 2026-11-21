//go:build darwin && arm64

package docling

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Cancel in the window after verification's last observation. Stack-gated
// cancellation follows the existing boundary-test pattern without a runtime seam.
type cancelAfterPackVerification struct {
	context.Context
	cancel    context.CancelFunc
	triggered bool
}

func (c *cancelAfterPackVerification) Err() error {
	err := c.Context.Err()
	if err == nil && !c.triggered {
		var pcs [12]uintptr
		n := runtime.Callers(2, pcs[:])
		frames := runtime.CallersFrames(pcs[:n])
		for {
			f, more := frames.Next()
			if strings.HasSuffix(f.Function, "(*Runtime).verify.func1") {
				c.triggered = true
				c.cancel()
				break
			}
			if !more {
				break
			}
		}
	}
	return err
}

func TestAuthenticatedStatusCancellationAfterVerification(t *testing.T) {
	root := t.TempDir()
	t.Cleanup(func() {
		_ = filepath.WalkDir(root, func(p string, d os.DirEntry, e error) error {
			if e == nil && d.IsDir() {
				_ = os.Chmod(p, 0700)
			}
			return e
		})
	})
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	registry, _ := json.Marshal(map[string]any{"schema_version": 1, "product": "maestro", "authorities": []any{map[string]any{"issuer": "fixture", "key_id": "ephemeral", "algorithm": "ed25519", "public_key": base64.StdEncoding.EncodeToString(public), "status": "active", "valid_from": "2020-01-01T00:00:00Z", "valid_until": "2100-01-01T00:00:00Z"}}})
	registryPath := filepath.Join(root, "authority.json")
	mustWrite(t, registryPath, registry)
	options := Options{DataRoot: filepath.Join(root, "state"), AuthorityRegistryPath: registryPath, AuthorityRegistrySHA256: digest(registry), CLIVersion: "0.1.0"}
	if err = os.Mkdir(options.DataRoot, 0700); err != nil {
		t.Fatal(err)
	}
	release := syntheticRelease(t, root, "0.1.0", "cancel-after-authenticated-verification", private, registryPath, options.AuthorityRegistrySHA256)
	requirement := requirementForTest(release)
	options.ResolveRequirement = func(context.Context) (Requirement, error) { return requirement, nil }
	if err = PlaceVerified(t.Context(), options, release); err != nil {
		t.Fatal(err)
	}
	converter := New(options)
	if got := converter.Status(t.Context()); got.State != "ready" || got.PackSHA256 != requirement.PackSHA256 {
		t.Fatalf("authenticated fixture not ready: %+v", got)
	}
	parent, cancel := context.WithCancel(t.Context())
	defer cancel()
	ctx := &cancelAfterPackVerification{Context: parent, cancel: cancel}
	got := converter.Status(ctx)
	if !ctx.triggered || parent.Err() != context.Canceled {
		t.Fatal("verification boundary did not cancel the actual caller")
	}
	if got.State != "unavailable" || got.Reason != "conversion_cancelled" {
		t.Fatalf("late cancellation escaped final readiness fence: %+v", got)
	}
	if after := converter.Status(t.Context()); after.State != "ready" || after.PackSHA256 != requirement.PackSHA256 {
		t.Fatalf("cancelled read changed authenticated pack: %+v", after)
	}
}
