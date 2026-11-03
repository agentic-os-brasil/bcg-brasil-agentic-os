package privatewiki

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func compilerInput() Input {
	return Input{HomeRef: strings.Repeat("a", 64), WorkspaceRef: strings.Repeat("b", 64), CommitRef: strings.Repeat("c", 64), Watermark: strings.Repeat("d", 64), Implementation: strings.Repeat("e", 64), Configuration: strings.Repeat("f", 64), PolicyRef: strings.Repeat("1", 64), CommittedAt: time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC), Sources: []Source{{ID: strings.Repeat("2", 64), Version: strings.Repeat("3", 64), Reference: strings.Repeat("4", 64), Layer: "L1", Period: "2026-07-20", Content: "Known source marker\n[untrusted](../../outside.md)\n```\n<untrusted-html>"}}}
}

func TestPrivateWikiCompilerProducesDeterministicLiteralRoutes(t *testing.T) {
	in := compilerInput()
	a, err := Compile(in)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Compile(in)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatal("same admitted source changed graph bytes")
	}
	name := "concepts/" + strings.Repeat("2", 64) + ".md"
	if !strings.Contains(string(a.Files["index.md"]), "[L1 2026-07-20]("+name+")") {
		t.Fatal("independent daily route absent")
	}
	if !strings.Contains(string(a.Files[name]), "Known source marker") {
		t.Fatal("known rollup summary absent")
	}
	var backlinks map[string][]string
	if err := json.Unmarshal(a.Files["backlinks.json"], &backlinks); err != nil {
		t.Fatal(err)
	}
	if strings.Join(backlinks[name], ",") != "index.md,layers/l1/index.md" {
		t.Fatalf("unexpected actual backlinks: %v", backlinks[name])
	}
	if strings.Contains(string(a.Files["log.md"]), "Known source marker") {
		t.Fatal("private body reached metadata log")
	}
}

func TestPrivateWikiCompilerRefusesSecurityAndBacklinkTampering(t *testing.T) {
	for _, kind := range []string{"authority", "backlinks"} {
		t.Run(kind, func(t *testing.T) {
			c, err := Compile(compilerInput())
			if err != nil {
				t.Fatal(err)
			}
			name := "concepts/" + strings.Repeat("2", 64) + ".md"
			if kind == "authority" {
				c.Files[name] = []byte(strings.Replace(string(c.Files[name]), "local-home-workspace", "invented-enterprise", 1))
			} else {
				name = "backlinks.json"
				c.Files[name] = []byte(`{}`)
			}
			c.Manifest.Files[name] = Digest(c.Files[name])
			b, _ := json.Marshal(c.Manifest)
			c.Files["manifest.json"] = b
			if err := Validate(c); !errors.Is(err, ErrInvalid) {
				t.Fatalf("tampered %s admitted: %v", kind, err)
			}
		})
	}
}

func TestPrivateWikiCompilerRefusesBudgetsWithoutDroppingSources(t *testing.T) {
	in := compilerInput()
	in.Sources[0].Content = strings.Repeat("x", MaxSourceBytes+1)
	if _, err := Compile(in); !errors.Is(err, ErrBudget) {
		t.Fatalf("individual source overflow admitted: %v", err)
	}
	in = compilerInput()
	one := in.Sources[0]
	in.Sources = nil
	for i := 0; i < MaxSources; i++ {
		s := one
		s.ID = fmt.Sprintf("%064x", i+1)
		s.Reference = fmt.Sprintf("%064x", i+1000)
		s.Content = strings.Repeat("x", 65537)
		in.Sources = append(in.Sources, s)
	}
	if _, err := Compile(in); !errors.Is(err, ErrBudget) {
		t.Fatalf("aggregate source overflow admitted: %v", err)
	}
	in = compilerInput()
	in.Sources = make([]Source, MaxSources+1)
	if _, err := Compile(in); !errors.Is(err, ErrBudget) {
		t.Fatalf("source count overflow admitted: %v", err)
	}
}
