package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"reflect"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/dev/spikes/maestromcp/distribution"
)

func testDependencies() dependencies {
	return dependencies{
		home: "/synthetic/Developer/Maestro Team Pilot", target: "darwin-arm64", platform: "darwin",
		apply: func(distribution.InstallInput) (distribution.Receipt, error) {
			return distribution.Receipt{Status: "READY", Marketplace: "/synthetic/market place", Extension: "/synthetic/Maestro.mcpb"}, nil
		},
		inspect: func(string) (distribution.Receipt, error) { return distribution.Receipt{Status: "READY"}, nil },
		execute: func(string, ...string) error { return nil },
	}
}

func pinnedTestKey() string { return hex.EncodeToString(make([]byte, ed25519.PublicKeySize)) }

func TestInvalidInputsDoNotApplyOrActivate(t *testing.T) {
	for _, args := range [][]string{{}, {"install"}, {"install", "--feed", "/feed", "extra"}, {"status", "--activate-code"}, {"unknown"}} {
		t.Run(reflect.ValueOf(args).String(), func(t *testing.T) {
			d := testDependencies()
			d.apply = func(distribution.InstallInput) (distribution.Receipt, error) {
				t.Fatal("unexpected apply")
				return distribution.Receipt{}, nil
			}
			d.execute = func(string, ...string) error { t.Fatal("unexpected host activation"); return nil }
			if got := runWith(args, &bytes.Buffer{}, &bytes.Buffer{}, d, pinnedTestKey()); got == 0 {
				t.Fatalf("accepted invalid args: %v", args)
			}
		})
	}
}

func TestUnpinnedInstallerDoesNotApply(t *testing.T) {
	d := testDependencies()
	d.apply = func(distribution.InstallInput) (distribution.Receipt, error) {
		t.Fatal("unexpected apply")
		return distribution.Receipt{}, nil
	}
	if got := runWith([]string{"install", "--feed", "/feed"}, &bytes.Buffer{}, &bytes.Buffer{}, d, ""); got == 0 {
		t.Fatal("accepted absent pinned key")
	}
}

func TestVerificationFailureDoesNotActivate(t *testing.T) {
	d := testDependencies()
	d.apply = func(distribution.InstallInput) (distribution.Receipt, error) {
		return distribution.Receipt{}, errors.New("signature invalid")
	}
	d.execute = func(string, ...string) error { t.Fatal("activated unverified payload"); return nil }
	if got := runWith([]string{"update", "--feed", "/feed", "--activate-code", "--open-desktop"}, &bytes.Buffer{}, &bytes.Buffer{}, d, pinnedTestKey()); got == 0 {
		t.Fatal("accepted unverified release")
	}
}

func TestActivationUsesArgumentVectorsAndRetainsConfirmationBoundary(t *testing.T) {
	d := testDependencies()
	var calls [][]string
	d.execute = func(name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
		return nil
	}
	var out bytes.Buffer
	if got := runWith([]string{"install", "--feed", "/feed", "--activate-code", "--open-desktop"}, &out, &bytes.Buffer{}, d, pinnedTestKey()); got != 0 {
		t.Fatalf("exit %d: %s", got, out.String())
	}
	want := [][]string{{"claude", "plugin", "marketplace", "add", "/synthetic/market place"}, {"claude", "plugin", "install", "maestro-team-pilot@maestro-team-pilot-releases"}, {"open", "-a", "Claude", "/synthetic/Maestro.mcpb"}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls %#v", calls)
	}
	if !bytes.Contains(out.Bytes(), []byte("CODE_INSTALL_COMMANDS_SUCCEEDED")) || !bytes.Contains(out.Bytes(), []byte("DESKTOP_CONFIRMATION_REQUIRED")) {
		t.Fatalf("missing accurate activation status: %s", out.String())
	}
}

func TestActivationFailureIsNotReportedAsSuccess(t *testing.T) {
	d := testDependencies()
	d.execute = func(string, ...string) error { return errors.New("policy denied") }
	var out bytes.Buffer
	if got := runWith([]string{"install", "--feed", "/feed", "--activate-code"}, &out, &bytes.Buffer{}, d, pinnedTestKey()); got == 0 {
		t.Fatal("host failure should fail command")
	}
	if bytes.Contains(out.Bytes(), []byte("CODE_INSTALL_COMMANDS_SUCCEEDED")) || !bytes.Contains(out.Bytes(), []byte("CODE_ACTIVATION_FAILED")) {
		t.Fatal(out.String())
	}
}

func TestStatusDoesNotNeedPublisherKey(t *testing.T) {
	if got := runWith([]string{"status"}, &bytes.Buffer{}, &bytes.Buffer{}, testDependencies(), ""); got != 0 {
		t.Fatalf("status exit %d", got)
	}
}

func TestWindowsOpenerDoesNotUseCommandInterpreter(t *testing.T) {
	d := testDependencies()
	d.platform = "windows"
	d.apply = func(distribution.InstallInput) (distribution.Receipt, error) {
		return distribution.Receipt{Extension: `C:\Users\Test\Developer\Maestro & pilot\Maestro.mcpb`}, nil
	}
	var calls [][]string
	d.execute = func(name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
		return nil
	}
	if code := runWith([]string{"install", "--feed", "/feed", "--open-desktop"}, &bytes.Buffer{}, &bytes.Buffer{}, d, pinnedTestKey()); code != 0 {
		t.Fatalf("exit %d", code)
	}
	want := [][]string{{"explorer.exe", `C:\Users\Test\Developer\Maestro & pilot\Maestro.mcpb`}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls %#v", calls)
	}
}

func TestUpdateUsesExistingMarketplaceWithoutRegisteringItAgain(t *testing.T) {
	d := testDependencies()
	var calls [][]string
	d.execute = func(name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
		return nil
	}
	if code := runWith([]string{"update", "--feed", "/feed", "--activate-code"}, &bytes.Buffer{}, &bytes.Buffer{}, d, pinnedTestKey()); code != 0 {
		t.Fatalf("exit %d", code)
	}
	want := [][]string{{"claude", "plugin", "update", "maestro-team-pilot@maestro-team-pilot-releases"}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls %#v", calls)
	}
}
