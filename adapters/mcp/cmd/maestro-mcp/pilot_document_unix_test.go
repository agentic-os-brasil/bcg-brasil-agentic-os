//go:build maestro_experimental_installed_seed && (darwin || linux || freebsd || openbsd || netbsd || dragonfly || aix || solaris)

package main

import (
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/pilotdocument"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"testing"
)

func TestInstalledPilotDocumentRefusesSpecialLeafAndAncestorLinks(t *testing.T) {
	root := t.TempDir()
	fifo := filepath.Join(root, pilotdocument.Name)
	if err := unix.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareFixedPilotDocument(root); err == nil {
		t.Fatal("special file was followed or replaced")
	}
	if info, err := os.Lstat(fifo); err != nil || info.Mode()&os.ModeNamedPipe == 0 {
		t.Fatal("special file was changed")
	}
	real := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(real, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareFixedPilotDocument(alias); err == nil {
		t.Fatal("workspace symlink was followed")
	}
	if _, err := os.Lstat(filepath.Join(real, pilotdocument.Name)); !os.IsNotExist(err) {
		t.Fatal("ancestor-link refusal wrote a file")
	}
}
