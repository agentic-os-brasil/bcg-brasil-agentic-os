package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
)

func validConfig(t *testing.T) config {
	t.Helper()
	policy, err := memory.LoadFile(filepath.Join("..", "..", "..", "bundles", "base", "memory", "policy.json"))
	if err != nil {
		t.Fatal(err)
	}
	return config{Policy: policy, Budgets: map[string]int{"L1": 1024, "L2": 1024, "L3": 1024, "lifetime": 1024},
		Clock: time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)}
}

func TestConfigurationPreservesSourceAndRejectsUnknownTrailingAndUnboundedInput(t *testing.T) {
	cfg := validConfig(t)
	body, _ := json.Marshal(cfg)
	actual, err := readConfig(bytes.NewReader(body))
	if err != nil || !reflect.DeepEqual(actual, cfg) {
		t.Fatalf("configuration changed: %v", err)
	}
	for _, invalid := range [][]byte{
		bytes.Replace(body, []byte(`"clock":`), []byte(`"unknown":`), 1),
		append(append([]byte{}, body...), []byte(` {}`)...),
		append(bytes.Repeat([]byte(" "), (1<<20)+1), body...),
		bytes.Replace(body, []byte(`"schema_version":1`), []byte(`"schema_version":1,"unknown":true`), 1),
		bytes.Replace(body, []byte(`"L1":1024`), []byte(`"L1":0`), 1),
		bytes.Replace(body, []byte(`"freshness":null`), []byte(`"freshness":{"L1":-1}`), 1),
	} {
		if _, err := readConfig(bytes.NewReader(invalid)); err == nil {
			t.Fatal("invalid historical configuration accepted")
		}
	}
}

func TestClassificationNeverRelabelsUnavailableOrGenericFailureAsCohort(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{nil, "success"},
		{memory.ErrConfinementUnavailable, "confinement_unavailable"},
		{errors.New(memory.ErrConfinementUnavailable.Error()), "unexpected_error"},
		{errors.New("invalid memory visibility checkpoint requirement"), "visibility_requirement_refused"},
		{errors.New("invalid input"), "unexpected_error"},
	}
	for _, example := range cases {
		call := classify("actual API", example.err, true)
		if call.Outcome != example.want || (example.err != nil && call.Error != example.err.Error()) {
			t.Fatalf("actual error was substituted: %+v", call)
		}
	}
}

func TestUnboundHarnessAndInvalidProtocolRefuseWithoutStateWrites(t *testing.T) {
	root := t.TempDir()
	var out bytes.Buffer
	if err := run([]string{"positive", root, filepath.Join(root, "config.json")}, &out); err == nil || out.Len() != 0 {
		t.Fatal("unbound harness invoked library")
	}
	previous := linkedLibraryRevision
	linkedLibraryRevision = historicalRevision
	t.Cleanup(func() { linkedLibraryRevision = previous })
	for _, args := range [][]string{nil, {"positive"}, {"legacy_capture", root, "config"},
		{"positive", root, "config", "extra", "1"}} {
		if err := run(args, &out); err == nil || !strings.Contains(err.Error(), "usage:") || out.Len() != 0 {
			t.Fatalf("invalid protocol did not refuse early: %v %v", args, err)
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatal("protocol refusal created state")
	}
}
