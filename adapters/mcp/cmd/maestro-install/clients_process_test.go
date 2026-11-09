package main

// The positive process witness is compiled only with go test -c. It composes
// the exact production owners with the existing trusted native-verifier seam,
// bound to actual independently built seed bytes. Shipping builds contain no
// fixture flag, environment override or native-admission bypass.
import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/setup"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
)

type clientProcessFixture struct{ Self, Version, Pin, SeedSHA, ManagedRoot, UserHome, Workspace string }

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "test-client-hold" {
		fmt.Println("ready")
		_, _ = io.Copy(io.Discard, os.Stdin)
		os.Exit(0)
	}
	if len(os.Args) > 1 && (os.Args[1] == "test-client-launch" || os.Args[1] == "test-client-manage" || os.Args[1] == "test-client-interrupt") {
		body, err := os.ReadFile(os.Getenv("MAESTRO_CLIENT_PROCESS_FIXTURE"))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		var f clientProcessFixture
		if err = json.Unmarshal(body, &f); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		o := installer.ClientOptions{SeededOperationOptions: installer.SeededOperationOptions{Self: f.Self, Version: f.Version, AuthorityRegistrySHA256: f.Pin, ManagedRoot: f.ManagedRoot, UserHome: f.UserHome, DataRoot: filepath.Join(f.UserHome, ".maestro-install"), VerifyNative: func(_ context.Context, path string) error {
			body, e := os.ReadFile(path)
			if e != nil {
				return e
			}
			if fmt.Sprintf("%x", sha256.Sum256(body)) != f.SeedSHA {
				return errors.New("engineering native seed bytes changed")
			}
			return nil
		}}, Request: installer.ClientRequest{Workspace: f.Workspace}, LookupEnv: func(string) (string, bool) { return "", false }}
		if os.Args[1] == "test-client-manage" {
			if len(os.Args) > 2 {
				o.Request.CarrierID = os.Args[2]
			}
			err = manageClients(context.Background(), o, os.Stdin, os.Stdout)
		} else if os.Args[1] == "test-client-interrupt" {
			if len(os.Args) < 3 || len(os.Args) > 4 {
				os.Exit(2)
			}
			o.Request.CarrierID = os.Args[2]
			status, e := installer.ReadClientsSeeded(context.Background(), o)
			if status.Record == nil {
				fmt.Fprintln(os.Stderr, e)
				os.Exit(2)
			}
			r := *status.Record
			o.Request = installer.ClientRequest{CarrierID: r.CarrierID, Workspace: r.Binding.Workspace, CodeRoot: r.Materialization.CodeRoot, DesktopPath: r.Materialization.DesktopPath, CodeExecutable: r.Code.Executable, CodeScope: r.Code.Scope}
			o.Request.ManualDesktopUpdate = len(os.Args) == 4 && os.Args[3] == "manual-desktop-update"
			review, e := installer.ReviewClientsSeeded(context.Background(), o)
			if e != nil {
				fmt.Fprintln(os.Stderr, e)
				os.Exit(2)
			}
			base := setup.CurrentClientMaterializer(o)
			_, err = installer.MaterializeClientsSeeded(context.Background(), o, review.Digest, true, func(ctx context.Context, s installtx.State, b installer.ClientBinding, previous *installer.ClientRecord) (installer.ClientMaterialization, error) {
				m, e := base(ctx, s, b, previous)
				if e != nil {
					return m, e
				}
				proof, _ := json.Marshal(map[string]any{"phase": "actual_packets_swapped_before_current_record_publication", "materialization": m, "pid": os.Getpid()})
				if e = os.WriteFile(os.Getenv("MAESTRO_CLIENT_PROCESS_FIXTURE")+".interrupted", proof, 0600); e != nil {
					return m, e
				}
				os.Exit(73)
				return m, nil
			})
		} else {
			if len(os.Args) != 3 {
				os.Exit(2)
			}
			o.Request.CarrierID = os.Args[2]
			var launch installer.QualifiedLaunch
			launch, err = installer.PrepareStableLaunch(context.Background(), o)
			if err == nil {
				cmd := exec.Command(launch.Executable, launch.Args...)
				cmd.Stdin = os.Stdin
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				err = cmd.Run()
			}
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	if os.Getenv("MAESTRO_CODE_PROCESS_STATE") != "" && len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "plugin") {
		if err := codeProcessWitness(os.Args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type codeProcessState struct {
	Root, Marketplace, PluginID, Scope, Version string
	Market, Installed, Enabled                  bool
}

func codeProcessWitness(args []string) error {
	if args[0] == "--version" {
		fmt.Println("2.1.295 (controlled engineering Code process witness)")
		return nil
	}
	if args[len(args)-1] == "--help" {
		fmt.Println("supported command --json --scope")
		return nil
	}
	path := os.Getenv("MAESTRO_CODE_PROCESS_STATE")
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var s codeProcessState
	if err = json.Unmarshal(body, &s); err != nil {
		return err
	}
	if len(args) == 4 && args[1] == "marketplace" && args[2] == "list" {
		rows := []any{}
		if s.Market {
			rows = append(rows, map[string]any{"name": s.Marketplace, "source": "directory", "path": s.Root})
		}
		return json.NewEncoder(os.Stdout).Encode(rows)
	}
	if len(args) == 3 && args[1] == "list" {
		rows := []any{}
		if s.Installed {
			rows = append(rows, map[string]any{"id": s.PluginID, "version": s.Version, "scope": s.Scope, "enabled": s.Enabled, "installPath": filepath.Join(s.Root, "plugins", "maestro-product"), "folderVersion": s.Version, "readFromFolder": true})
		}
		return json.NewEncoder(os.Stdout).Encode(rows)
	}
	log, err := os.OpenFile(path+".calls", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if err = json.NewEncoder(log).Encode(args); err != nil {
		log.Close()
		return err
	}
	if err = log.Close(); err != nil {
		return err
	}
	var manifest struct {
		Version string `json:"version"`
	}
	readVersion := func() error {
		b, e := os.ReadFile(filepath.Join(s.Root, "plugins", "maestro-product", ".claude-plugin", "plugin.json"))
		if e != nil {
			return e
		}
		if e = json.Unmarshal(b, &manifest); e != nil {
			return e
		}
		s.Version = manifest.Version
		return nil
	}
	switch args[1] {
	case "marketplace":
		if len(args) < 4 {
			return errors.New("marketplace arguments")
		}
		if args[2] == "add" {
			s.Market = true
			s.Root = args[3]
			b, e := os.ReadFile(filepath.Join(s.Root, ".claude-plugin", "marketplace.json"))
			if e != nil {
				return e
			}
			var mk struct{ Name string }
			if e = json.Unmarshal(b, &mk); e != nil {
				return e
			}
			s.Marketplace = mk.Name
		} else if args[2] != "update" {
			return errors.New("unsupported marketplace mutation")
		}
	case "install":
		s.Installed = true
		s.Enabled = true
		s.PluginID = args[2]
		if err = readVersion(); err != nil {
			return err
		}
	case "update":
		if err = readVersion(); err != nil {
			return err
		}
		s.Enabled = true
	case "disable":
		s.Enabled = false
	default:
		return errors.New("unsupported process witness command")
	}
	body, err = json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(path, body, 0600); err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]string{"command": "engineering-process-witness", "outcome": "success"})
}
