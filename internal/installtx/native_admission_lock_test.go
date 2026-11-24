package installtx

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// These callbacks exercise only serialization, never authenticated source or a
// native proof. Each request must enter once and retain its exclusive interval.
func TestNativeAdmissionLockSerializesCallbacks(t *testing.T) {
	root := t.TempDir()
	firstReady, releaseFirst := make(chan struct{}), make(chan struct{})
	firstDone, secondDone := make(chan error, 1), make(chan error, 1)
	var active, firstCalls, secondCalls atomic.Int32
	use := func(calls *atomic.Int32, hold bool) func(time.Time) error {
		return func(time.Time) error {
			calls.Add(1)
			if active.Add(1) != 1 {
				return errors.New("native admission callbacks overlapped")
			}
			defer active.Add(-1)
			if hold {
				close(firstReady)
				<-releaseFirst
			}
			return nil
		}
	}
	go func() {
		firstDone <- withNativeAdmissionLock(context.Background(), root, time.Now(), acquireNativeAdmissionLockContext, use(&firstCalls, true))
	}()
	select {
	case <-firstReady:
	case err := <-firstDone:
		t.Fatal("first native reader failed before its held operation", err)
	case <-time.After(2 * time.Second):
		t.Fatal("first native reader never reached its held operation")
	}
	go func() {
		secondDone <- withNativeAdmissionLock(context.Background(), root, time.Now(), acquireNativeAdmissionLockContext, use(&secondCalls, false))
	}()
	time.Sleep(40 * time.Millisecond)
	if secondCalls.Load() != 0 {
		close(releaseFirst)
		t.Fatal("second callback entered while the original owner held its lock")
	}
	close(releaseFirst)
	if firstErr, secondErr := <-firstDone, <-secondDone; firstErr != nil || secondErr != nil || firstCalls.Load() != 1 || secondCalls.Load() != 1 {
		t.Fatalf("native admission did not serialize once: %v %v calls=%d/%d", firstErr, secondErr, firstCalls.Load(), secondCalls.Load())
	}
}

func TestNativeAdmissionWaitBoundsPreserveForeignLock(t *testing.T) {
	for _, name := range []string{"cap", "cancel-before", "cancel-waiting", "earlier-deadline"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			unlock, err := acquireLock(root)
			if err != nil {
				t.Fatal(err)
			}
			defer unlock()
			path := filepath.Join(root, ".activation.lock")
			before, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			want := os.ErrExist
			switch name {
			case "cancel-before":
				cancel()
				want = context.Canceled
			case "cancel-waiting":
				time.AfterFunc(25*time.Millisecond, cancel)
				want = context.Canceled
			case "earlier-deadline":
				ctx, cancel = context.WithTimeout(ctx, 25*time.Millisecond)
				defer cancel()
				want = context.DeadlineExceeded
			}
			calls := 0
			started := time.Now()
			err = withNativeAdmissionLock(ctx, root, started, acquireNativeAdmissionLockContext, func(time.Time) error { calls++; return nil })
			elapsed := time.Since(started)
			if !errors.Is(err, want) || calls != 0 || elapsed > 2*time.Second {
				t.Fatalf("native acquisition bound/cancellation changed: calls=%d elapsed=%s err=%v", calls, elapsed, err)
			}
			if name == "cap" && elapsed < 900*time.Millisecond {
				t.Fatal("cap refused before its finite wait")
			}
			if name != "cap" && name != "cancel-before" {
				var pathError *os.PathError
				if !errors.Is(err, os.ErrExist) || !errors.As(err, &pathError) || pathError.Path != path {
					t.Fatal("cancellation lost the actual collision error chain")
				}
			}
			after, statErr := os.Lstat(path)
			actual, readErr := os.ReadFile(path)
			if statErr != nil || readErr != nil || !os.SameFile(before, after) || string(actual) != string(body) || before.Mode() != after.Mode() {
				t.Fatal("wait/cancellation changed or reclaimed a foreign lock")
			}
		})
	}
}

func TestNativeAdmissionCanceledAfterAcquisitionReleasesWithoutUse(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	err := withNativeAdmissionLock(ctx, root, time.Now(), func(context.Context, string) (func(), error) {
		unlock, err := acquireLock(root)
		cancel()
		return unlock, err
	}, func(time.Time) error { calls++; return nil })
	if !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatal("canceled acquired lock entered its operation", err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".activation.lock")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("canceled acquired lock was not released", err)
	}
}

func TestNativeAdmissionNeverRetriesHeldOperation(t *testing.T) {
	root := t.TempDir()
	acquisitions, calls := 0, 0
	refused := &os.PathError{Op: "open", Path: filepath.Join(root, ".activation.lock"), Err: os.ErrExist}
	err := withNativeAdmissionLock(context.Background(), root, time.Now(), func(ctx context.Context, root string) (func(), error) {
		acquisitions++
		return acquireNativeAdmissionLockContext(ctx, root)
	}, func(time.Time) error { calls++; return refused })
	if err != refused || acquisitions != 1 || calls != 1 {
		t.Fatal("post-acquisition error retried the held validation/callback operation", err, acquisitions, calls)
	}
	// An invalid managed-root object is not contention and fails immediately.
	file := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	unlock, err := acquireNativeAdmissionLockContext(context.Background(), file)
	if unlock != nil {
		unlock()
		t.Fatal("invalid root acquired a lock")
	}
	if err == nil || errors.Is(err, os.ErrExist) || time.Since(started) > 500*time.Millisecond {
		t.Fatal("non-contention error acquired a wait", err)
	}
}

func TestNativeAdmissionRetryRequiresExactLockCollision(t *testing.T) {
	root := t.TempDir()
	unlock, err := acquireLock(root)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	unexpected, collision := acquireLock(root)
	if unexpected != nil {
		unexpected()
		t.Fatal("fixture did not collide")
	}
	if !errors.Is(collision, os.ErrExist) {
		t.Fatal("fixture lacks actual lock collision", collision)
	}
	path := filepath.Join(root, ".activation.lock")
	for _, test := range []struct {
		name  string
		err   error
		retry bool
	}{
		{"actual", collision, true},
		{"wrapped", fmt.Errorf("synthetic: %w", collision), true},
		{"bare_exists", os.ErrExist, false},
		{"same_text", errors.New(collision.Error()), false},
		{"other_path", &os.PathError{Op: "open", Path: filepath.Join(root, "other"), Err: os.ErrExist}, false},
		{"other_operation", &os.PathError{Op: "stat", Path: path, Err: os.ErrExist}, false},
		{"permission", &os.PathError{Op: "open", Path: path, Err: os.ErrPermission}, false},
		{"nil", nil, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if nativeAdmissionLockCollision(root, test.err) != test.retry {
				t.Fatal("unrelated failure acquired native lock retry")
			}
		})
	}
}

func TestNativeAdmissionWaitRetainsChangedRecordPins(t *testing.T) {
	for _, name := range []string{"manifest", "client-inputs"} {
		t.Run(name, func(t *testing.T) {
			state, registry, clock := qualifiedCurrentTestSource(t)
			var active atomic.Bool
			active.Store(true)
			check := qualifiedCurrentRecheck(context.Background(), state, registry, &active)
			root := t.TempDir()
			unlock, err := acquireLock(root)
			if err != nil {
				t.Fatal(err)
			}
			go func() { time.Sleep(30 * time.Millisecond); unlock() }()
			effects := 0
			err = withNativeAdmissionLock(context.Background(), root, *clock, acquireNativeAdmissionLockContext, func(admittedAt time.Time) error {
				expected := state.Binding
				if name == "manifest" {
					expected.ManifestSHA256 = strings.Repeat("c", 64)
				} else {
					expected.ClientInputsSHA256 = strings.Repeat("c", 64)
				}
				if err := check(expected, admittedAt); err != nil {
					return err
				}
				effects++
				return nil
			})
			if err == nil || effects != 0 {
				t.Fatal("changed record pins survived waiting", effects, err)
			}
		})
	}
}

// Reuse the original signed ephemeral unit fixture and late source-policy
// checker. This produces neither an installed carrier nor a native host proof.
func TestNativeAdmissionWaitAdvancesSourceExpiryBeforeEffects(t *testing.T) {
	state, registry, clock := qualifiedCurrentTestSource(t)
	var active atomic.Bool
	active.Store(true)
	check := qualifiedCurrentRecheck(context.Background(), state, registry, &active)
	anchor := clock.Add(time.Minute - 20*time.Millisecond)
	if err := check(state.Binding, anchor); err != nil {
		t.Fatal("fixture is not fresh before waiting", err)
	}
	root := t.TempDir()
	unlock, err := acquireLock(root)
	if err != nil {
		t.Fatal(err)
	}
	go func() { time.Sleep(60 * time.Millisecond); unlock() }()
	effects := 0
	err = withNativeAdmissionLock(context.Background(), root, anchor, acquireNativeAdmissionLockContext, func(admittedAt time.Time) error {
		if !admittedAt.After(anchor.Add(20 * time.Millisecond)) {
			return fmt.Errorf("stale admission clock: %s", admittedAt)
		}
		if err := check(state.Binding, admittedAt); err != nil {
			return err
		}
		effects++
		return nil
	})
	if err == nil || effects != 0 || !strings.Contains(err.Error(), "expired") {
		t.Fatal("source expiry during wait reached effects", effects, err)
	}
}

func TestNativeAdmissionAcquisitionCapDoesNotLimitAdmittedOperation(t *testing.T) {
	root := t.TempDir()
	started := time.Now()
	err := withNativeAdmissionLock(context.Background(), root, started, acquireNativeAdmissionLockContext, func(time.Time) error { time.Sleep(1050 * time.Millisecond); return nil })
	if err != nil || time.Since(started) < time.Second {
		t.Fatal("acquisition cap became an operation deadline", err)
	}
}

// No installed-source success is constructed: after contention clears, the
// original validator must inspect the absent source and refuse before effects.
func TestQualifiedNativeAdmissionWaitsBeforeCurrentValidation(t *testing.T) {
	root := t.TempDir()
	unlock, err := acquireLock(root)
	if err != nil {
		t.Fatal(err)
	}
	released := make(chan struct{})
	go func() {
		time.Sleep(40 * time.Millisecond)
		unlock()
		close(released)
	}()
	called := 0
	started := time.Now()
	err = GuardQualifiedNativeCurrentStateContext(context.Background(), root, filepath.Join(root, "state"), ReleaseBinding{}, started, func(State, func(ReleaseBinding, time.Time) error) error { called++; return nil })
	elapsed := time.Since(started)
	<-released
	if err == nil || errors.Is(err, os.ErrExist) || errors.Is(err, context.DeadlineExceeded) || elapsed < 30*time.Millisecond || called != 0 {
		t.Fatalf("native admission did not wait then retain original source refusal: elapsed=%s called=%d err=%v", elapsed, called, err)
	}
}
