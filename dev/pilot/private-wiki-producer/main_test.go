package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
)

func validConfig(t *testing.T) sourceConfig {
	t.Helper()
	policy, err := memory.LoadFile(filepath.Join("..", "..", "..", "bundles", "base", "memory", "policy.json"))
	if err != nil {
		t.Fatal(err)
	}
	return sourceConfig{Policy: policy, Budgets: map[string]int{"L1": 1024, "L2": 1024, "L3": 1024, "lifetime": 1024}}
}

func TestConfigPreservesExactSourceConfiguration(t *testing.T) {
	for _, freshness := range []map[string]time.Duration{nil, {"L1": 3 * time.Hour}} {
		config := validConfig(t)
		config.Freshness = freshness
		body, err := json.Marshal(config)
		if err != nil {
			t.Fatal(err)
		}
		actual, err := readConfig(bytes.NewReader(body))
		if err != nil || !reflect.DeepEqual(actual, config) {
			t.Fatalf("configuration changed: %+v %v", actual, err)
		}
	}
}

func TestConfigRejectsUnknownTrailingOversizedAndInvalidSettings(t *testing.T) {
	config := validConfig(t)
	valid, _ := json.Marshal(config)
	invalidPolicy := config
	invalidPolicy.Policy.SchemaVersion = 2
	policyBody, _ := json.Marshal(invalidPolicy)
	cases := map[string][]byte{
		"unknown":            append(append([]byte{}, valid[:len(valid)-1]...), []byte(`,"workspace":"other"}`)...),
		"nested_unknown":     bytes.Replace(valid, []byte(`"schema_version":1`), []byte(`"schema_version":1,"unknown":true`), 1),
		"trailing":           append(append([]byte{}, valid...), []byte(` {}`)...),
		"oversized":          append(bytes.Repeat([]byte(" "), (1<<20)+1), valid...),
		"invalid_policy":     policyBody,
		"missing_budgets":    bytes.Replace(valid, []byte(`"budgets":{`), []byte(`"budgets_missing":{`), 1),
		"zero_budget":        bytes.Replace(valid, []byte(`"L1":1024`), []byte(`"L1":0`), 1),
		"unknown_budget":     bytes.Replace(valid, []byte(`"L1":1024`), []byte(`"unknown":1024`), 1),
		"unknown_freshness":  bytes.Replace(valid, []byte(`"freshness":null`), []byte(`"freshness":{"unknown":1}`), 1),
		"negative_freshness": bytes.Replace(valid, []byte(`"freshness":null`), []byte(`"freshness":{"L1":-1}`), 1),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := readConfig(bytes.NewReader(body)); err == nil {
				t.Fatal("invalid producer configuration accepted")
			}
		})
	}
}

func TestArgumentsRefuseBeforeReadingOrWritingState(t *testing.T) {
	root := t.TempDir()
	for _, args := range [][]string{nil, {root}, {root, "relative.json"}, {"relative", filepath.Join(root, "config.json")}} {
		var out bytes.Buffer
		if err := run(args, &out); err == nil || out.Len() != 0 {
			t.Fatalf("invalid arguments admitted: %v %v", args, err)
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatal("argument refusal wrote state")
	}
	if _, err := readConfig(io.LimitReader(strings.NewReader("{}"), 0)); err == nil {
		t.Fatal("empty input admitted")
	}
}
