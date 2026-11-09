package datahome

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallationPreviewSharesSelectionWithoutGrantingAuthority(t *testing.T) {
	root := t.TempDir()
	user := filepath.Join(root, "user")
	if err := os.Mkdir(user, 0700); err != nil {
		t.Fatal(err)
	}
	options := Options{UserHome: user, ProductRoot: filepath.Join(root, "future-managed"), LookupEnv: func(string) (string, bool) { return "", false }}
	preview, err := PreviewInstallation(options)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Home != filepath.Join(user, ".maestro") || preview.SelectedBy != "default" || preview.Origin != "fresh-unconfirmed" || preview.Writable {
		t.Fatalf("preview granted or changed authority: %+v", preview)
	}
	for _, path := range []string{options.ProductRoot, preview.Home, filepath.Join(user, ".maestro-home.json")} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("preview created %s: %v", path, err)
		}
	}
	if err := os.Mkdir(options.ProductRoot, 0700); err != nil {
		t.Fatal(err)
	}
	resolved, err := Resolve(options)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != preview {
		t.Fatalf("preview and actual selection diverged: %+v %+v", preview, resolved)
	}
	confirmed, err := InitializeFresh(options)
	if err != nil {
		t.Fatal(err)
	}
	options.ProductRoot = filepath.Join(root, "next-managed")
	admitted, err := PreviewInstallation(options)
	if err != nil {
		t.Fatal(err)
	}
	if admitted.Home != confirmed.Home || admitted.Origin != confirmed.Origin || !admitted.Writable {
		t.Fatalf("preview lost existing canonical authority: %+v", admitted)
	}
}
