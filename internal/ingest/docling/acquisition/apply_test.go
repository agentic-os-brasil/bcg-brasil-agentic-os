package acquisition

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestAcquireRejectsChangedAndExpiredReviewBeforeEffects(t *testing.T) {
	f := fixture(t)
	p, err := Review(context.Background(), f.options)
	if err != nil {
		t.Fatal(err)
	}
	p.SHA256 = "changed"
	if _, err = Acquire(context.Background(), f.options, p); err == nil {
		t.Fatal("changed review accepted")
	}
	if _, err = os.Lstat(f.options.CacheRoot); !os.IsNotExist(err) {
		t.Fatal("stale review created cache")
	}
	now := time.Now().UTC().Truncate(time.Second)
	if _, err = reviewAt(context.Background(), f.options, now.Add(-31*time.Minute), now); err == nil {
		t.Fatal("expired review accepted")
	}
}
func TestAcquisitionRefusesChangedCarrierAndUnsignedRecipe(t *testing.T) {
	for _, which := range []string{"server", "base", "recipe", "signature", "extra", "archive occupied"} {
		t.Run(which, func(t *testing.T) {
			f := fixture(t)
			switch which {
			case "server":
				writeFixture(t, f.options.ServerPath, []byte("changed"))
			case "base":
				writeFixture(t, filepath.Join(f.options.ReleaseDirectory, "base_0.1.0.tar.gz"), []byte("changed"))
			case "recipe":
				writeFixture(t, filepath.Join(f.options.RecipeDirectory, RecipeName), []byte("{}"))
			case "signature":
				writeFixture(t, filepath.Join(f.options.RecipeDirectory, RecipeSignatureName), make([]byte, 64))
			case "extra":
				writeFixture(t, filepath.Join(f.options.ReleaseDirectory, "extra"), []byte("extra"))
			case "archive occupied":
				writeFixture(t, filepath.Join(f.options.ReleaseDirectory, f.recipe.RuntimeArtifact.Name), []byte("occupied"))
			}
			if _, err := Review(context.Background(), f.options); err == nil {
				t.Fatal("changed carrier accepted")
			}
			if _, err := os.Lstat(f.options.CacheRoot); !os.IsNotExist(err) {
				t.Fatal("review created cache")
			}
		})
	}
}

func TestNativePreparationCannotRunOnAnUnqualifiedHost(t *testing.T) {
	if runtime.GOOS == "windows" && runtime.GOARCH == "amd64" {
		t.Skip("real Windows acquisition requires the explicit reviewed native qualification journey")
	}
	f := fixture(t)
	p, err := Review(context.Background(), f.options)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Acquire(context.Background(), f.options, p); err == nil || err.Error() != "acquisition_native_windows_required" {
		t.Fatalf("non-native acquisition not refused: %v", err)
	}
	if _, err = os.Lstat(f.options.CacheRoot); !os.IsNotExist(err) {
		t.Fatal("host refusal created cache")
	}
}
