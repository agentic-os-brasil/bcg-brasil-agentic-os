package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
)

func TestFirstClientOwnerReviewChannelIsAnExplicitCommonChoice(t *testing.T) {
	for _, choice := range []string{"", "native-first", "local-browser", "model-approved"} {
		got, err := promptClientOwnerReviewChannel(func(message string) (string, error) {
			if !strings.Contains(message, "BOTH Code and Desktop") || !strings.Contains(message, "approves no write") {
				t.Fatal("first setup hid the common client scope or individual approval requirement")
			}
			return choice, nil
		}, false)
		if choice == "model-approved" {
			if err == nil {
				t.Fatal("unknown channel accepted")
			}
			continue
		}
		want := ""
		if choice == "local-browser" {
			want = choice
		}
		if err != nil || got != want {
			t.Fatalf("choice %q: channel=%q error=%v", choice, got, err)
		}
	}
}

func TestClientCapabilitiesReadOnlyWithoutInstallation(t *testing.T) {
	oldArgs, oldVersion, oldPin, oldOutput := os.Args, Version, AuthorityRegistrySHA256, os.Stdout
	defer func() { os.Args, Version, AuthorityRegistrySHA256, os.Stdout = oldArgs, oldVersion, oldPin, oldOutput }()
	Version, AuthorityRegistrySHA256 = "0.3.7", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	os.Args = []string{"maestro-install", "client-capabilities"}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	os.Stdout = w
	err = run()
	w.Close()
	os.Stdout = oldOutput
	body, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if err != nil {
		t.Fatalf("read-only capability discovery requires no installation: %v", err)
	}
	var value struct {
		Schema         int    `json:"schema_version"`
		Profile        string `json:"profile"`
		Protocol       string `json:"protocol"`
		Version        string `json:"version"`
		CarrierVersion string `json:"carrier_version"`
	}
	if err = json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	if value.Schema != 1 || value.Profile != "stable-local-v1" || value.Protocol != "maestro-stable-carrier-v1" || value.Version != "0.3.7" || value.CarrierVersion != "1.0.0" {
		t.Fatalf("unexpected actual capability: %s", body)
	}
	os.Args = []string{"maestro-install", "lifecycle-capabilities"}
	r2, w2, e := os.Pipe()
	if e != nil {
		t.Fatal(e)
	}
	defer r2.Close()
	os.Stdout = w2
	e = run()
	w2.Close()
	os.Stdout = oldOutput
	body, e2 := io.ReadAll(r2)
	var lifecycle struct {
		Profile         string   `json:"profile"`
		Events          []string `json:"events"`
		Enforcement     string   `json:"enforcement"`
		NativeExecution string   `json:"native_execution"`
	}
	if e != nil || e2 != nil || json.Unmarshal(body, &lifecycle) != nil || lifecycle.Profile != "mcp_lifecycle" || len(lifecycle.Events) != 3 || lifecycle.Enforcement != "unavailable" || lifecycle.NativeExecution != "not_observed" {
		t.Fatalf("finite lifecycle capability: %s %v %v", body, e, e2)
	}

}

func TestDesktopTargetCapabilityAndFrontendRefusals(t *testing.T) {
	oldArgs, oldVersion, oldPin, oldOutput := os.Args, Version, AuthorityRegistrySHA256, os.Stdout
	defer func() { os.Args, Version, AuthorityRegistrySHA256, os.Stdout = oldArgs, oldVersion, oldPin, oldOutput }()
	Version, AuthorityRegistrySHA256 = "0.3.7", strings.Repeat("b", 64)
	os.Args = []string{"maestro-install", "client-target-capabilities"}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	os.Stdout = w
	err = run()
	w.Close()
	os.Stdout = oldOutput
	body, readErr := io.ReadAll(r)
	var got installer.ClientTargetCapability
	if err != nil || readErr != nil || json.Unmarshal(body, &got) != nil || got != installer.DesktopClientCapabilities("installer", Version, AuthorityRegistrySHA256) {
		t.Fatalf("target-aware actual reader must disclose its closed protocol without installation: %s %v %v", body, err, readErr)
	}
	os.Args = append(os.Args, "--unreviewed")
	if err := run(); err == nil {
		t.Fatal("target capabilities accepted extra arguments")
	}
	for _, command := range []string{"manage", "clients-status"} {
		if handled, err := runClients(context.Background(), command, "", lifecycleFlags{ClientTarget: "both"}, "", false); !handled || err == nil || !strings.Contains(err.Error(), "unsupported client target") {
			t.Fatalf("unknown target must refuse before installation/discovery: %v %v", handled, err)
		}
	}
	if handled, err := runClients(context.Background(), "code-apply", "", lifecycleFlags{ClientTarget: "desktop"}, "", false); !handled || err == nil {
		t.Fatal("Code mutation accepted a caller-selected Desktop target")
	}
	if err := manageClients(context.Background(), installer.ClientOptions{Request: installer.ClientRequest{Target: "desktop", CodeConfigurationDirectory: "hidden-profile"}}, strings.NewReader(""), io.Discard); err == nil || !strings.Contains(err.Error(), "cannot include Code choices") {
		t.Fatalf("Desktop choices must refuse before installed state, prompts or effects: %v", err)
	}
	var prompts []string
	_, err = promptClientOwnerReviewChannel(func(prompt string) (string, error) { prompts = append(prompts, prompt); return "", nil }, true)
	if err != nil || len(prompts) != 1 || strings.Contains(prompts[0], "BOTH Code") || !strings.Contains(prompts[0], "approves no write") {
		t.Fatalf("Desktop review must describe its actual selected scope: %v %v", prompts, err)
	}
}
