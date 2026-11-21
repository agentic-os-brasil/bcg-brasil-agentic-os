//go:build windows

package docling

import (
	"bytes"
	"context"
	"errors"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
	"os"
	"path"
)

func verifyInstalled(ctx context.Context, root string, entries map[string]entry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	held, err := winprivate.OpenPinnedTree(root)
	if err != nil {
		return err
	}
	defer held.Close()
	directories := map[string]bool{".": true}
	for name := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		for dir := path.Dir(name); dir != "."; dir = path.Dir(dir) {
			directories[dir] = true
		}
	}
	count := 0
	var walk func(*winprivate.Directory, string) error
	walk = func(directory *winprivate.Directory, prefix string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		children, err := directory.ReadDir(maximumFiles)
		if err != nil {
			return err
		}
		for _, child := range children {
			if err := ctx.Err(); err != nil {
				return err
			}
			name := child.Name()
			if !relativeName(name) || path.Base(name) != name {
				return errors.New("installed_alias")
			}
			relative := name
			if prefix != "." {
				relative = prefix + "/" + name
			}
			if directories[relative] {
				sub, err := directory.Child(name, false, false)
				if err != nil {
					return err
				}
				err = walk(sub, relative)
				closeErr := sub.Close()
				if err != nil {
					return err
				}
				if closeErr != nil {
					return closeErr
				}
				continue
			}
			expected, ok := entries[relative]
			if !ok {
				return errors.New("extra_entry")
			}
			file, err := directory.OpenFile(name, os.O_RDONLY)
			if err != nil {
				return err
			}
			body, _, err := readOpenedFileContext(ctx, file, int64(len(expected.body)))
			closeErr := file.Close()
			if err != nil {
				return err
			}
			if closeErr != nil {
				return closeErr
			}
			// Windows ACL custody and authenticated logical modes replace POSIX mode bits.
			if !bytes.Equal(body, expected.body) {
				return errors.New("installed_file_changed")
			}
			count++
		}
		return nil
	}
	if err := walk(held, "."); err != nil {
		return err
	}
	if count != len(entries) {
		return errors.New("installed_file_missing")
	}
	return ctx.Err()
}
