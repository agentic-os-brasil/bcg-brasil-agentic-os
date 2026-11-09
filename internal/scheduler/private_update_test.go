package scheduler_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

func TestPrivateAtomicUpdateSerializesAndRejectsStaleBytes(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "state.json")
	if err := scheduler.UpdatePrivateFile(path, 128, func([]byte) ([]byte, error) { return []byte("0"), nil }); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := scheduler.UpdatePrivateFile(path, 128, func(body []byte) ([]byte, error) {
				n, err := strconv.Atoi(string(body))
				if err != nil {
					return nil, err
				}
				return []byte(strconv.Itoa(n + 1)), nil
			})
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	body, _ := os.ReadFile(path)
	if string(body) != "8" {
		t.Fatalf("lost update: %s", body)
	}
	err = scheduler.UpdatePrivateFile(path, 128, func([]byte) ([]byte, error) {
		if err := os.WriteFile(path, []byte("external"), 0600); err != nil {
			return nil, err
		}
		return []byte("overwrite"), nil
	})
	if err == nil {
		t.Fatal("stale compare-and-swap published")
	}
	body, _ = os.ReadFile(path)
	if string(body) != "external" {
		t.Fatal("outside bytes lost")
	}
	before := append([]byte(nil), body...)
	err = scheduler.UpdatePrivateFile(path, 128, func([]byte) ([]byte, error) { return bytes.Repeat([]byte("x"), 129), nil })
	if err == nil {
		t.Fatal("oversized private state admitted")
	}
	body, _ = os.ReadFile(path)
	if !bytes.Equal(body, before) {
		t.Fatal("overflow changed prior state")
	}
}
func TestPrivateAtomicUpdateRejectsAliasesAndPreservesFailedMutation(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside")
	os.Mkdir(outside, 0700)
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(outside, alias); err != nil {
		t.Fatal(err)
	}
	if scheduler.UpdatePrivateFile(filepath.Join(alias, "state"), 128, func([]byte) ([]byte, error) { return []byte("bad"), nil }) == nil {
		t.Fatal("parent alias accepted")
	}
	entries, _ := os.ReadDir(outside)
	if len(entries) != 0 {
		t.Fatal("alias wrote outside")
	}
	path := filepath.Join(root, "state")
	os.WriteFile(path, []byte("original"), 0600)
	if scheduler.UpdatePrivateFile(path, 128, func([]byte) ([]byte, error) { return nil, errors.New("refused") }) == nil {
		t.Fatal("mutation error ignored")
	}
	body, _ := os.ReadFile(path)
	if string(body) != "original" {
		t.Fatal("failed mutation changed data")
	}
	leaf := filepath.Join(root, "leaf")
	os.Symlink(path, leaf)
	if scheduler.UpdatePrivateFile(leaf, 128, func([]byte) ([]byte, error) { return []byte("bad"), nil }) == nil {
		t.Fatal("leaf alias accepted")
	}
}
