package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/dev/spikes/maestromcp/distribution"
)

func TestInvalidReleaseFlagsDoNotPublish(t *testing.T) {
	for _, args := range [][]string{{}, {"publish"}, {"publish", "--key", "missing", "--sequence", "0"}, {"keygen"}, {"publish", "extra"}} {
		var out, diagnostic bytes.Buffer
		if code := runWith(args, &out, &diagnostic, func(distribution.ReleaseInput) (distribution.Manifest, error) {
			t.Fatal("unexpected publish")
			return distribution.Manifest{}, nil
		}); code == 0 {
			t.Fatalf("accepted %v", args)
		}
	}
}

func TestPrivateKeyDecodeRejectsWrongLengthAndInconsistentPublicHalf(t *testing.T) {
	for _, value := range []string{"", "zz", hex.EncodeToString(make([]byte, 32)), hex.EncodeToString(make([]byte, 64))} {
		if _, err := decodePrivateKey([]byte(value)); err == nil {
			t.Fatal("accepted malformed private key")
		}
	}
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	if got, err := decodePrivateKey([]byte(hex.EncodeToString(key) + "\n")); err != nil || !bytes.Equal(got, key) {
		t.Fatal("rejected valid private key")
	}
}

func TestKeygenRequiresNewFilesAndDoesNotPrintPrivateKey(t *testing.T) {
	base := t.TempDir()
	privateFile, publicFile := filepath.Join(base, "private.key"), filepath.Join(base, "public.key")
	var out bytes.Buffer
	if err := generateKeys(privateFile, publicFile, &out); err != nil {
		t.Fatal(err)
	}
	private, _ := os.ReadFile(privateFile)
	public, _ := os.ReadFile(publicFile)
	if bytes.Contains(out.Bytes(), bytes.TrimSpace(private)) {
		t.Fatal("printed private key")
	}
	if len(bytes.TrimSpace(public)) != 64 {
		t.Fatal("wrong public key encoding")
	}
	info, _ := os.Stat(privateFile)
	if info.Mode().Perm() != 0600 {
		t.Fatalf("private mode %o", info.Mode().Perm())
	}
	before := append([]byte(nil), private...)
	if err := generateKeys(privateFile, publicFile, &out); err == nil {
		t.Fatal("overwrote keys")
	}
	after, _ := os.ReadFile(privateFile)
	if !bytes.Equal(before, after) {
		t.Fatal("private key changed")
	}
}

func TestPrivateKeyCannotBePublishedInsideFeed(t *testing.T) {
	feed := t.TempDir()
	if err := keyOutsideFeed(filepath.Join(feed, "private.hex"), feed); err == nil {
		t.Fatal("accepted private key inside feed")
	}
	if err := keyOutsideFeed(filepath.Join(filepath.Dir(feed), "private.hex"), feed); err != nil {
		t.Fatal(err)
	}
}
