package acquisition

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

func privateDirectory(name string, create bool) error {
	if _, err := os.Lstat(name); err != nil {
		if !create || !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err = mkdirPrivateNew(name); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
	}
	if err := scheduler.ValidatePrivateDirectory(name); err != nil {
		return err
	}
	return directoryOwner(name)
}

func validDigest(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}

func cachedInput(root string, input docling.Input) (bool, error) {
	if !validDigest(input.SHA256) || input.Size <= 0 || input.Size > 256<<20 {
		return false, errors.New("acquisition_input_invalid")
	}
	name := filepath.Join(root, "blobs", input.SHA256)
	file, err := scheduler.OpenRegularFile(name)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, errors.New("acquisition_cache_unsafe")
	}
	defer file.Close()
	if scheduler.ValidatePrivateFile(file, 0600) != nil {
		return false, errors.New("acquisition_cache_unsafe")
	}
	before, err := file.Stat()
	if err != nil {
		return false, errors.New("acquisition_cache_unsafe")
	}
	if before.Size() != input.Size || scheduler.ValidateSingleLink(file) != nil {
		return false, errors.New("acquisition_cache_changed")
	}
	hash := sha256.New()
	count, err := io.Copy(hash, io.LimitReader(file, input.Size+1))
	if err != nil {
		return false, errors.New("acquisition_cache_unreadable")
	}
	after, err := file.Stat()
	if err != nil {
		return false, errors.New("acquisition_cache_changed")
	}
	if !os.SameFile(before, after) || count != input.Size || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) || scheduler.ValidateSingleLink(file) != nil || hex.EncodeToString(hash.Sum(nil)) != input.SHA256 {
		return false, errors.New("acquisition_cache_changed")
	}
	return true, nil
}

func writeInput(name string, input docling.Input, source io.Reader) (result error) {
	if !validDigest(input.SHA256) || input.Size <= 0 || input.Size > 256<<20 {
		return errors.New("acquisition_input_invalid")
	}
	file, err := openPrivateNew(name)
	if err != nil {
		return errors.New("acquisition_partial_conflict_preserved")
	}
	defer func() { result = errors.Join(result, file.Close()) }()
	hash := sha256.New()
	count, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(source, input.Size))
	if err != nil || count != input.Size {
		return errors.New("acquisition_input_incomplete_preserved")
	}
	var extra [1]byte
	countExtra, err := io.ReadFull(source, extra[:])
	if countExtra != 0 || !errors.Is(err, io.EOF) {
		return errors.New("acquisition_input_size_mismatch_preserved")
	}
	if hex.EncodeToString(hash.Sum(nil)) != input.SHA256 {
		return errors.New("acquisition_input_hash_mismatch_preserved")
	}
	if err = file.Sync(); err != nil {
		return errors.New("acquisition_input_sync_failed")
	}
	return nil
}

func publishBlob(root string, input docling.Input, pending string) error {
	cached, err := cachedInput(root, input)
	if err != nil {
		return err
	}
	if cached {
		return nil
	}
	body, err := readBounded(pending, input.Size)
	if err != nil || int64(len(body)) != input.Size || digest(body) != input.SHA256 {
		return errors.New("acquisition_partial_changed_preserved")
	}
	if err = scheduler.WriteNewPrivateFile(filepath.Join(root, "blobs", input.SHA256), body); err != nil {
		if same, check := cachedInput(root, input); check == nil && same {
			return nil
		}
		return errors.New("acquisition_cache_conflict_preserved")
	}
	return nil
}

// cacheState only inspects this package's closed cache layout. A failed attempt
// remains preserved; completed blobs can be reused after a fresh review.
func cacheState(root string, inputs []docling.Input) ([]InputState, int, error) {
	states := make([]InputState, len(inputs))
	for n, input := range inputs {
		states[n] = InputState{Input: input}
	}
	if _, err := os.Lstat(root); errors.Is(err, os.ErrNotExist) {
		return states, 0, nil
	} else if err != nil {
		return nil, 0, errors.New("acquisition_cache_unavailable")
	}
	if err := privateDirectory(root, false); err != nil {
		return nil, 0, errors.New("acquisition_cache_unsafe")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, 0, errors.New("acquisition_cache_unavailable")
	}
	occupied := map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		allowed := name == "blobs" || name == ".acquisition.lock"
		for n := 0; n < maximumAttempts; n++ {
			if name == fmt.Sprintf("attempt-%d", n) {
				allowed = true
			}
		}
		if !allowed || entry.Type()&os.ModeSymlink != 0 {
			return nil, 0, errors.New("acquisition_cache_inventory_refused")
		}
		if name != ".acquisition.lock" {
			if err := privateDirectory(filepath.Join(root, name), false); err != nil {
				return nil, 0, errors.New("acquisition_cache_unsafe")
			}
		}
		occupied[name] = true
	}
	attempt := 0
	for ; attempt < maximumAttempts; attempt++ {
		if !occupied[fmt.Sprintf("attempt-%d", attempt)] {
			break
		}
	}
	if attempt == maximumAttempts {
		return nil, 0, errors.New("acquisition_attempt_limit_preserved")
	}
	if occupied["blobs"] {
		for n, input := range inputs {
			cached, err := cachedInput(root, input)
			if err != nil {
				return nil, 0, err
			}
			states[n].Cached = cached
		}
	}
	return states, attempt, nil
}
