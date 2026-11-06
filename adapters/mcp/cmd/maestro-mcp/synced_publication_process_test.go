package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestMain composes only a test executable. Shipping go-build entrypoints do
// not contain this file, synchronization environment or constructor callback.
// Its identity and server are the real production implementations; no healthy
// executable/capability callback substitutes for actual process/file identity.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "identity" {
		if err := run(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	if len(os.Args) > 1 && os.Args[1] == "serve" {
		if err := servePublicationTestProcess(); err != nil {
			fmt.Fprintln(os.Stderr, "controlled publication process:", err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func servePublicationTestProcess() error {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var options product.Options
	flags.StringVar(&options.ProductRoot, "product-root", "", "")
	flags.StringVar(&options.UserHome, "user-home", "", "")
	flags.StringVar(&options.WorkspacePath, "workspace", "", "")
	flags.StringVar(&options.InstalledManifestSHA256, "installed-manifest", "", "")
	flags.StringVar(&options.InstalledClientInputsSHA256, "installed-client-inputs", "", "")
	if err := flags.Parse(os.Args[2:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected test-process serve arguments")
	}
	seedDoclingAuthority(&options)
	options.GuardProcess = true
	barrier := os.Getenv("MAESTRO_SYNCED_R1_PUBLICATION_BARRIER")
	options.LookupEnv = func(key string) (string, bool) {
		if key == "MAESTRO_HOME" && barrier != "" && publicationGuardInTestStack() {
			if _, err := os.Stat(filepath.Join(barrier, "armed")); err == nil {
				identity, identityErr := processidentity.Current()
				state, stateErr := installtx.ReadSourceState(filepath.Join(options.UserHome, ".maestro-install"), options.ProductRoot)
				err := publishPublicationReady(filepath.Join(barrier, "ready.json"), map[string]any{"running_sha256": identity.SHA256, "compiled_version": Version, "current_server_sha256": state.Server.SHA256, "current_release": state.Release, "identity_error": fmt.Sprint(identityErr), "state_error": fmt.Sprint(stateErr), "manifest_pin": options.InstalledManifestSHA256, "client_inputs_pin": options.InstalledClientInputsSHA256})
				if err != nil && !errors.Is(err, os.ErrExist) {
					return filepath.Join(barrier, "synchronization-unavailable"), true
				}
				if err == nil {
					deadline := time.NewTimer(90 * time.Second)
					defer deadline.Stop()
					ticker := time.NewTicker(5 * time.Millisecond)
					defer ticker.Stop()
					for {
						select {
						case <-deadline.C:
							return filepath.Join(barrier, "synchronization-unavailable"), true
						case <-ticker.C:
							if _, err := os.Stat(filepath.Join(barrier, "continue")); err == nil {
								_ = os.WriteFile(filepath.Join(barrier, "continued"), []byte("controlled constructor synchronization completed\n"), 0600)
								return "", false
							}
						}
					}
				}
			}
		}
		// The fixture deliberately has no MAESTRO_HOME override. No source,
		// clock, identity, approval or installed release is supplied here.
		if key == "MAESTRO_HOME" {
			return "", false
		}
		return os.LookupEnv(key)
	}
	bound, err := product.Open(options)
	if err != nil {
		return err
	}
	return bound.Server().Run(context.Background(), &mcp.StdioTransport{})
}

func publicationGuardInTestStack() bool {
	pcs := make([]uintptr, 32)
	n := runtime.Callers(2, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	for {
		frame, more := frames.Next()
		if strings.Contains(frame.Function, ".(*Bound).guardPublication") {
			return true
		}
		if !more {
			return false
		}
	}
}

// publishPublicationReady is the test-process readiness boundary consumed by
// the parent; an existing readiness record belongs to its original publisher.
func publishPublicationReady(path string, snapshot any) error {
	body, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	return scheduler.PublishNewPrivateFile(path, append(body, '\n'))
}
