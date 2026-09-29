package zipruntime

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestDailyReviewBOMActiveCase(t *testing.T) {
	root := t.TempDir()
	dailyPut(t, filepath.Join(root, "brain/accounts/.active"), "\uFEFFac/ca\r\n")
	date := time.Now().Format("2006-01-02")
	dailyPut(t, filepath.Join(root, "brain/accounts/ac/cases/ca/daily", date+".md"), "BOM-case-history")
	if !strings.Contains(dailyRun(t, root, "daily-context"), "BOM-case-history") {
		t.Fatal("PS5 active marker lost case scope")
	}
}

func TestDailyReviewCorruptGenerationRetainsCheckpoint(t *testing.T) {
	for _, replay := range []bool{true, false} {
		root := t.TempDir()
		queue := filepath.Join(root, "brain/.maestro/daily-pending/a.json")
		a := checkpoint(t, root, "owner", "a", "", "A")
		dailyPut(t, queue, a)
		dailyRun(t, root, "daily-stop")
		marker := filepath.Join(root, "brain/memory/.dream-requested")
		before, _ := os.ReadFile(marker)
		dailyPut(t, filepath.Join(root, "brain/.maestro/daily-generation"), "corrupt\n")
		if !replay {
			queue = filepath.Join(root, "brain/.maestro/daily-pending/b.json")
			a = checkpoint(t, root, "owner", "b", "", "B")
		}
		dailyPut(t, queue, a)
		page := filepath.Join(root, "brain/daily", time.Now().Format("2006-01-02")+".md")
		pageBefore, _ := os.ReadFile(page)
		got := dailyRun(t, root, "daily-stop")
		if !strings.Contains(got, `"failed":1`) {
			t.Fatal(got)
		}
		if _, e := os.Stat(queue); e != nil {
			t.Fatal("corrupt generation consumed queue")
		}
		after, _ := os.ReadFile(marker)
		pageAfter, _ := os.ReadFile(page)
		if !bytes.Equal(before, after) || !bytes.Equal(pageBefore, pageAfter) {
			t.Fatal("corrupt generation mutated durable work")
		}
	}
}

func TestDailyReviewLegacyDreamRequest(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "brain/memory/.dream-requested")
	legacy := "2026-09-28T23:40:00Z\n"
	dailyPut(t, marker, legacy)
	if !strings.Contains(dailyRun(t, root, "daily-context"), "Dream pending:") {
		t.Fatal("legacy request silently omitted")
	}
	got := dailyRun(t, root, "daily-stop")
	if strings.Contains(got, `"failed":1`) {
		t.Fatal(got)
	}
	digest, _ := os.ReadFile(marker)
	if len(strings.TrimSpace(string(digest))) != 64 {
		t.Fatal("legacy request not normalized", string(digest))
	}
	preserved, e := os.ReadFile(filepath.Join(root, "brain/memory/.dream-legacy-request"))
	if e != nil || string(preserved) != legacy {
		t.Fatal("legacy provenance not preserved")
	}
	payload, _ := json.Marshal(map[string]string{"scope": "owner", "digest": strings.TrimSpace(string(digest))})
	var out bytes.Buffer
	if e = Run([]string{"daily-dream-ack", "--root", root}, bytes.NewReader(payload), &out); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(marker); !os.IsNotExist(e) {
		t.Fatal("legacy success remains stuck")
	}
	dailyRun(t, root, "daily-stop")
	if _, e = os.Stat(marker); !os.IsNotExist(e) {
		t.Fatal("legacy rearmed on no-op")
	}
}

func TestDailyReviewCalendarClock(t *testing.T) {
	zone, e := time.LoadLocation("America/New_York")
	if e != nil {
		t.Fatal(e)
	}
	for _, now := range []time.Time{time.Date(2027, 1, 1, 0, 10, 0, 0, zone), time.Date(2026, 3, 9, 0, 10, 0, 0, zone), time.Date(2026, 9, 29, 23, 30, 0, 0, zone)} {
		root := t.TempDir()
		for i := -1; i <= 3; i++ {
			day := now.AddDate(0, 0, -i).Format("2006-01-02")
			dailyPut(t, filepath.Join(root, "brain/daily", day+".md"), "calendar-"+day)
		}
		var out bytes.Buffer
		if e := dailyContextAt(root, &out, now); e != nil {
			t.Fatal(e)
		}
		for i := 0; i <= 2; i++ {
			if !strings.Contains(out.String(), "calendar-"+now.AddDate(0, 0, -i).Format("2006-01-02")) {
				t.Fatal(out.String())
			}
		}
		for _, i := range []int{-1, 3} {
			if strings.Contains(out.String(), "calendar-"+now.AddDate(0, 0, -i).Format("2006-01-02")) {
				t.Fatal(out.String())
			}
		}
	}
}

func TestDailyReviewReplayCannotRewindRequest(t *testing.T) {
	root := t.TempDir()
	queue := filepath.Join(root, "brain/.maestro/daily-pending")
	marker := filepath.Join(root, "brain/memory/.dream-requested")
	a := checkpoint(t, root, "owner", "a", "", "A")
	dailyPut(t, filepath.Join(queue, "a.json"), a)
	dailyRun(t, root, "daily-stop")
	old, _ := os.ReadFile(marker)
	dailyPut(t, filepath.Join(queue, "b.json"), checkpoint(t, root, "owner", "b", "", "B"))
	dailyRun(t, root, "daily-stop")
	current, _ := os.ReadFile(marker)
	dailyPut(t, filepath.Join(queue, "a.json"), a)
	dailyRun(t, root, "daily-stop")
	after, _ := os.ReadFile(marker)
	if !bytes.Equal(current, after) {
		t.Fatal("replay rewound cumulative request")
	}
	payload, _ := json.Marshal(map[string]string{"scope": "owner", "digest": strings.TrimSpace(string(old))})
	var out bytes.Buffer
	if Run([]string{"daily-dream-ack", "--root", root}, bytes.NewReader(payload), &out) == nil {
		t.Fatal("A acknowledged B")
	}
}

func TestDailyReviewMigrationBlocksEveryMutation(t *testing.T) {
	for _, command := range []string{"daily-stop", "daily-dream-ack", "codex-stop"} {
		t.Run(command, func(t *testing.T) {
			root := t.TempDir()
			dailyPut(t, filepath.Join(root, "data/profile/identity.md"), "legacy")
			dailyPut(t, filepath.Join(root, "brain/owner/identity.md"), "current")
			q := filepath.Join(root, "brain/.maestro/daily-pending/a.json")
			dailyPut(t, q, checkpoint(t, root, "owner", "a", "", "blocked"))
			marker := filepath.Join(root, "brain/memory/.dream-requested")
			dailyPut(t, marker, strings.Repeat("a", 64)+"\n")
			args := []string{command, "--root", root}
			payload := `{"scope":"owner","digest":"` + strings.Repeat("a", 64) + `"}`
			if command == "codex-stop" {
				args = []string{"codex-hook", "Stop", "--root", root}
				payload = `{"session_id":"test"}`
			}
			var out bytes.Buffer
			_ = Run(args, strings.NewReader(payload), &out)
			if _, e := os.Stat(q); e != nil {
				t.Fatal("queue consumed despite blocked migration")
			}
			if _, e := os.Stat(marker); e != nil {
				t.Fatal("ack consumed despite blocked migration")
			}
			if !strings.Contains(out.String(), "migration_blocked") {
				t.Fatal("missing bounded migration diagnostic", out.String())
			}
		})
	}
}

func TestDailyReviewCanonicalFrontmatter(t *testing.T) {
	root := t.TempDir()
	date := time.Now().Format("2006-01-02")
	dailyPut(t, filepath.Join(root, "brain/accounts/.active"), "ac/ca")
	for _, tc := range []struct{ base, scope, sensitivity string }{{"brain", "owner", "owner-private"}, {"brain/accounts/ac/cases/ca", "account/ac/case/ca", "client-confidential"}} {
		dailyPut(t, filepath.Join(root, tc.base, ".maestro/daily-pending/a.json"), checkpoint(t, root, tc.scope, "a", date, "body"))
		dailyRun(t, root, "daily-stop")
		b, e := os.ReadFile(filepath.Join(root, tc.base, "daily", date+".md"))
		if e != nil {
			t.Fatal(e)
		}
		if !strings.Contains(string(b), "id: "+strings.TrimPrefix(strings.TrimPrefix(tc.base+"/", "brain/"), "/")+"daily/"+date+"\n") || !strings.Contains(string(b), "sensitivity: "+tc.sensitivity+"\n") {
			t.Fatal(string(b))
		}
	}
}

func TestDailyReviewFailureVisibleInCodex(t *testing.T) {
	for _, event := range []string{"SessionStart", "Stop"} {
		root := t.TempDir()
		dailyPut(t, filepath.Join(root, "brain/.maestro/daily-pending/a.json"), "invalid-private-checkpoint")
		dailyPut(t, filepath.Join(root, "brain/daily", time.Now().Format("2006-01-02")+".md"), "valid-history")
		var out bytes.Buffer
		if e := Run([]string{"codex-hook", event, "--root", root}, strings.NewReader(`{"session_id":"test"}`), &out); e != nil {
			t.Fatal(e)
		}
		if !strings.Contains(out.String(), "daily_recovery") || strings.Contains(out.String(), "invalid-private-checkpoint") {
			t.Fatal(out.String())
		}
		if event == "SessionStart" && !strings.Contains(out.String(), "valid-history") {
			t.Fatal("recovery failure hid history")
		}
	}
}

func TestDailyReviewHardlinksAndFIFO(t *testing.T) {
	t.Run("hardlink", func(t *testing.T) {
		root := t.TempDir()
		source := filepath.Join(root, "brain/accounts/inactive/cases/c/daily/secret.md")
		dailyPut(t, source, "inactive-secret-body")
		target := filepath.Join(root, "brain/daily", time.Now().Format("2006-01-02")+".md")
		os.MkdirAll(filepath.Dir(target), 0700)
		if e := os.Link(source, target); e != nil {
			t.Skip(e)
		}
		got := dailyRun(t, root, "daily-context")
		if strings.Contains(got, "inactive-secret-body") || !strings.Contains(got, "unavailable") {
			t.Fatal(got)
		}
	})
	t.Run("fifo", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Unix FIFO fixture")
		}
		root := t.TempDir()
		fifo := filepath.Join(root, "brain/.maestro/daily-pending/a.json")
		os.MkdirAll(filepath.Dir(fifo), 0700)
		if e := exec.Command("mkfifo", fifo).Run(); e != nil {
			t.Skip(e)
		}
		done := make(chan string, 1)
		go func() {
			var out bytes.Buffer
			_ = Run([]string{"daily-stop", "--root", root}, strings.NewReader(""), &out)
			done <- out.String()
		}()
		select {
		case got := <-done:
			if !strings.Contains(got, `"failed":1`) {
				t.Fatal(got)
			}
		case <-time.After(300 * time.Millisecond):
			f, _ := os.OpenFile(fifo, os.O_WRONLY, 0600)
			if f != nil {
				f.Close()
			}
			<-done
			t.Fatal("FIFO blocked daily-stop")
		}
	})
}
