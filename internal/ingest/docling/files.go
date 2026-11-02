package docling

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"golang.org/x/text/unicode/norm"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

func digest(body []byte) string { s := sha256.Sum256(body); return hex.EncodeToString(s[:]) }
func relativeName(name string) bool {
	if name == "" || name == "." || path.Clean(name) != name || strings.HasPrefix(name, "../") || strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\:\x00") || !utf8.ValidString(name) || !norm.NFC.IsNormalString(name) {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return false
		}
	}
	for _, part := range strings.Split(name, "/") {
		if strings.TrimRight(part, ". ") != part {
			return false
		}
	}
	return true
}

func readFile(name string, maximum int64) ([]byte, os.FileInfo, error) {
	f, err := scheduler.OpenRegularFile(name)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	return readOpenedFile(f, maximum)
}

func readOpenedFile(f *os.File, maximum int64) ([]byte, os.FileInfo, error) {
	before, err := f.Stat()
	if err != nil {
		return nil, nil, err
	}
	if !before.Mode().IsRegular() || !singleLink(before) || before.Size() < 0 || before.Size() > maximum {
		return nil, nil, errors.New("file_not_bounded_single_link")
	}
	body, err := io.ReadAll(io.LimitReader(f, maximum+1))
	if err != nil {
		return nil, nil, err
	}
	after, err := f.Stat()
	if err != nil {
		return nil, nil, err
	}
	if !os.SameFile(before, after) || !after.Mode().IsRegular() || int64(len(body)) > maximum || before.Size() != int64(len(body)) || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) || !singleLink(after) {
		return nil, nil, errors.New("file_changed")
	}
	return body, after, nil
}
func decode(body []byte, target any) error {
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return err
	}
	var tail any
	if err := d.Decode(&tail); !errors.Is(err, io.EOF) {
		return errors.New("trailing_json")
	}
	return nil
}
func writeExclusive(name string, body []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(body)
	if err == nil {
		err = f.Chmod(mode)
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
