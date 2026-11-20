package product

import (
	"context"
	"errors"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
)

// The reader argument is a private process-composition seam, never a flag or
// mutable global. The observation itself grants no authenticated authority:
// retained callbacks must still revalidate the real Current when consumed.
func TestDoclingOwningContextCannotDowngradeBetweenConstructorReads(t *testing.T) {
	for _, phase := range []string{"enrolled", "read-failed"} {
		t.Run(phase, func(t *testing.T) {
			root := t.TempDir()
			b := &Bound{options: Options{ProductRoot: filepath.Join(root, "managed"), UserHome: filepath.Join(root, "owner"), CLIVersion: "0.1.43"}, home: filepath.Join(root, "brain-home")}
			initial := installtx.State{SchemaVersion: 4, Binding: installtx.ReleaseBinding{ManifestSHA256: "observed-initial-Current", ClientInputsSHA256: "observed-client-inputs"}}
			reads := 0
			read := func(string, string) (installtx.State, error) {
				reads++
				if reads == 1 {
					return initial, nil
				}
				if phase == "read-failed" {
					return installtx.State{}, errors.New("current changed during construction")
				}
				enrolled := initial
				enrolled.Source = &installtx.SourceControl{}
				return enrolled, nil
			}
			options := b.doclingOptionsWithReader(read)
			if options.ResolveRequirement == nil || options.GuardSelection == nil {
				t.Fatalf("known initial Current lost enforcement after %s observation (reads=%d)", phase, reads)
			}
			if reads != 1 {
				t.Fatalf("constructor classified more than one owning snapshot: %d", reads)
			}
			if _, err := options.ResolveRequirement(context.Background()); err == nil {
				t.Fatal("reader observation replaced real Current authentication")
			}
			published := false
			if err := options.GuardSelection(func() error { published = true; return nil }); err == nil || published {
				t.Fatal("missing or changed actual Current published through initial guard")
			}
		})
	}
}

func TestDoclingNoOwningCurrentRetainsOriginalVerifier(t *testing.T) {
	for _, name := range []string{"uninstalled", "legacy-schema3"} {
		t.Run(name, func(t *testing.T) {
			b := &Bound{}
			reads := 0
			read := func(string, string) (installtx.State, error) {
				reads++
				if name == "uninstalled" {
					return installtx.State{}, os.ErrNotExist
				}
				return installtx.State{SchemaVersion: 3}, nil
			}
			options := b.doclingOptionsWithReader(read)
			if options.ResolveRequirement != nil || options.GuardSelection != nil {
				t.Fatal("unowned legacy call gained installed Current authority")
			}
		})
	}
}

// Readable initial custody exists only for admitted installation targets.
// Unsupported hosts retain rejected target metadata to exercise refusal, never
// a signature or native witness. Supported hosts keep the original plain wrapper.
func doclingRecoveredInitialFixture(t *testing.T) (*Bound, bool) {
	t.Helper()
	b := localReviewFixture(t, "")
	b.options.CLIVersion = "0.1.43"
	data := filepath.Join(b.options.UserHome, ".maestro-install")
	if err := os.MkdirAll(filepath.Join(data, "config"), 0700); err != nil {
		t.Fatal(err)
	}
	state := installtx.State{SchemaVersion: 4, ManagedRoot: b.options.ProductRoot, Profile: releasecontract.ModernProfile, Release: "0.1.43", Channel: "canary", CLIVersion: "0.1.43", BundleVersion: "0.1.43", TargetOS: runtime.GOOS, TargetArch: runtime.GOARCH, ActivatedAt: time.Now().UTC(), Server: installtx.ExecutableIdentity{ExecutionContract: releasecontract.SyncedSourceContract, Version: "0.1.43", Protocol: releasecontract.ServerProtocol, CompatibleBundle: ">=0.1.0 <0.2.0", CompatibleServer: ">=0.1.0 <0.2.0", EmbeddedInputsSHA256: strings.Repeat("a", 64), ArtifactName: "server-0.1.43", SHA256: strings.Repeat("b", 64), Size: 1}, Binding: installtx.ReleaseBinding{ManifestSHA256: strings.Repeat("c", 64), ClientInputsSHA256: strings.Repeat("d", 64), ManifestJSON: "{}", ManifestSignature: strings.Repeat("e", 88), BundleSHA256: strings.Repeat("f", 64), BundleSize: 1, BundleArchive: filepath.Join(data, "updates", "bundle.tar.gz")}}
	if err := installtx.WriteState(data, state); err != nil {
		t.Fatal(err)
	}
	_, readErr := installtx.ReadSourceState(data, b.options.ProductRoot)
	supported := (state.TargetOS == "darwin" || state.TargetOS == "windows") && (state.TargetArch == "amd64" || state.TargetArch == "arm64")
	if !supported {
		if readErr == nil {
			t.Fatal("unsupported installation target acquired readable initial custody")
		}
		return b, false
	}
	if readErr != nil {
		t.Fatal("readable initial custody precondition", readErr)
	}
	if err := b.validate(); err != nil {
		t.Fatal("ordinary initial bound precondition", err)
	}
	return b, true
}

func TestDoclingClassifierFailureStaysUnavailableAfterInitialRecovery(t *testing.T) {
	b, _ := doclingRecoveredInitialFixture(t)
	observed := errors.New("owning classification temporarily failed")
	options := b.doclingOptionsWithReader(func(string, string) (installtx.State, error) { return installtx.State{}, observed })
	if options.ResolveRequirement == nil || options.GuardSelection == nil {
		t.Fatal("classification error dropped enforcement")
	}
	if _, err := options.ResolveRequirement(context.Background()); !errors.Is(err, observed) {
		t.Errorf("runtime retried classification after error instead of preserving refusal: %v", err)
	}
	published := false
	if err := options.GuardSelection(func() error { published = true; return nil }); !errors.Is(err, observed) || published {
		t.Fatalf("classification error recovered into unqualified publication: called=%t err=%v", published, err)
	}
}

func TestDoclingQualifiedGuardCannotReclassifyRecoveredInitialAsUnqualified(t *testing.T) {
	b, supported := doclingRecoveredInitialFixture(t)
	options := b.doclingOptionsWithReader(func(string, string) (installtx.State, error) {
		return installtx.State{SchemaVersion: 4, Source: &installtx.SourceControl{}}, nil
	})
	if options.ResolveRequirement == nil || options.GuardSelection == nil {
		t.Fatal("qualified observation dropped enforcement")
	}
	published := false
	if err := options.GuardSelection(func() error { published = true; return nil }); err == nil || published {
		t.Fatalf("qualified runtime downgraded to Source=nil publication: called=%t err=%v", published, err)
	}
	ordinary := false
	err := b.guardPublication(func() error { ordinary = true; return nil })
	if !supported {
		if err == nil || ordinary {
			t.Fatalf("unsupported initial target published: called=%t err=%v", ordinary, err)
		}
		return
	}
	if err != nil || !ordinary {
		t.Fatalf("ordinary unqualified publication changed: called=%t err=%v", ordinary, err)
	}
}
