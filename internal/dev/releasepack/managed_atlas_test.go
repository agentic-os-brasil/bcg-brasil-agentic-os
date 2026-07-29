package releasepack

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestBaseDistributionAllowlistIncludesEveryManagedAtlasFile(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", ".."))
	allowlist, err := LoadAllowlist(filepath.Join(root, "bundles", "base", "distribution.json"))
	if err != nil {
		t.Fatal(err)
	}
	allowed := make(map[string]bool, len(allowlist.Files))
	for _, entry := range allowlist.Files {
		allowed[entry.Source] = true
	}
	atlasRoot := filepath.Join(root, "bundles", "base", "atlas", "managed")
	err = filepath.WalkDir(atlasRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if !allowed[filepath.ToSlash(relative)] {
			t.Errorf("managed atlas file is missing from distribution allowlist: %s", relative)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
