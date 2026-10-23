package distribution

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
)

func releaseFixture(t *testing.T) (string, ed25519.PublicKey, ReleaseInput) {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	server, installer := filepath.Join(root, "server"), filepath.Join(root, "installer")
	if err := os.WriteFile(server, []byte("synthetic server input"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(installer, []byte("synthetic installer input"), 0700); err != nil {
		t.Fatal(err)
	}
	return root, pub, ReleaseInput{FeedRoot: filepath.Join(root, "Feed with spaces"), Version: "0.0.3", Target: "darwin-arm64", Server: server, Installer: installer, Sequence: 1, PrivateKey: key}
}

func TestSignedPublicationAndIncompleteSync(t *testing.T) {
	_, pub, in := releaseFixture(t)
	m, err := Publish(in)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Verify(in.FeedRoot, pub, in.Target)
	if err != nil || got.Version != m.Version || got.Sequence != 1 {
		t.Fatalf("verified publication: %+v %v", got, err)
	}
	if err := os.WriteFile(filepath.Join(in.FeedRoot, m.Artifacts[0].Path), []byte("incomplete sync"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(in.FeedRoot, pub, in.Target); err == nil {
		t.Fatal("partial/tampered payload accepted")
	}
}

func TestFeedRejectsWrongKeyTargetAndSameVersionMutation(t *testing.T) {
	_, pub, in := releaseFixture(t)
	if _, err := Publish(in); err != nil {
		t.Fatal(err)
	}
	other, _, _ := ed25519.GenerateKey(rand.Reader)
	if _, err := Verify(in.FeedRoot, other, in.Target); err == nil {
		t.Fatal("untrusted key accepted")
	}
	if _, err := Verify(in.FeedRoot, pub, "windows-amd64"); err == nil {
		t.Fatal("wrong target accepted")
	}
	if err := os.WriteFile(in.Server, []byte("changed server"), 0700); err != nil {
		t.Fatal(err)
	}
	in.Sequence = 2
	if _, err := Publish(in); err == nil {
		t.Fatal("same version replaced")
	}
}

func TestFailedPublicationPreservesActiveFeed(t *testing.T) {
	_, pub, in := releaseFixture(t)
	if _, err := Publish(in); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(in.FeedRoot, "feed.json"))
	in.Version = "0.0.4"
	in.Sequence = 2
	in.Server = filepath.Join(in.FeedRoot, "missing")
	if _, err := Publish(in); err == nil {
		t.Fatal("missing artifact published")
	}
	after, _ := os.ReadFile(filepath.Join(in.FeedRoot, "feed.json"))
	if string(before) != string(after) {
		t.Fatal("active pointer changed on failed publication")
	}
	if _, err := Verify(in.FeedRoot, pub, in.Target); err != nil {
		t.Fatal(err)
	}
}

func TestPublicationRejectsLinkedVersionDirectory(t *testing.T) {
	root, _, in := releaseFixture(t)
	if err := os.Mkdir(in.FeedRoot, 0700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside")
	if err := os.Mkdir(outside, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(in.FeedRoot, "versions")); err != nil {
		t.Skip(err)
	}
	if _, err := Publish(in); err == nil {
		t.Fatal("publisher followed linked versions directory")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatal("publisher wrote through link")
	}
	if _, err = os.Lstat(filepath.Join(in.FeedRoot, "feed.json")); !os.IsNotExist(err) {
		t.Fatal("publisher promoted linked payload")
	}
}
