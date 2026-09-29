package zipruntime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func dailyPut(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestDailyRecoveryDreamAckAndBound(t *testing.T) {
	root := t.TempDir()
	q := filepath.Join(root, "brain/.maestro/daily-pending/a.json")
	body := checkpoint(t, root, "owner", "a", "", "newest-important-work")
	dailyPut(t, q, body)
	// A failed request write occurs after page publication: retry must not append.
	dailyPut(t, filepath.Join(root, "brain/memory"), "obstruction")
	got := dailyRun(t, root, "daily-stop")
	if !strings.Contains(got, `"failed":1`) {
		t.Fatal(got)
	}
	os.Remove(filepath.Join(root, "brain/memory"))
	got = dailyRun(t, root, "daily-stop")
	if !strings.Contains(got, `"replayed":1`) {
		t.Fatal(got)
	}
	if !strings.Contains(dailyRun(t, root, "daily-context"), "newest-important-work") {
		t.Fatal("newest checkpoint missing")
	}
	marker := filepath.Join(root, "brain/memory/.dream-requested")
	a, _ := os.ReadFile(marker)
	dailyPut(t, filepath.Join(root, "brain/.maestro/daily-pending/b.json"), checkpoint(t, root, "owner", "b", "", "later-work"))
	dailyRun(t, root, "daily-stop")
	ack := func(digest []byte) error {
		var out bytes.Buffer
		payload, _ := json.Marshal(map[string]string{"scope": "owner", "digest": strings.TrimSpace(string(digest))})
		return Run([]string{"daily-dream-ack", "--root", root}, bytes.NewReader(payload), &out)
	}
	if ack(a) == nil {
		t.Fatal("old dream cleared new work")
	}
	b, _ := os.ReadFile(marker)
	if err := ack(b); err != nil {
		t.Fatal(err)
	}
	dailyRun(t, root, "daily-stop")
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("no-op rearmed dream")
	}
	for i := 0; i < 33; i++ {
		id := fmt.Sprintf("n%d", i)
		dailyPut(t, filepath.Join(root, "brain/.maestro/daily-pending", id+".json"), checkpoint(t, root, "owner", id, "", "bounded"))
	}
	got = dailyRun(t, root, "daily-stop")
	if !strings.Contains(got, `"saved":32`) {
		t.Fatal(got)
	}
	pending, _ := os.ReadDir(filepath.Dir(q))
	if len(pending) != 1 {
		t.Fatal(len(pending))
	}
}

func TestDailyValidationAndLongScopeBudget(t *testing.T) {
	root := t.TempDir()
	scopeID := strings.Repeat("a", 64) + "/" + strings.Repeat("b", 64)
	dailyPut(t, filepath.Join(root, "brain/accounts/.active"), scopeID)
	parts := strings.Split(scopeID, "/")
	scopePath := filepath.Join(root, "brain/accounts", parts[0], "cases", parts[1])
	for _, base := range []string{filepath.Join(root, "brain"), scopePath} {
		dailyPut(t, filepath.Join(base, "memory/.dream-requested"), strings.Repeat("a", 64)+"\n")
		for _, layer := range []string{"daily", "memory/recent"} {
			for i := 0; i < 3; i++ {
				date := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
				dailyPut(t, filepath.Join(base, layer, date+".md"), strings.Repeat("é", 2000))
			}
		}
	}
	got := dailyRun(t, root, "daily-context")
	if len(got) > 5000 {
		t.Fatal(len(got))
	}
	base := checkpoint(t, root, "owner", "bad", "", "work")
	for _, body := range []string{base + " {}", strings.Replace(base, root, root+"-wrong", 1), strings.Replace(base, "agent-authored", "attested", 1), string([]byte{0xff})} {
		dailyPut(t, filepath.Join(root, "brain/.maestro/daily-pending/bad.json"), body)
		if got := dailyRun(t, root, "daily-stop"); !strings.Contains(got, `"failed":1`) {
			t.Fatal(got)
		}
	}
}
func dailyRun(t *testing.T, root, command string) string {
	t.Helper()
	var out bytes.Buffer
	if err := Run([]string{command, "--root", root}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	return out.String()
}
func checkpoint(t *testing.T, root, scope, id, date, summary string) string {
	t.Helper()
	now := time.Now()
	if date == "" {
		date = now.Format("2006-01-02")
	}
	p := map[string]any{"schema_version": 1, "id": id, "session_id": "test-session", "workspace": root, "scope": scope, "captured_at": date + "T23:59:00-03:00", "local_date": date, "summary": summary, "decisions": []string{"decision"}, "next_actions": []string{"next"}, "provenance": "agent-authored"}
	b, _ := json.Marshal(p)
	return string(b)
}
func TestDailyContextWindow(t *testing.T) {
	root := t.TempDir()
	for i := -1; i < 4; i++ {
		day := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		dailyPut(t, filepath.Join(root, "brain/daily", day+".md"), "marker-"+day)
	}
	dailyPut(t, filepath.Join(root, "brain/daily/index.md"), "INDEX-MUST-NOT-APPEAR")
	got := dailyRun(t, root, "daily-context")
	for i := 0; i < 3; i++ {
		if !strings.Contains(got, "marker-"+time.Now().AddDate(0, 0, -i).Format("2006-01-02")) {
			t.Fatal(got)
		}
	}
	for _, i := range []int{-1, 3} {
		if strings.Contains(got, "marker-"+time.Now().AddDate(0, 0, -i).Format("2006-01-02")) {
			t.Fatal(got)
		}
	}
}
func TestDailyWriterReplayConflictAndPreservation(t *testing.T) {
	root := t.TempDir()
	date := time.Now().Format("2006-01-02")
	page := filepath.Join(root, "brain/daily", date+".md")
	dailyPut(t, page, "existing bytes\n")
	queue := filepath.Join(root, "brain/.maestro/daily-pending/a.json")
	body := checkpoint(t, root, "owner", "a", date, "work")
	dailyPut(t, queue, body)
	got := dailyRun(t, root, "daily-stop")
	if !strings.Contains(got, `"saved":1`) {
		t.Fatal(got)
	}
	dailyPut(t, queue, body)
	dailyRun(t, root, "daily-stop")
	saved, _ := os.ReadFile(page)
	if !strings.HasPrefix(string(saved), "existing bytes\n") || strings.Count(string(saved), "\nwork\n") != 1 {
		t.Fatal(string(saved))
	}
	dailyPut(t, queue, checkpoint(t, root, "owner", "a", date, "conflict"))
	got = dailyRun(t, root, "daily-stop")
	if !strings.Contains(got, `"failed":1`) {
		t.Fatal(got)
	}
	if _, err := os.Stat(queue); err != nil {
		t.Fatal(err)
	}
}
func TestDailyIsolationMidnightAndInvalid(t *testing.T) {
	root := t.TempDir()
	dailyPut(t, filepath.Join(root, "brain/accounts/.active"), "acct/case")
	scope := filepath.Join(root, "brain/accounts/acct/cases/case")
	os.MkdirAll(scope, 0700)
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	dailyPut(t, filepath.Join(scope, ".maestro/daily-pending/c.json"), checkpoint(t, root, "account/acct/case/case", "c", yesterday, "case-only"))
	dailyPut(t, filepath.Join(root, "brain/.maestro/daily-pending/bad.json"), checkpoint(t, root, "acct/case", "bad", yesterday, "must-not-save"))
	dailyPut(t, filepath.Join(root, "brain/.maestro/daily-pending/empty.json"), "")
	dailyPut(t, filepath.Join(root, "brain/.maestro/daily-pending/large.json"), strings.Repeat("x", 20000))
	got := dailyRun(t, root, "daily-stop")
	if !strings.Contains(got, `"saved":1`) || !strings.Contains(got, `"failed":3`) {
		t.Fatal(got)
	}
	b, err := os.ReadFile(filepath.Join(scope, "daily", yesterday+".md"))
	if err != nil || !strings.Contains(string(b), "case-only") {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "brain/daily", yesterday+".md")); err == nil {
		t.Fatal("case leaked")
	}
	dailyPut(t, filepath.Join(root, "brain/accounts/.active"), "other/case")
	if strings.Contains(dailyRun(t, root, "daily-context"), "case-only") {
		t.Fatal("inactive case leaked")
	}
}
func TestDailyBoundedConcurrentAndAliases(t *testing.T) {
	root := t.TempDir()
	q := filepath.Join(root, "brain/.maestro/daily-pending/a.json")
	dailyPut(t, q, checkpoint(t, root, "owner", "a", "", "once"))
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var out bytes.Buffer
			_ = Run([]string{"daily-stop", "--root", root}, strings.NewReader(""), &out)
		}()
	}
	wg.Wait()
	page := filepath.Join(root, "brain/daily", time.Now().Format("2006-01-02")+".md")
	b, _ := os.ReadFile(page)
	if strings.Count(string(b), "\nonce\n") != 1 {
		t.Fatal(string(b))
	}
	dailyPut(t, page, strings.Repeat("é", 10000))
	got := dailyRun(t, root, "daily-context")
	if len(got) > 8192 || !strings.Contains(got, "omitted") {
		t.Fatal(len(got), got)
	}
	other := t.TempDir()
	os.Remove(page)
	if err := os.Symlink(filepath.Join(other, "out.md"), page); err != nil {
		t.Skip(err)
	}
	dailyPut(t, q, checkpoint(t, root, "owner", "b", "", "escape"))
	got = dailyRun(t, root, "daily-stop")
	if !strings.Contains(got, `"failed":1`) {
		t.Fatal(got)
	}
}
