//go:build darwin

package packaging

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func weeklyIdentitySyntheticDeclaration() processidentity.WeeklyNativeDeclaration {
	return processidentity.WeeklyNativeDeclaration{Topology: processidentity.WeeklyNativeDarwinStandalone, TeamID: "ABCDEFGHIJ", SigningIdentifier: "test.synthetic.unregistered", CodeDirectoryHash: strings.Repeat("0", 40), Architecture: runtime.GOARCH}
}

func TestWeeklyIdentityDarwinArgumentsAreFixedLiteralValues(t *testing.T) {
	d := weeklyIdentitySyntheticDeclaration()
	requirement, err := weeklyDarwinRequirement(d)
	if err != nil {
		t.Fatal(err)
	}
	want := `anchor apple generic and certificate leaf[subject.OU] = "ABCDEFGHIJ" and identifier "test.synthetic.unregistered" and cdhash H"` + strings.Repeat("0", 40) + `"`
	if requirement != want || strings.Contains(requirement, "\\") {
		t.Fatal("requirement not exact bounded literal", requirement)
	}
	dynamic, err := weeklyDarwinDynamicArguments(d, 123)
	if err != nil || !reflect.DeepEqual(dynamic, []string{"--verify", "--verbose=1", "--test-requirement", "=" + want, "+123"}) {
		t.Fatal("dynamic PID vector changed", dynamic, err)
	}
	path := filepath.Join(t.TempDir(), "synthetic file with spaces")
	static, err := weeklyDarwinStaticArguments(d, path)
	if err != nil || !reflect.DeepEqual(static, []string{"--verify", "--verbose=1", "--test-requirement", "=" + want, "--", path}) {
		t.Fatal("static vector changed", err)
	}
	d.SigningIdentifier = `x" or true`
	if _, err := weeklyDarwinDynamicArguments(d, 123); err == nil {
		t.Fatal("requirement injection accepted")
	}
	d = weeklyIdentitySyntheticDeclaration()
	if _, err := weeklyDarwinDynamicArguments(d, 0); err == nil {
		t.Fatal("reported invalid PID accepted")
	}
	if _, err := weeklyDarwinStaticArguments(d, path+"\n"); err == nil {
		t.Fatal("ambiguous path accepted")
	}
}

func TestWeeklyIdentityDarwinRejectsSyntheticCompiledImage(t *testing.T) {
	// The only inspected executable is this controlled Go test binary, never an
	// installed vendor/profile. A matching hash/header is not vendor identity.
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	identity, err := InspectWeeklyRuntime(ctx, path, hex.EncodeToString(hash.Sum(nil)), weeklyIdentitySyntheticDeclaration())
	if err != nil && !errors.Is(err, errWeeklyIdentityStaticSignature) {
		t.Fatalf("synthetic image did not reach actual static signature denial: %v", err)
	}
	if err == nil || identity != nil {
		if identity != nil {
			_ = identity.Close()
		}
		t.Fatal("compiled synthetic bytes became supported native identity")
	}
}

func TestWeeklyIdentityDarwinUnsafeInputsNeverLaunchInspector(t *testing.T) {
	d := weeklyIdentitySyntheticDeclaration()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "script")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte("#!/bin/sh\nexit 0\n"))
	if identity, err := InspectWeeklyRuntime(ctx, path, hex.EncodeToString(hash[:]), d); err == nil || identity != nil {
		t.Fatal("interpreter shim admitted")
	}
	alias := filepath.Join(filepath.Dir(path), "alias")
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	if identity, err := InspectWeeklyRuntime(ctx, alias, hex.EncodeToString(hash[:]), d); err == nil || identity != nil {
		t.Fatal("symlink image admitted")
	}
	cancelled, stop := context.WithCancel(ctx)
	stop()
	if _, err := InspectWeeklyRuntime(cancelled, path, strings.Repeat("a", 64), d); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled inspection did not preserve cancellation", err)
	}
	if _, err := InspectWeeklyRuntime(context.Background(), path, strings.Repeat("a", 64), d); err == nil {
		t.Fatal("unbounded inspector context accepted")
	}
	var zero WeeklyRuntimeIdentity
	if err := zero.VerifyCurrent(ctx); err == nil {
		t.Fatal("zero metadata became held identity")
	}
	if _, err := zero.ObserveChild(ctx, nil, os.Getpid(), time.Now()); err == nil {
		t.Fatal("missing actual child became observed identity")
	}
	if err := zero.Close(); err != nil {
		t.Fatal(err)
	}
}
