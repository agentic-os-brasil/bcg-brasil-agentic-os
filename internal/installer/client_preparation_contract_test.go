package installer

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
)

func TestClientPreparationCanceledBeforeEntryHasNoBuilder(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ReviewDesktopPreparationSeeded(ctx, ClientOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation did not precede review admission", err)
	}
	calls := 0
	builder := func(context.Context, installtx.State, ClientBinding, *ClientRecord) (ClientMaterialization, error) {
		calls++
		return ClientMaterialization{}, nil
	}
	if _, err := MaterializeDesktopPreparationSeeded(ctx, ClientOptions{}, strings.Repeat("a", 64), true, builder); !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatal("canceled preparation reached a builder", err, calls)
	}
}

func TestClientPreparationProjectionPreservesBothSelection(t *testing.T) {
	root := t.TempDir()
	request := ClientRequest{VersionedSlots: true, NativeEnforcement: true, GenerationID: strings.Repeat("a", 32), CarrierID: strings.Repeat("b", 32), Workspace: filepath.Join(root, "workspace"), CodeRoot: filepath.Join(root, "code"), DesktopPath: filepath.Join(root, "desktop.mcpb"), CodeExecutable: filepath.Join(root, "claude"), CodeConfigurationDirectory: filepath.Join(root, "private-code-profile"), CodeScope: "user", Reconcile: true}
	options := ClientOptions{SeededOperationOptions: SeededOperationOptions{Self: filepath.Join(root, "maestro-install"), ManagedRoot: filepath.Join(root, "product"), UserHome: root}, Request: request}
	projection, err := DesktopPreparationOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	if options.Request != request || projection.PreparationSelection == nil {
		t.Fatal("preparation mutated the selected client request")
	}
	wanted := request
	wanted.Reconcile = false
	if *projection.PreparationSelection != wanted || !projection.Request.DesktopOnly() || projection.Request.NativeEnforcement || projection.Request.CodeRoot != "" || projection.Request.CodeExecutable != "" || projection.Request.CodeConfigurationDirectory != "" || projection.Request.GenerationID != "" {
		t.Fatal("host projection lost the selected full profile or claimed Code preparation")
	}
	for _, goos := range []string{"darwin", "windows"} {
		_, launcher := clientManagementLauncherForOS(projection, true, goos)
		text := string(launcher)
		for _, selected := range []string{request.CodeRoot, request.CodeExecutable, request.CodeConfigurationDirectory, request.CodeScope, request.DesktopPath} {
			if !strings.Contains(text, selected) {
				t.Fatalf("%s management launcher lost selected binding %q", goos, selected)
			}
		}
		if strings.Contains(text, "--client-target") || strings.Contains(text, "--confirm") {
			t.Fatalf("%s launcher retargeted or approved the selected carrier", goos)
		}
	}
	for _, kind := range []string{"target", "unversioned", "non-native", "lifecycle", "missing Code", "generation"} {
		t.Run(kind, func(t *testing.T) {
			bad := options
			switch kind {
			case "target":
				bad.Request.Target = "desktop"
			case "unversioned":
				bad.Request.VersionedSlots = false
			case "non-native":
				bad.Request.NativeEnforcement = false
			case "lifecycle":
				bad.Request.LifecycleContext = true
			case "missing Code":
				bad.Request.CodeExecutable = ""
			case "generation":
				bad.Request.GenerationID = "old-proof"
			}
			if _, err := DesktopPreparationOptions(bad); err == nil {
				t.Fatal("incompatible selected profile was projected")
			}
		})
	}
}

func TestClientPreparationCapabilityIsFiniteAndSeparate(t *testing.T) {
	version, pin := "0.1.0", strings.Repeat("a", 64)
	capability := ClientPreparationCapabilities("server", version, pin)
	body, err := json.Marshal(capability)
	if err != nil || validateClientPreparationCapability(body, "server", version, pin) != nil {
		t.Fatal("exact preparation capability refused", err)
	}
	for _, kind := range []string{"protocol", "component", "version", "pin", "schema", "unknown field", "size"} {
		t.Run(kind, func(t *testing.T) {
			bad := capability
			switch kind {
			case "protocol":
				bad.Protocol = DesktopClientTargetProtocol
			case "component":
				bad.Component = "installer"
			case "version":
				bad.Version = "0.0.0"
			case "pin":
				bad.RegistrySHA256 = strings.Repeat("b", 64)
			case "schema":
				bad.SchemaVersion = 2
			}
			wire, _ := json.Marshal(bad)
			if kind == "unknown field" {
				wire = append(wire[:len(wire)-1], []byte(`,"qualified":true}`)...)
			}
			if kind == "size" {
				wire = []byte(strings.Repeat(" ", 64<<10+1))
			}
			if validateClientPreparationCapability(wire, "server", version, pin) == nil {
				t.Fatal("foreign or unbounded capability accepted")
			}
		})
	}
	legacy, _ := json.Marshal(StableClientCapabilities("server", version, pin))
	if strings.Contains(string(legacy), ClientPreparationProtocol) {
		t.Fatal("legacy capability JSON changed")
	}
}
