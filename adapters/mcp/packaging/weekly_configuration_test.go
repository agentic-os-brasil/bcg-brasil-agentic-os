package packaging

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
)

func weeklyConfigurationFiles(t *testing.T, body string) maintenance.WeeklyConfiguration {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	c := maintenance.WeeklyConfiguration{{Path: path, Role: "settings", SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(body)))}, {Path: filepath.Join(dir, "managed.json"), Role: "managed_policy", Absent: true}}
	sort.Slice(c, func(i, j int) bool { return c[i].Path < c[j].Path })
	return c
}

func TestWeeklyConfigurationVerification(t *testing.T) {
	for _, body := range []string{`{}`, `{"model":"accepted-model","theme":"dark","permissions":{"defaultMode":"dontAsk"}}`} {
		if err := VerifyWeeklyConfiguration(context.Background(), weeklyConfigurationFiles(t, body)); err != nil {
			t.Fatal(err)
		}
	}
	for _, body := range []string{`null`, `[]`, `{} {}`, `{"theme":"a","theme":"b"}`, `{"x":{"k":1,"k":2}}`, `{"hooks":{}}`, `{"env":{}}`, `{"enabledPlugins":{}}`, `{"mcpServers":{}}`, `{"agent":"worker"}`, `{"apiKeyHelper":"cmd"}`, `{"statusLine":{"type":"command","command":"cmd"}}`, strings.Repeat(" ", 65537)} {
		t.Run(fmt.Sprintf("denied-%x", sha256.Sum256([]byte(body))), func(t *testing.T) {
			if VerifyWeeklyConfiguration(context.Background(), weeklyConfigurationFiles(t, body)) == nil {
				t.Fatal("unsafe/ambiguous configuration admitted")
			}
		})
	}
	c := weeklyConfigurationFiles(t, `{}`)
	for i := range c {
		if c[i].Role == "settings" {
			if err := os.WriteFile(c[i].Path, []byte(`{"theme":"changed"}`), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	if VerifyWeeklyConfiguration(context.Background(), c) == nil {
		t.Fatal("changed content admitted")
	}
	c = weeklyConfigurationFiles(t, `{}`)
	for i := range c {
		if c[i].Absent {
			if err := os.WriteFile(c[i].Path, []byte(`{}`), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	if VerifyWeeklyConfiguration(context.Background(), c) == nil {
		t.Fatal("appeared absence admitted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if VerifyWeeklyConfiguration(ctx, weeklyConfigurationFiles(t, `{}`)) == nil {
		t.Fatal("cancelled check admitted")
	}
	if VerifyWeeklyConfiguration(nil, weeklyConfigurationFiles(t, `{}`)) == nil {
		t.Fatal("nil context admitted")
	}
}

func TestWeeklyConfigurationRefusesAliasesAndTotalSize(t *testing.T) {
	c := weeklyConfigurationFiles(t, `{}`)
	for i := range c {
		if !c[i].Absent {
			target := c[i].Path + ".real"
			if err := os.Rename(c[i].Path, target); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, c[i].Path); err != nil {
				t.Skip(err)
			}
		}
	}
	if VerifyWeeklyConfiguration(context.Background(), c) == nil {
		t.Fatal("symlink admitted")
	}
	body := []byte("{}" + strings.Repeat(" ", 65534))
	c = weeklyConfigurationFiles(t, string(body))
	var source string
	for _, e := range c {
		if !e.Absent {
			source = e.Path
		}
	}
	dir := filepath.Dir(source)
	for i := 0; i < 4; i++ {
		path := filepath.Join(dir, fmt.Sprintf("extra-%d.json", i))
		if err := os.WriteFile(path, body, 0600); err != nil {
			t.Fatal(err)
		}
		c = append(c, maintenance.WeeklyConfigurationEntry{Path: path, Role: "settings", SHA256: fmt.Sprintf("%x", sha256.Sum256(body))})
		if i == 2 {
			sort.Slice(c, func(i, j int) bool { return c[i].Path < c[j].Path })
			if err := VerifyWeeklyConfiguration(context.Background(), c); err != nil {
				t.Fatalf("exact total bound refused: %v", err)
			}
		}
	}
	sort.Slice(c, func(i, j int) bool { return c[i].Path < c[j].Path })
	if VerifyWeeklyConfiguration(context.Background(), c) == nil {
		t.Fatal("total size admitted")
	}
}

func TestWeeklyConfigurationAbsentAndHardlinkRefusal(t *testing.T) {
	c := weeklyConfigurationFiles(t, `{}`)
	for _, e := range c {
		if !e.Absent {
			if err := os.Link(e.Path, e.Path+".alias"); err != nil {
				t.Fatal(err)
			}
		}
	}
	if VerifyWeeklyConfiguration(context.Background(), c) == nil {
		t.Fatal("hardlink admitted")
	}
	c = weeklyConfigurationFiles(t, `{}`)
	for i := range c {
		if c[i].Absent {
			target := filepath.Join(filepath.Dir(c[i].Path), "existing-directory")
			if err := os.Mkdir(target, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, c[i].Path); err != nil {
				t.Fatal(err)
			}
		}
	}
	if VerifyWeeklyConfiguration(context.Background(), c) == nil {
		t.Fatal("absence accepted existing symlink")
	}
}
