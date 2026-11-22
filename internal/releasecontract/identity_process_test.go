package releasecontract

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const identityProbeTestMode = "MAESTRO_TEST_RELEASE_IDENTITY_MODE"
const identityProbeTestReady = "MAESTRO_TEST_RELEASE_IDENTITY_READY"

func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == "identity" && os.Getenv(identityProbeTestMode) != "" {
		if os.Getenv(identityProbeTestMode) == "blocked" {
			if err := os.WriteFile(os.Getenv(identityProbeTestReady), []byte("ready"), 0600); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(2)
			}
			time.Sleep(time.Minute)
		}
		identity := ServerIdentity{SchemaVersion: 2, Product: "maestro", Component: "mcp_server", Protocol: ServerProtocol, Version: "0.1.0", CompatibleBundle: ">=0.1.0 <0.2.0", AuthorityRegistrySHA256: strings.Repeat("b", 64), Resources: []EmbeddedResource{{URI: "maestro://test", Bytes: 1, SHA256: strings.Repeat("a", 64)}}, ExecutionContract: SyncedSourceContract}
		var err error
		identity.EmbeddedInputsSHA256, err = EmbeddedDigest(identity.Resources)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		body, err := json.Marshal(identity)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		switch os.Getenv(identityProbeTestMode) {
		case "unknown field":
			body = append(body[:len(body)-1], []byte(`,"unexpected":true}`)...)
		case "duplicate field":
			body = append(body[:len(body)-1], []byte(`,"version":"0.1.0"}`)...)
		case "trailing value":
			body = append(body, body...)
		case "maximum output":
			body = append(body, []byte(strings.Repeat(" ", maximumManifestBytes-len(body)))...)
		case "oversized output":
			body = append(body, []byte(strings.Repeat(" ", maximumManifestBytes))...)
		}
		os.Stdout.Write(body)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestReadServerIdentityContextCancelsRunningProbe(t *testing.T) {
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ready := filepath.Join(t.TempDir(), "ready")
	t.Setenv(identityProbeTestMode, "blocked")
	t.Setenv(identityProbeTestReady, ready)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		_, err := ReadServerIdentityContext(ctx, path)
		done <- err
	}()
	t.Cleanup(func() { cancel(); <-finished })
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("identity subprocess did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	start := time.Now()
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Errorf("running probe ignored caller cancellation: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("caller cancellation waited for the independent probe timeout: %s", elapsed)
	}
}

func TestReadServerIdentityContextRejectsAlreadyDoneCaller(t *testing.T) {
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"cancelled", "expired"} {
		t.Run(kind, func(t *testing.T) {
			ready := filepath.Join(t.TempDir(), "ready")
			t.Setenv(identityProbeTestMode, "blocked")
			t.Setenv(identityProbeTestReady, ready)
			ctx, cancel := context.WithCancel(t.Context())
			want := error(context.Canceled)
			if kind == "expired" {
				cancel()
				ctx, cancel = context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
				want = context.DeadlineExceeded
			} else {
				cancel()
			}
			defer cancel()
			if _, err := ReadServerIdentityContext(ctx, path); !errors.Is(err, want) {
				t.Fatalf("caller completion lost: got %v, want %v", err, want)
			}
			if _, err := os.Stat(ready); !os.IsNotExist(err) {
				t.Fatal("completed caller started an identity subprocess")
			}
		})
	}
}

func TestReadServerIdentityContextPreservesProtocolBounds(t *testing.T) {
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"valid", "unknown field", "duplicate field", "trailing value", "maximum output", "oversized output"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv(identityProbeTestMode, mode)
			identity, err := ReadServerIdentityContext(t.Context(), path)
			if mode == "valid" || mode == "maximum output" {
				if err != nil || identity.Version != "0.1.0" || identity.ExecutionContract != SyncedSourceContract {
					t.Fatalf("valid protocol refused: identity=%+v, error=%v", identity, err)
				}
				if _, err := ReadServerIdentity(path); err != nil {
					t.Fatalf("legacy wrapper lost valid protocol: %v", err)
				}
			} else if err == nil {
				t.Fatalf("accepted %s", mode)
			}
		})
	}
}
