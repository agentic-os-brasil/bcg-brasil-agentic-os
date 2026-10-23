package distribution

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

const maxArtifact = 128 << 20

var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var hashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type envelope struct {
	Manifest  json.RawMessage `json:"manifest"`
	Signature string          `json:"signature"`
}

func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func supported(target string) bool {
	return target == "darwin-arm64" || target == "darwin-amd64" || target == "windows-amd64"
}
func executable(name, target string) string {
	if strings.HasPrefix(target, "windows-") {
		return name + ".exe"
	}
	return name
}

// strictJSON rejects ambiguous duplicate keys as well as unknown/trailing data.
func strictJSON(b []byte, out any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	var walk func() error
	walk = func() error {
		t, e := d.Token()
		if e != nil {
			return e
		}
		if delim, ok := t.(json.Delim); ok {
			switch delim {
			case '{':
				seen := map[string]bool{}
				for d.More() {
					k, e := d.Token()
					if e != nil {
						return e
					}
					s, ok := k.(string)
					if !ok || seen[s] {
						return errors.New("duplicate or invalid JSON key")
					}
					seen[s] = true
					if e = walk(); e != nil {
						return e
					}
				}
			case '[':
				for d.More() {
					if e := walk(); e != nil {
						return e
					}
				}
			default:
				return errors.New("invalid JSON delimiter")
			}
			_, e = d.Token()
			return e
		}
		return nil
	}
	if err := walk(); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("trailing JSON")
	}
	d = json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	return d.Decode(out)
}

func regularBytes(file string, limit int64) ([]byte, error) {
	info, err := os.Lstat(file)
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("regular available file required: %s", filepath.Base(file))
	}
	if info.Size() > limit {
		return nil, errors.New("file exceeds bounded input size")
	}
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, errors.New("file changed while opening")
	}
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(b)) > limit {
		return nil, errors.New("file read exceeded bound")
	}
	return b, nil
}

func confined(root, relative string) (string, error) {
	if relative == "" || path.IsAbs(relative) || path.Clean(relative) != relative || strings.Contains(relative, "\\") {
		return "", errors.New("invalid relative path")
	}
	parts := strings.Split(relative, "/")
	current := root
	for _, part := range parts {
		if part == ".." || part == "." || part == "" {
			return "", errors.New("path escapes feed")
		}
		current = filepath.Join(current, part)
		st, err := os.Lstat(current)
		if err != nil {
			return "", errors.New("sync incomplete: referenced file is unavailable")
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("links are not permitted in release paths")
		}
	}
	return current, nil
}

func validate(m Manifest, target string) error {
	if m.SchemaVersion != 1 || m.Channel != Channel || !versionPattern.MatchString(m.Version) || m.Sequence == 0 || m.Target != target || !supported(target) || len(m.Artifacts) != 2 {
		return errors.New("unsupported experimental manifest, version or target")
	}
	for i, a := range m.Artifacts {
		name := []string{"server", "installer"}[i]
		if a.Name != name || a.Path != path.Join("versions", m.Version, executable(name, target)) || a.Size < 1 || a.Size > maxArtifact || !hashPattern.MatchString(a.SHA256) {
			return errors.New("invalid artifact contract")
		}
	}
	return nil
}

func load(root string, key ed25519.PublicKey, target string) (Manifest, []byte, map[string][]byte, error) {
	var m Manifest
	if len(key) != ed25519.PublicKeySize {
		return m, nil, nil, errors.New("independently pinned experimental public key required")
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return m, nil, nil, errors.New("feed unavailable; finish OneDrive synchronization")
	}
	b, err := regularBytes(filepath.Join(root, "feed.json"), 1<<20)
	if err != nil {
		return m, nil, nil, err
	}
	var e envelope
	if err = strictJSON(b, &e); err != nil {
		return m, nil, nil, err
	}
	sig, err := base64.StdEncoding.Strict().DecodeString(e.Signature)
	if err != nil || !ed25519.Verify(key, e.Manifest, sig) {
		return m, nil, nil, errors.New("experimental feed signature rejected")
	}
	if err = strictJSON(e.Manifest, &m); err != nil {
		return m, nil, nil, err
	}
	if err = validate(m, target); err != nil {
		return m, nil, nil, err
	}
	payload := map[string][]byte{}
	for _, a := range m.Artifacts {
		p, err := confined(root, a.Path)
		if err != nil {
			return m, nil, nil, err
		}
		data, err := regularBytes(p, maxArtifact)
		if err != nil {
			return m, nil, nil, err
		}
		if int64(len(data)) != a.Size || digest(data) != a.SHA256 {
			return m, nil, nil, errors.New("artifact incomplete or changed; active installation preserved")
		}
		payload[a.Name] = data
	}
	return m, b, payload, nil
}

func Verify(root string, key ed25519.PublicKey, target string) (Manifest, error) {
	m, _, _, err := load(root, key, target)
	return m, err
}

func writeNew(file string, b []byte, mode os.FileMode) error {
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	return errors.Join(err, f.Close())
}

func atomicFile(file string, b []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(file), ".promote-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	return os.Rename(tmp, file)
}

func Publish(in ReleaseInput) (Manifest, error) {
	m := Manifest{SchemaVersion: 1, Channel: Channel, Version: in.Version, Target: in.Target, Sequence: in.Sequence}
	if len(in.PrivateKey) != ed25519.PrivateKeySize || !versionPattern.MatchString(in.Version) || !supported(in.Target) || in.Sequence == 0 || !filepath.IsAbs(in.FeedRoot) {
		return m, errors.New("valid experimental key, version, target, absolute feed and positive sequence required")
	}
	if !bytes.Equal(ed25519.NewKeyFromSeed(in.PrivateKey[:ed25519.SeedSize]), in.PrivateKey) {
		return m, errors.New("inconsistent private key")
	}
	root, err := localHome(in.FeedRoot, true)
	if err != nil {
		return m, err
	}
	lock := filepath.Join(root, ".publish-lock")
	if err = os.Mkdir(lock, 0700); err != nil {
		return m, errors.New("publication locked; no active pointer changed")
	}
	defer os.Remove(lock)
	if _, err = os.Lstat(filepath.Join(root, "feed.json")); err == nil {
		previous, e := Verify(root, in.PrivateKey.Public().(ed25519.PublicKey), in.Target)
		if e != nil {
			return m, e
		}
		if in.Sequence <= previous.Sequence {
			return m, errors.New("publication sequence must increase")
		}
	} else if !os.IsNotExist(err) {
		return m, err
	}
	data := map[string][]byte{}
	for i, p := range []string{in.Server, in.Installer} {
		name := []string{"server", "installer"}[i]
		b, e := regularBytes(p, maxArtifact)
		if e != nil {
			return m, e
		}
		data[name] = b
		m.Artifacts = append(m.Artifacts, Artifact{Name: name, Path: path.Join("versions", in.Version, executable(name, in.Target)), SHA256: digest(b), Size: int64(len(b))})
	}
	if err = validate(m, in.Target); err != nil {
		return m, err
	}
	versions := filepath.Join(root, "versions")
	if err = os.Mkdir(versions, 0700); err != nil && !os.IsExist(err) {
		return m, err
	}
	info, err := os.Lstat(versions)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return m, errors.New("publication versions must be an ordinary directory")
	}
	dir := filepath.Join(root, "versions", in.Version)
	if err = os.Mkdir(dir, 0700); err != nil {
		return m, errors.New("release version already exists; never replace published bytes")
	}
	for _, a := range m.Artifacts {
		if err = writeNew(filepath.Join(dir, filepath.Base(a.Path)), data[a.Name], 0700); err != nil {
			return m, err
		}
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return m, err
	}
	signed, err := json.Marshal(envelope{Manifest: raw, Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(in.PrivateKey, raw))})
	if err != nil {
		return m, err
	}
	if err = writeNew(filepath.Join(dir, "release.json"), signed, 0600); err != nil {
		return m, err
	}
	if err = WriteFeedLauncher(root, m); err != nil {
		return m, err
	}
	if err = atomicFile(filepath.Join(root, "feed.json"), signed, 0600); err != nil {
		return m, err
	}
	return m, nil
}
