package distribution

import (
	"bytes"
	"crypto/ed25519"
	"debug/macho"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Header fixtures exercise packaging contracts only; the separate real-binary
// rehearsal is required to qualify native subprocess behavior.
func installFixture(t *testing.T) (ed25519.PublicKey, ReleaseInput, InstallInput) {
	t.Helper()
	root, pub, in := releaseFixture(t)
	var b bytes.Buffer
	for _, v := range []uint32{macho.Magic64, uint32(macho.CpuArm64), 0, uint32(macho.TypeExec), 0, 0, 0, 0} {
		_ = binary.Write(&b, binary.LittleEndian, v)
	}
	for _, p := range []string{in.Server, in.Installer} {
		if err := os.WriteFile(p, b.Bytes(), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Publish(in); err != nil {
		t.Fatal(err)
	}
	return pub, in, InstallInput{FeedRoot: in.FeedRoot, Home: filepath.Join(root, "Local participant"), Target: in.Target, PublicKey: pub}
}

func TestInstallUpdateAndReplayPreserveState(t *testing.T) {
	_, pub, in := installFixture(t)
	r, err := Apply(in)
	if err != nil {
		t.Fatal(err)
	}
	before, err := treeHashes(r.StateRoot)
	if err != nil {
		t.Fatal(err)
	}
	noop, err := Apply(in)
	if err != nil || noop.Changed {
		t.Fatalf("repeat install: %+v %v", noop, err)
	}
	old, _ := os.ReadFile(filepath.Join(pub.FeedRoot, "feed.json"))
	pub.Version = "0.0.4"
	pub.Sequence = 2
	if _, err = Publish(pub); err != nil {
		t.Fatal(err)
	}
	updated, err := Apply(in)
	if err != nil || updated.Version != "0.0.4" || !updated.Changed {
		t.Fatalf("update: %+v %v", updated, err)
	}
	after, err := treeHashes(r.StateRoot)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("state changed on update")
	}
	if _, err = os.Stat(r.Extension); err != nil {
		t.Fatal("old package deleted")
	}
	if err = os.WriteFile(filepath.Join(pub.FeedRoot, "feed.json"), old, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = Apply(in); err == nil {
		t.Fatal("old signed pointer replay accepted")
	}
	active, err := Inspect(in.Home)
	if err != nil || active.Version != "0.0.4" {
		t.Fatal("replay changed active version")
	}
}

func TestDamagedUpdateAndLocalEditDoNotPromote(t *testing.T) {
	_, pub, in := installFixture(t)
	r, err := Apply(in)
	if err != nil {
		t.Fatal(err)
	}
	pub.Version = "0.0.4"
	pub.Sequence = 2
	m, err := Publish(pub)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(pub.FeedRoot, m.Artifacts[0].Path)
	if err = os.WriteFile(p, []byte("partial"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = Apply(in); err == nil {
		t.Fatal("damaged update accepted")
	}
	active, err := Inspect(in.Home)
	if err != nil || active.Version != r.Version {
		t.Fatal("damage promoted")
	}
	b, err := os.ReadFile(pub.Server)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(p, b, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(filepath.Dir(r.Extension), "plugin", "skills", "operator", "SKILL.md"), []byte("owner edit"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = Apply(in); err == nil {
		t.Fatal("owner modification overwritten")
	}
}

func TestSignedTraversalUnknownKeysAndLinksRejected(t *testing.T) {
	_, key, in := releaseFixture(t)
	m, err := Publish(in)
	if err != nil {
		t.Fatal(err)
	}
	original, _ := os.ReadFile(filepath.Join(in.FeedRoot, "feed.json"))
	var env envelope
	_ = json.Unmarshal(original, &env)
	m.Artifacts[0].Path = "../outside"
	raw, _ := json.Marshal(m)
	env.Manifest = raw
	env.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(in.PrivateKey, raw))
	b, _ := json.Marshal(env)
	_ = os.WriteFile(filepath.Join(in.FeedRoot, "feed.json"), b, 0600)
	if _, err = Verify(in.FeedRoot, key, in.Target); err == nil {
		t.Fatal("signed traversal accepted")
	}
	_ = os.WriteFile(filepath.Join(in.FeedRoot, "feed.json"), original, 0600)
	file := filepath.Join(in.FeedRoot, "versions", in.Version, "server")
	if err = os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(in.Server, file); err != nil {
		t.Skip(err)
	}
	if _, err = Verify(in.FeedRoot, key, in.Target); err == nil {
		t.Fatal("artifact link accepted")
	}
	var got Manifest
	if err = strictJSON([]byte(`{"schema_version":1,"schema_version":2}`), &got); err == nil {
		t.Fatal("duplicate key accepted")
	}
	if err = strictJSON([]byte(`{"unknown":true}`), &got); err == nil {
		t.Fatal("unknown key accepted")
	}
}
