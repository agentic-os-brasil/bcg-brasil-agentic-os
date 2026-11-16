package zipruntime

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func scopedQueue(t *testing.T) (*DailyQueue, string) {
	t.Helper()
	root := t.TempDir()
	physical, e := filepath.EvalSymlinks(root)
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(physical, "brain")
	if e = os.Mkdir(path, 0700); e != nil {
		t.Fatal(e)
	}
	q, e := OpenDailyQueue(path, physical, "owner")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(q.Close)
	return q, physical
}
func scopedCheckpoint(root, id string) DailyCheckpoint {
	return DailyCheckpoint{SchemaVersion: 1, ID: id, SessionID: "session-a", Workspace: root, Scope: "owner", CapturedAt: "2026-11-15T23:59:59-03:00", LocalDate: "2026-11-15", Summary: "Selected authored synthetic checkpoint", Decisions: []string{"Preserve scope"}, NextActions: []string{"Continue tomorrow"}, Provenance: "agent-authored"}
}
func dailyGuard() error { return nil }
func enqueueScoped(t *testing.T, q *DailyQueue, c DailyCheckpoint) {
	t.Helper()
	r, e := q.ReviewEnqueue(c)
	if e != nil {
		t.Fatal(e)
	}
	out, e := q.Enqueue(r, dailyGuard)
	if e != nil || out.State != "queued" {
		t.Fatalf("enqueue %+v %v", out, e)
	}
}
func dailySnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	e := filepath.WalkDir(root, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		rel, _ := filepath.Rel(root, p)
		if d.IsDir() {
			out[rel] = "directory"
			return nil
		}
		b, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		out[rel] = string(b)
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	return out
}
func TestDailyScopedStatusReadOnlyAndExactRetry(t *testing.T) {
	q, root := scopedQueue(t)
	before := dailySnapshot(t, root)
	status, e := q.Status()
	if e != nil || status.State != "empty" || status.Pending != 0 {
		t.Fatal(status, e)
	}
	if !reflect.DeepEqual(before, dailySnapshot(t, root)) {
		t.Fatal("status created state")
	}
	c := scopedCheckpoint(root, "one")
	enqueueScoped(t, q, c)
	enqueueScoped(t, q, c)
	different := c
	different.Summary = "Different payload"
	if _, e = q.ReviewEnqueue(different); e == nil {
		t.Fatal("conflicting ID admitted")
	}
	review, e := q.ReviewDrain([]string{"one"})
	if e != nil {
		t.Fatal(e)
	}
	out, e := q.Drain(review, dailyGuard)
	if e != nil || out.State != "persisted" || len(out.Entries) != 1 {
		t.Fatal(out, e)
	}
	generation := out.Generation
	page, e := os.ReadFile(filepath.Join(root, "brain/daily/2026-11-15.md"))
	if e != nil || !bytes.Contains(page, []byte(c.Summary)) || strings.Count(string(page), "<!-- maestro:daily-id ") != 1 {
		t.Fatal(string(page), e)
	}
	again, e := q.ReviewDrain([]string{"one"})
	if e != nil {
		t.Fatal(e)
	}
	out, e = q.Drain(again, dailyGuard)
	if e != nil || out.Generation != generation {
		t.Fatal("restart replay advanced generation", out, e)
	}
	ack, e := q.ReviewAck(generation)
	if e != nil {
		t.Fatal(e)
	}
	out, e = q.Ack(ack, dailyGuard)
	if e != nil || out.State != "acknowledged" {
		t.Fatal(out, e)
	}
	ack, e = q.ReviewAck(generation)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = q.Ack(ack, dailyGuard); e != nil {
		t.Fatal(e)
	}
}
func TestDailyScopedRecoversEachPublicationBoundary(t *testing.T) {
	for _, stage := range []string{"intent", "binding", "page", "generation", "committed", "proof", "request", "consumed"} {
		t.Run(stage, func(t *testing.T) {
			q, root := scopedQueue(t)
			c := scopedCheckpoint(root, "recover")
			enqueueScoped(t, q, c)
			review, e := q.ReviewDrain([]string{c.ID})
			if e != nil {
				t.Fatal(e)
			}
			expectedPrevious := []byte(nil)
			binding := q.binding(c)
			want := dailyHash(append(append(expectedPrevious, []byte(c.ID+" ")...), binding...))
			q.FaultPoint = func(at string) error {
				if at == stage {
					return errors.New("simulated crash")
				}
				return nil
			}
			if _, e = q.Drain(review, dailyGuard); e == nil {
				t.Fatal("fault did not interrupt")
			}
			q.Close()
			q, e = OpenDailyQueue(filepath.Join(root, "brain"), root, "owner")
			if e != nil {
				t.Fatal(e)
			}
			defer q.Close()
			snapshot := dailySnapshot(t, root)
			status, e := q.Status()
			if e != nil || status.Recovery == "" {
				t.Fatal(status, e)
			}
			if !reflect.DeepEqual(snapshot, dailySnapshot(t, root)) {
				t.Fatal("status repaired recovery")
			}
			review, e = q.ReviewDrain([]string{c.ID})
			if e != nil {
				t.Fatal(e)
			}
			out, e := q.Drain(review, dailyGuard)
			if e != nil || out.Generation != want {
				t.Fatal("recovery changed generation", out, e)
			}
			if e = q.verify(c); e != nil {
				t.Fatal(e)
			}
			status, e = q.Status()
			if e != nil || status.Pending != 0 || status.Recovery != "" || status.Dream != want {
				t.Fatal(status, e)
			}
		})
	}
}
func TestDailyScopedReviewCASAndQueueSwap(t *testing.T) {
	t.Run("queue replacement survives", func(t *testing.T) {
		q, root := scopedQueue(t)
		c := scopedCheckpoint(root, "swap")
		enqueueScoped(t, q, c)
		review, e := q.ReviewDrain([]string{c.ID})
		if e != nil {
			t.Fatal(e)
		}
		var replacement []byte
		q.FaultPoint = func(stage string) error {
			if stage == "page" {
				replacement = []byte(`{"replacement":"preserve"}`)
				if e = os.WriteFile(filepath.Join(root, "brain/.maestro/daily-pending/swap.json"), replacement, 0600); e != nil {
					t.Fatal(e)
				}
			}
			return nil
		}
		if _, e = q.Drain(review, dailyGuard); e == nil {
			t.Fatal("changed queue consumed")
		}
		got, e := os.ReadFile(filepath.Join(root, "brain/.maestro/daily-pending/swap.json"))
		if e != nil || !bytes.Equal(got, replacement) {
			t.Fatal("replacement lost", e)
		}
	})
	t.Run("unreviewed new input remains", func(t *testing.T) {
		q, root := scopedQueue(t)
		enqueueScoped(t, q, scopedCheckpoint(root, "first"))
		review, e := q.ReviewDrain([]string{"first"})
		if e != nil {
			t.Fatal(e)
		}
		enqueueScoped(t, q, scopedCheckpoint(root, "second"))
		if _, e = q.Drain(review, dailyGuard); e != nil {
			t.Fatal(e)
		}
		status, e := q.Status()
		if e != nil || status.Pending != 1 || status.Entries[0].ID != "second" {
			t.Fatal(status, e)
		}
	})
	t.Run("authored page changes preserved", func(t *testing.T) {
		q, root := scopedQueue(t)
		c := scopedCheckpoint(root, "page")
		enqueueScoped(t, q, c)
		review, e := q.ReviewDrain([]string{c.ID})
		if e != nil {
			t.Fatal(e)
		}
		dailyPut(t, filepath.Join(root, "brain/daily/2026-11-15.md"), "owner edited page\n")
		if _, e = q.Drain(review, dailyGuard); e == nil {
			t.Fatal("unreviewed page edit overwritten")
		}
		b, _ := os.ReadFile(filepath.Join(root, "brain/daily/2026-11-15.md"))
		if string(b) != "owner edited page\n" {
			t.Fatal("owner page lost")
		}
	})
	t.Run("old ack preserves new work", func(t *testing.T) {
		q, root := scopedQueue(t)
		enqueueScoped(t, q, scopedCheckpoint(root, "old"))
		r, e := q.ReviewDrain([]string{"old"})
		if e != nil {
			t.Fatal(e)
		}
		out, e := q.Drain(r, dailyGuard)
		if e != nil {
			t.Fatal(e)
		}
		ack, e := q.ReviewAck(out.Generation)
		if e != nil {
			t.Fatal(e)
		}
		enqueueScoped(t, q, scopedCheckpoint(root, "new"))
		r, e = q.ReviewDrain([]string{"new"})
		if e != nil {
			t.Fatal(e)
		}
		out, e = q.Drain(r, dailyGuard)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = q.Ack(ack, dailyGuard); e == nil {
			t.Fatal("stale acknowledgement accepted")
		}
		status, e := q.Status()
		if e != nil || status.Dream != out.Generation {
			t.Fatal("new work cleared", status, e)
		}
	})
}

func TestDailyScopedRecoveryWithPriorGenerationAndLegacyCaller(t *testing.T) {
	q, root := scopedQueue(t)
	enqueueScoped(t, q, scopedCheckpoint(root, "prior"))
	r, e := q.ReviewDrain([]string{"prior"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = q.Drain(r, dailyGuard); e != nil {
		t.Fatal(e)
	}
	enqueueScoped(t, q, scopedCheckpoint(root, "interrupted"))
	r, e = q.ReviewDrain([]string{"interrupted"})
	if e != nil {
		t.Fatal(e)
	}
	q.FaultPoint = func(stage string) error {
		if stage == "generation" {
			return errors.New("interrupted before request")
		}
		return nil
	}
	if _, e = q.Drain(r, dailyGuard); e == nil {
		t.Fatal("fault not observed")
	}
	q.FaultPoint = nil
	status, e := q.Status()
	if e != nil || status.Recovery == "" {
		t.Fatal("valid prior dream request blocked recovery", status, e)
	}
	if got := dailyRun(t, root, "daily-stop"); !strings.Contains(got, `"replayed":1`) {
		t.Fatal("legacy caller failed to continue scoped transition", got)
	}
	enqueueScoped(t, q, scopedCheckpoint(root, "consumed"))
	r, e = q.ReviewDrain([]string{"consumed"})
	if e != nil {
		t.Fatal(e)
	}
	q.FaultPoint = func(stage string) error {
		if stage == "consumed" {
			return errors.New("interrupted after consumption")
		}
		return nil
	}
	if _, e = q.Drain(r, dailyGuard); e == nil {
		t.Fatal("fault not observed")
	}
	q.FaultPoint = nil
	if got := dailyRun(t, root, "daily-stop"); !strings.Contains(got, `"replayed":1`) {
		t.Fatal("legacy caller lost consumed transition", got)
	}
	status, e = q.Status()
	if e != nil || status.Pending != 0 || status.Recovery != "" {
		t.Fatal(status, e)
	}
}
func TestDailyScopedReviewBudgetAndLegacyLargePage(t *testing.T) {
	q, root := scopedQueue(t)
	prefix := strings.Repeat("prior authored prose\n", (2<<20)/20)
	path := filepath.Join(root, "brain/daily/2026-11-15.md")
	dailyPut(t, path, prefix)
	enqueueScoped(t, q, scopedCheckpoint(root, "large"))
	before := dailySnapshot(t, root)
	if _, e := q.ReviewDrain([]string{"large"}); e == nil || e.Error() != "daily_review_too_large" {
		t.Fatal("unbounded current/proposed review", e)
	}
	if !reflect.DeepEqual(before, dailySnapshot(t, root)) {
		t.Fatal("overbudget review changed data")
	}
	if got := dailyRun(t, root, "daily-stop"); !strings.Contains(got, `"saved":1`) {
		t.Fatal("existing >1MiB daily page regressed", got)
	}
	body, e := os.ReadFile(path)
	if e != nil || !strings.HasPrefix(string(body), prefix) {
		t.Fatal("large authored page changed", e)
	}
}
func TestDailyScopedAncestryAndLiveBindingGuard(t *testing.T) {
	t.Run("renamed scope refuses writes", func(t *testing.T) {
		q, root := scopedQueue(t)
		r, e := q.ReviewEnqueue(scopedCheckpoint(root, "bound"))
		if e != nil {
			t.Fatal(e)
		}
		if e = os.Rename(filepath.Join(root, "brain"), filepath.Join(root, "retained")); e != nil {
			t.Fatal(e)
		}
		if e = os.Mkdir(filepath.Join(root, "brain"), 0700); e != nil {
			t.Fatal(e)
		}
		before := dailySnapshot(t, root)
		if _, e = q.Enqueue(r, dailyGuard); e == nil {
			t.Fatal("renamed scope admitted")
		}
		if !reflect.DeepEqual(before, dailySnapshot(t, root)) {
			t.Fatal("replacement scope changed")
		}
	})
	t.Run("live guard runs at publication", func(t *testing.T) {
		q, root := scopedQueue(t)
		enqueueScoped(t, q, scopedCheckpoint(root, "guarded"))
		r, e := q.ReviewDrain([]string{"guarded"})
		if e != nil {
			t.Fatal(e)
		}
		valid := true
		q.FaultPoint = func(stage string) error {
			if stage == "intent" {
				valid = false
			}
			return nil
		}
		guard := func() error {
			if !valid {
				return errors.New("scope changed")
			}
			return nil
		}
		if _, e = q.Drain(r, guard); e == nil {
			t.Fatal("binding change ignored")
		}
		if _, e = os.Stat(filepath.Join(root, "brain/daily/2026-11-15.md")); !os.IsNotExist(e) {
			t.Fatal("page published after binding loss", e)
		}
		if _, e = os.Stat(filepath.Join(root, "brain/.maestro/daily-pending/guarded.json")); e != nil {
			t.Fatal("pending input lost", e)
		}
	})
}

func TestDailyScopedEscapedPayloadRecovery(t *testing.T) {
	q, root := scopedQueue(t)
	c := scopedCheckpoint(root, "escaped")
	c.Summary = strings.Repeat("<>&", 1300)
	enqueueScoped(t, q, c)
	r, e := q.ReviewDrain([]string{c.ID})
	if e != nil {
		t.Fatal(e)
	}
	q.FaultPoint = func(stage string) error {
		if stage == "intent" {
			return errors.New("interrupt escaped intent")
		}
		return nil
	}
	if _, e = q.Drain(r, dailyGuard); e == nil {
		t.Fatal("fault not observed")
	}
	q.FaultPoint = nil
	if _, e = q.Status(); e != nil {
		t.Fatal("valid expanded intent rejected", e)
	}
	r, e = q.ReviewDrain([]string{c.ID})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = q.Drain(r, dailyGuard); e != nil {
		t.Fatal(e)
	}
	if e = q.verify(c); e != nil {
		t.Fatal(e)
	}
}

func TestDailyScopedLegacyCommittedPendingBuildsProof(t *testing.T) {
	q, root := scopedQueue(t)
	c := scopedCheckpoint(root, "legacy")
	enqueueScoped(t, q, c)
	r, e := q.ReviewDrain([]string{c.ID})
	if e != nil {
		t.Fatal(e)
	}
	out, e := q.Drain(r, dailyGuard)
	if e != nil {
		t.Fatal(e)
	}
	generation := out.Generation
	if e = os.Remove(filepath.Join(root, "brain/.maestro/daily-proofs/legacy.json")); e != nil {
		t.Fatal(e)
	}
	dailyPut(t, filepath.Join(root, "brain/.maestro/daily-pending/legacy.json"), string(dailyCheckpointBytes(c)))
	page, e := os.ReadFile(filepath.Join(root, "brain/daily/2026-11-15.md"))
	if e != nil {
		t.Fatal(e)
	}
	r, e = q.ReviewDrain([]string{c.ID})
	if e != nil {
		t.Fatal(e)
	}
	out, e = q.Drain(r, dailyGuard)
	if e != nil || out.Generation != generation {
		t.Fatal("legacy pending replay changed generation", out, e)
	}
	after, e := os.ReadFile(filepath.Join(root, "brain/daily/2026-11-15.md"))
	if e != nil || !bytes.Equal(page, after) {
		t.Fatal("legacy page changed", e)
	}
	r, e = q.ReviewDrain([]string{c.ID})
	if e != nil {
		t.Fatal("completed legacy retry lacks compact proof", e)
	}
	out, e = q.Drain(r, dailyGuard)
	if e != nil || out.Generation != generation {
		t.Fatal("legacy ID-only retry failed", out, e)
	}
}
