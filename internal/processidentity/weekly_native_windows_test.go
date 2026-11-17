//go:build windows

package processidentity

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

type weeklyWindowsFixture struct {
	pid, threadPID, tid uint32
	created             uint64
	changed             bool
	creationReads       int
	member              bool
	wait                uint32
	fail                string
}

func weeklyWindowsFixtureLive() *weeklyWindowsFixture {
	return &weeklyWindowsFixture{pid: 21, threadPID: 21, tid: 22, created: 133700000000000000, member: true, wait: uint32(windows.WAIT_TIMEOUT)}
}
func (f *weeklyWindowsFixture) processID(h windows.Handle) (uint32, error) {
	if h != 11 || f.fail == "pid" {
		return 0, ErrUnavailable
	}
	return f.pid, nil
}
func (f *weeklyWindowsFixture) threadIDs(h windows.Handle) (uint32, uint32, error) {
	if h != 12 || f.fail == "thread" {
		return 0, 0, ErrUnavailable
	}
	return f.threadPID, f.tid, nil
}
func (f *weeklyWindowsFixture) creation(h windows.Handle) (uint64, error) {
	f.creationReads++
	if f.fail == "creation" {
		return 0, ErrUnavailable
	}
	if f.changed && f.creationReads%2 == 0 {
		return f.created + 1, nil
	}
	return f.created, nil
}
func (f *weeklyWindowsFixture) inJob(p, j windows.Handle) (bool, error) {
	if f.fail == "job" {
		return false, ErrUnavailable
	}
	return j == 13 && f.member, nil
}
func (f *weeklyWindowsFixture) state(h windows.Handle) (uint32, error) {
	if f.fail == "wait" {
		return 0, ErrUnavailable
	}
	return f.wait, nil
}
func (f *weeklyWindowsFixture) image(h windows.Handle) (string, error) {
	if f.fail == "image" {
		return "", ErrUnavailable
	}
	return `C:\Program Files\Fixture\fixture.exe`, nil
}

func TestWeeklyWindowsActualRetainedIdentity(t *testing.T) {
	fixture := weeklyWindowsFixtureLive()
	observed, err := weeklyObserveWindows(context.Background(), fixture, 11, 12, 13, 21, 22, false)
	if err != nil || observed.PID() != 21 || observed.creationTime != fixture.created || observed.ImagePath() == "" || observed.Digest() == "" || !observed.Same(observed) {
		t.Fatalf("retained identity unavailable: %v", err)
	}
	if err := weeklyVerifyWindowsTerminal(context.Background(), fixture, 11, 12, 13, 21, 22, observed); err == nil {
		t.Fatal("live child accepted as terminal")
	}
	fixture.wait = windows.WAIT_OBJECT_0
	if err := weeklyVerifyWindowsTerminal(context.Background(), fixture, 11, 12, 13, 21, 22, observed); err != nil {
		t.Fatalf("same signaled retained object rejected: %v", err)
	}
	if _, err := weeklyObserveWindows(context.Background(), fixture, 11, 12, 13, 21, 22, false); err == nil {
		t.Fatal("signaled child accepted before input")
	}
	for _, change := range []func(*WeeklyNativeChild){func(o *WeeklyNativeChild) { o.creationTime++ }, func(o *WeeklyNativeChild) { o.processHandle++ }, func(o *WeeklyNativeChild) { o.threadHandle++ }, func(o *WeeklyNativeChild) { o.jobHandle++ }, func(o *WeeklyNativeChild) { o.threadID++ }, func(o *WeeklyNativeChild) { o.imagePath += ".other" }} {
		copied := observed
		change(&copied)
		if weeklyVerifyWindowsTerminal(context.Background(), fixture, 11, 12, 13, 21, 22, copied) == nil {
			t.Fatal("different retained terminal identity accepted")
		}
	}
	if weeklyVerifyWindowsTerminal(context.Background(), fixture, 11, 12, 13, 21, 22, WeeklyNativeChild{}) == nil {
		t.Fatal("zero observation accepted")
	}
}
func TestWeeklyWindowsIdentityRejectsMissingOrMismatchedFacts(t *testing.T) {
	cases := []struct {
		name   string
		change func(*weeklyWindowsFixture)
	}{
		{"wrong PID", func(f *weeklyWindowsFixture) { f.pid++ }},
		{"swapped thread process", func(f *weeklyWindowsFixture) { f.threadPID++ }},
		{"swapped thread ID", func(f *weeklyWindowsFixture) { f.tid++ }},
		{"zero creation", func(f *weeklyWindowsFixture) { f.created = 0 }},
		{"creation changed", func(f *weeklyWindowsFixture) { f.changed = true }},
		{"wrong job", func(f *weeklyWindowsFixture) { f.member = false }},
		{"signaled", func(f *weeklyWindowsFixture) { f.wait = windows.WAIT_OBJECT_0 }},
		{"unknown wait", func(f *weeklyWindowsFixture) { f.wait = windows.WAIT_ABANDONED }},
	}
	for _, stage := range []string{"pid", "thread", "creation", "job", "wait", "image"} {
		stage := stage
		cases = append(cases, struct {
			name   string
			change func(*weeklyWindowsFixture)
		}{"API " + stage, func(f *weeklyWindowsFixture) { f.fail = stage }})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := weeklyWindowsFixtureLive()
			tc.change(f)
			if _, err := weeklyObserveWindows(context.Background(), f, 11, 12, 13, 21, 22, false); err == nil {
				t.Fatal("unavailable identity accepted")
			}
		})
	}
	for _, handles := range [][3]windows.Handle{{0, 12, 13}, {windows.InvalidHandle, 12, 13}, {windows.Handle(^uintptr(0) - 1), 12, 13}, {14, 12, 13}, {11, 0, 13}, {11, windows.InvalidHandle, 13}, {11, 14, 13}, {11, 12, 0}, {11, 12, windows.InvalidHandle}, {11, 12, 14}} {
		if _, err := weeklyObserveWindows(context.Background(), weeklyWindowsFixtureLive(), handles[0], handles[1], handles[2], 21, 22, false); err == nil {
			t.Fatal("unrelated/invalid retained handle accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := weeklyObserveWindows(ctx, weeklyWindowsFixtureLive(), 11, 12, 13, 21, 22, false); !errors.Is(err, ErrUnavailable) {
		t.Fatal("cancelled observation accepted")
	}
}
func TestWeeklyWindowsCompleteLocalImagePath(t *testing.T) {
	good := utf16.Encode([]rune(`C:\Program Files\Fixture\fixture.exe`))
	good = append(good, 0)
	if _, err := weeklyWindowsImagePath(good, uint32(len(good)-1)); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"", `\\server\share\file.exe`, `\\?\C:\file.exe`, `\Device\file.exe`, `C:file.exe`, `C:\a\..\b.exe`, `C:\a\.\b.exe`, `C:\a\\b.exe`, `C:\a\b.exe:stream`, `C:\a\b.exe `, `C:\a\b.exe.`, "C:\\a\x00b.exe", `C:\a/b.exe`, `C:\PROGRA~1\fixture.exe`, `C:\aux\fixture.exe`, `C:\a\COM1.exe`, strings.Repeat("a", 32768)} {
		buf := append(utf16.Encode([]rune(path)), 0)
		if _, err := weeklyWindowsImagePath(buf, uint32(len(buf)-1)); err == nil {
			t.Errorf("ambiguous image path accepted: %q", path)
		}
	}
	unterminated := append([]uint16(nil), good...)
	unterminated[len(unterminated)-1] = 'x'
	if _, err := weeklyWindowsImagePath(unterminated, uint32(len(unterminated)-1)); err == nil {
		t.Fatal("nonterminated image path accepted")
	}
	if _, err := weeklyWindowsImagePath(good, uint32(len(good))); err == nil {
		t.Fatal("full/truncated buffer accepted")
	}
	if _, err := weeklyWindowsImagePath([]uint16{'C', ':', '\\', 0xD800, 0}, 4); err == nil {
		t.Fatal("malformed UTF16 accepted")
	}
}
