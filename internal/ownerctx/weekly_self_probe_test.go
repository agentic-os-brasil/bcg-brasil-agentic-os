package ownerctx

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWeeklySelfProbeInputIsMemoryOnlyAndDoesNotReplacePending(t *testing.T) {
	root, options, now := weeklyCoreRoot(t)
	input, err := InspectWeeklySelfInput(context.Background(), root, options, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := input.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "owner/self/weekly")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("memory-only probe created weekly state", err)
	}
	pending, err := FreezeWeeklySelf(root, options, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(weeklySelfPath(root))
	options.OccurrenceID = strings.Repeat("f", 64)
	probe, err := InspectWeeklySelfInput(context.Background(), root, options, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if probe.InputSHA256 == pending.InputSHA256 {
		t.Fatal("distinct probe reused pending identity")
	}
	after, _ := os.ReadFile(weeklySelfPath(root))
	if !bytes.Equal(before, after) {
		t.Fatal("memory-only probe replaced normal pending manifest")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := InspectWeeklySelfInput(cancelled, root, options, nil, now); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled probe selected input", err)
	}
	after, _ = os.ReadFile(weeklySelfPath(root))
	if !bytes.Equal(before, after) {
		t.Fatal("cancelled probe changed owner state")
	}
}

func TestWeeklySelfInvocationRechecksSelectedSourceBeforeInput(t *testing.T) {
	root, options, now := weeklyCoreRoot(t)
	input, err := InspectWeeklySelfInput(context.Background(), root, options, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = ValidateWeeklySelfInputCurrent(context.Background(), root, input, now); err != nil {
		t.Fatal(err)
	}
	if err = ValidateWeeklySelfInputCurrent(context.Background(), root, input, input.RecoverUntil); !errors.Is(err, ErrWeeklySelfExpired) {
		t.Fatal("expired snapshot admitted", err)
	}
	if err = os.WriteFile(filepath.Join(root, "owner/self/communication-style.md"), []byte("# Current\n\nOwner correction after frozen input.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if ValidateWeeklySelfInputCurrent(context.Background(), root, input, now) == nil {
		t.Fatal("changed source admitted before model input")
	}
	if _, err = os.Stat(filepath.Join(root, "owner/self/weekly")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("validation created pending state", err)
	}
}
