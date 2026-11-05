//go:build windows

package datahome

import (
	"encoding/json"
	"errors"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
	"os"
	"path/filepath"
)

func initializeFresh(options Options, result Result) error {
	user, e := winprivate.Open(options.UserHome, false)
	if e != nil {
		return e
	}
	defer user.Close()
	locator, e := user.OpenFile(".maestro-home.json", os.O_RDONLY)
	if e == nil {
		locator.Close()
		return errors.New("locator appeared during bootstrap")
	}
	if !errors.Is(e, os.ErrNotExist) {
		return e
	}
	home, e := winprivate.Fresh(result.Home)
	if e != nil {
		return e
	}
	defer home.Close()
	entries, e := home.ReadDir(1)
	if e != nil {
		return e
	}
	if len(entries) != 0 {
		return errors.New("home is no longer empty")
	}
	brain, e := home.Child("brain", true, true)
	if e != nil {
		return e
	}
	defer brain.Close()
	metadata, e := brain.Child(".maestro", true, true)
	if e != nil {
		return e
	}
	defer metadata.Close()
	if !samePath(result.Home, filepath.Join(options.UserHome, ".maestro")) {
		b, _ := json.Marshal(map[string]any{"schema_version": 1, "home": result.Home, "attempt_id": ""})
		if e = user.Publish(".maestro-home.json", b, true); e != nil {
			return e
		}
	}
	b, _ := json.Marshal(map[string]any{"schema_version": 1, "home": result.Home, "origin": "fresh-confirmed", "attempt_id": "", "plan_sha256": ""})
	return metadata.Publish("origin.json", b, true)
}
