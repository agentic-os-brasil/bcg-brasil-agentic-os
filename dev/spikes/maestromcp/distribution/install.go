package distribution

import (
	"bytes"
	"debug/macho"
	"debug/pe"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/dev/spikes/maestromcp/packaging"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/dev/spikes/maestromcp/spike"
)

type catalog struct {
	Name  string `json:"name"`
	Owner struct {
		Name string `json:"name"`
	} `json:"owner"`
	Plugins []catalogPlugin `json:"plugins"`
}
type catalogPlugin struct {
	Name    string `json:"name"`
	Source  string `json:"source"`
	Version string `json:"version"`
}

func contains(root, child string) bool {
	r, e := filepath.Rel(root, child)
	return e == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator))
}

func localHome(home string, create bool) (string, error) {
	user, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	developerPath := filepath.Join(user, "Developer")
	if create {
		if e := os.Mkdir(developerPath, 0700); e != nil && !os.IsExist(e) {
			return "", e
		}
	}
	developer, err := filepath.EvalSymlinks(developerPath)
	if err != nil {
		return "", errors.New("create the local Developer directory first")
	}
	home, err = filepath.Abs(home)
	if err != nil || home == developer || !contains(developer, home) || strings.Contains(home, "${") {
		return "", errors.New("installation must be a separate physical directory under Developer")
	}
	rel, _ := filepath.Rel(developer, home)
	current := developer
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		st, e := os.Lstat(current)
		if os.IsNotExist(e) && create {
			e = os.Mkdir(current, 0700)
			if e == nil {
				st, e = os.Lstat(current)
			}
		}
		if e != nil {
			return "", e
		}
		if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("installation directory cannot contain links or special files")
		}
	}
	return home, nil
}

func binaryTarget(b []byte) string {
	if m, e := macho.NewFile(bytes.NewReader(b)); e == nil && m.Type == macho.TypeExec {
		if m.Cpu == macho.CpuArm64 {
			return "darwin-arm64"
		}
		if m.Cpu == macho.CpuAmd64 {
			return "darwin-amd64"
		}
	}
	if p, e := pe.NewFile(bytes.NewReader(b)); e == nil && p.Machine == pe.IMAGE_FILE_MACHINE_AMD64 && p.Characteristics&pe.IMAGE_FILE_EXECUTABLE_IMAGE != 0 && p.Characteristics&pe.IMAGE_FILE_DLL == 0 {
		return "windows-amd64"
	}
	return ""
}

func treeHashes(root string) (map[string]string, error) {
	result := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.Type()&os.ModeSymlink != 0 {
			return errors.New("local package contains a link")
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if rel == "receipt.json" {
			return nil
		}
		b, e := regularBytes(p, maxArtifact)
		if e != nil {
			return e
		}
		result[rel] = digest(b)
		return nil
	})
	return result, err
}

func readReceipt(home, source string) (Receipt, error) {
	var r Receipt
	if !strings.HasPrefix(source, "./versions/") || !strings.HasSuffix(source, "/plugin") {
		return r, errors.New("invalid local marketplace source")
	}
	rel := strings.TrimSuffix(strings.TrimPrefix(source, "./"), "/plugin")
	dir, err := confined(home, rel)
	if err != nil {
		return r, err
	}
	b, err := regularBytes(filepath.Join(dir, "receipt.json"), 1<<20)
	if err != nil {
		return r, err
	}
	if err = strictJSON(b, &r); err != nil {
		return r, err
	}
	if r.Home != home || !versionPattern.MatchString(r.Version) || r.Sequence == 0 || r.Marketplace != home || r.StateRoot != filepath.Join(home, "State") || r.Extension != filepath.Join(dir, "Maestro.mcpb") || r.Installer != filepath.Join(dir, executable("installer", r.Target)) {
		return r, errors.New("local receipt binding mismatch")
	}
	actual, err := treeHashes(dir)
	if err != nil {
		return r, err
	}
	if !reflect.DeepEqual(actual, r.Files) {
		return r, errors.New("installed generation changed; preserve it and inspect before updating")
	}
	return r, nil
}

// Inspect reads the catalog that is the sole local activation pointer.
func Inspect(home string) (Receipt, error) {
	var r Receipt
	home, err := localHome(home, false)
	if err != nil {
		return r, err
	}
	file, err := confined(home, ".claude-plugin/marketplace.json")
	if err != nil {
		return r, err
	}
	b, err := regularBytes(file, 1<<20)
	if err != nil {
		return r, err
	}
	var c catalog
	if err = strictJSON(b, &c); err != nil {
		return r, err
	}
	if c.Name != "maestro-team-pilot-releases" || len(c.Plugins) != 1 || c.Plugins[0].Name != "maestro-team-pilot" {
		return r, errors.New("unexpected local marketplace")
	}
	r, err = readReceipt(home, c.Plugins[0].Source)
	if err == nil && c.Plugins[0].Version != r.Version {
		return r, errors.New("catalog version differs from generation")
	}
	return r, err
}

// Apply verifies a snapshot before any materialization, then promotes one local
// pointer last. Host registration/GUI are separate explicit CLI actions.
func Apply(in InstallInput) (Receipt, error) {
	var r Receipt
	m, signed, payload, err := load(in.FeedRoot, in.PublicKey, in.Target)
	if err != nil {
		return r, err
	}
	for _, name := range []string{"server", "installer"} {
		if binaryTarget(payload[name]) != in.Target {
			return r, errors.New("artifact executable does not match the signed target")
		}
	}
	feed, err := filepath.Abs(in.FeedRoot)
	if err != nil {
		return r, err
	}
	feed, err = filepath.EvalSymlinks(feed)
	if err != nil {
		return r, err
	}
	homeAbs, err := filepath.Abs(in.Home)
	if err != nil {
		return r, err
	}
	if contains(feed, homeAbs) || contains(homeAbs, feed) {
		return r, errors.New("synced feed and local installation must be separate")
	}
	home, err := localHome(in.Home, true)
	if err != nil {
		return r, err
	}
	lock := filepath.Join(home, ".update-lock")
	if err = os.Mkdir(lock, 0700); err != nil {
		return r, errors.New("another update holds the lock; active installation preserved")
	}
	defer os.Remove(lock)
	if _, e := os.Lstat(filepath.Join(home, ".claude-plugin", "marketplace.json")); e == nil {
		previous, e := Inspect(home)
		if e != nil {
			return r, e
		}
		if previous.Target != m.Target || previous.FeedRoot != feed {
			return r, errors.New("installed target/feed differs; explicit new enrollment required")
		}
		if m.Sequence < previous.Sequence {
			return r, errors.New("older publication replay rejected")
		}
		if m.Sequence == previous.Sequence {
			if previous.FeedSHA256 != digest(signed) {
				return r, errors.New("publication sequence reused with different bytes")
			}
			previous.Changed = false
			return previous, nil
		}
		if previous.Version == m.Version && previous.Files[executable("server", m.Target)] != digest(payload["server"]) {
			return r, errors.New("same version cannot change its executable")
		}
	} else if !os.IsNotExist(e) {
		return r, e
	}
	state := filepath.Join(home, "State")
	if _, e := os.Lstat(state); os.IsNotExist(e) {
		if _, err = spike.Seed(state); err != nil {
			return r, err
		}
	} else if e != nil {
		return r, e
	}
	if _, err = spike.Open(state, "case-a"); err != nil {
		return r, err
	}
	for _, dir := range []string{"versions", "staging", ".claude-plugin"} {
		p := filepath.Join(home, dir)
		if e := os.Mkdir(p, 0700); e != nil && !os.IsExist(e) {
			return r, e
		}
		st, e := os.Lstat(p)
		if e != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return r, errors.New("local update directories must be regular directories")
		}
	}
	generation := strconv.FormatUint(m.Sequence, 10) + "-" + m.Version
	destination := filepath.Join(home, "versions", generation)
	if _, e := os.Lstat(destination); e == nil {
		r, e = readReceipt(home, "./versions/"+generation+"/plugin")
		if e != nil || r.FeedSHA256 != digest(signed) {
			return r, errors.New("existing generation differs; nothing overwritten")
		}
	} else if !os.IsNotExist(e) {
		return r, e
	} else {
		stage, e := os.MkdirTemp(filepath.Join(home, "staging"), "generation-*")
		if e != nil {
			return r, e
		}
		for _, name := range []string{"server", "installer"} {
			if err = writeNew(filepath.Join(stage, executable(name, m.Target)), payload[name], 0700); err != nil {
				return r, err
			}
		}
		if err = writeNew(filepath.Join(stage, "release.json"), signed, 0600); err != nil {
			return r, err
		}
		o := packaging.Options{Server: filepath.Join(stage, executable("server", m.Target)), StateRoot: state, Workspace: "case-a", Version: m.Version, Name: "maestro-team-pilot", Destination: filepath.Join(stage, "plugin")}
		if err = packaging.BuildPlugin(o); err != nil {
			return r, err
		}
		o.Destination = filepath.Join(stage, "Maestro.mcpb")
		if err = packaging.BuildExtension(o); err != nil {
			return r, err
		}
		hashes, e := treeHashes(stage)
		if e != nil {
			return r, e
		}
		r = Receipt{Status: "LOCAL_PACKAGES_READY_HOST_ACTIVATION_PENDING", Version: m.Version, Target: m.Target, Home: home, Marketplace: home, Extension: filepath.Join(destination, "Maestro.mcpb"), Installer: filepath.Join(destination, executable("installer", m.Target)), StateRoot: state, Sequence: m.Sequence, Changed: true, FeedRoot: feed, FeedSHA256: digest(signed), Files: hashes}
		b, e := json.MarshalIndent(r, "", "  ")
		if e != nil {
			return r, e
		}
		if err = writeNew(filepath.Join(stage, "receipt.json"), b, 0600); err != nil {
			return r, err
		}
		if err = os.Rename(stage, destination); err != nil {
			return r, err
		}
	}
	c := catalog{Name: "maestro-team-pilot-releases", Plugins: []catalogPlugin{{Name: "maestro-team-pilot", Source: "./versions/" + generation + "/plugin", Version: m.Version}}}
	c.Owner.Name = "Maestro experimental team pilot"
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return r, err
	}
	if err = WriteLocalLauncher(r); err != nil {
		return r, err
	}
	if err = atomicFile(filepath.Join(home, ".claude-plugin", "marketplace.json"), b, 0600); err != nil {
		return r, err
	}
	r.Changed = true
	return r, nil
}
