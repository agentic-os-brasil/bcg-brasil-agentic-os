package distribution

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const macLauncherHeader = "#!/bin/sh\n# Maestro synthetic team pilot: generated launcher v1\n"
const windowsLauncherHeader = "@echo off\r\nrem Maestro synthetic team pilot: generated launcher v1\r\n"

// WriteFeedLauncher offers one attended bootstrap entry point per native feed.
// The initial executable still requires independent source verification and
// ordinary OS acceptance; this launcher does not attest either one.
func WriteFeedLauncher(feedRoot string, m Manifest) error {
	if !filepath.IsAbs(feedRoot) || !supported(m.Target) || !versionPattern.MatchString(m.Version) {
		return errors.New("launcher requires absolute feed and supported release")
	}
	if strings.HasPrefix(m.Target, "darwin-") {
		body := macLauncherHeader + "set -eu\n" +
			"SCRIPT_DIR=$(CDPATH= cd -- \"$(dirname -- \"$0\")\" && pwd -P)\n" +
			"exec \"$SCRIPT_DIR/versions/" + m.Version + "/installer\" install --feed \"$SCRIPT_DIR\" --activate-code --open-desktop\n"
		return writeOwnedLauncher(filepath.Join(feedRoot, "Instalar Maestro.command"), []byte(body), []byte(macLauncherHeader))
	}
	body := windowsLauncherHeader + "setlocal DisableDelayedExpansion\r\n" +
		`"%~dp0versions\` + m.Version + `\installer.exe" install --feed "%~dp0." --activate-code --open-desktop` + "\r\n" +
		"exit /b %errorlevel%\r\n"
	return writeOwnedLauncher(filepath.Join(feedRoot, "Instalar Maestro.cmd"), []byte(body), []byte(windowsLauncherHeader))
}

// WriteLocalLauncher keeps update paths explicit and refuses to replace an
// unrelated pre-existing file. It invokes the verified installed executable.
func WriteLocalLauncher(r Receipt) error {
	if !filepath.IsAbs(r.Home) || !supported(r.Target) || r.Installer == "" || r.FeedRoot == "" {
		return errors.New("local launcher requires bound installation receipt")
	}
	if strings.HasPrefix(r.Target, "darwin-") {
		body := macLauncherHeader + "set -eu\nexec " + shellQuote(r.Installer) + " update --feed " + shellQuote(r.FeedRoot) + " --home " + shellQuote(r.Home) + " --activate-code --open-desktop\n"
		return writeOwnedLauncher(filepath.Join(r.Home, "Atualizar Maestro.command"), []byte(body), []byte(macLauncherHeader))
	}
	installer, err := batchQuote(r.Installer)
	if err != nil {
		return err
	}
	feed, err := batchQuote(r.FeedRoot)
	if err != nil {
		return err
	}
	home, err := batchQuote(r.Home)
	if err != nil {
		return err
	}
	body := windowsLauncherHeader + "setlocal DisableDelayedExpansion\r\n" + installer + " update --feed " + feed + " --home " + home + " --activate-code --open-desktop\r\nexit /b %errorlevel%\r\n"
	return writeOwnedLauncher(filepath.Join(r.Home, "Atualizar Maestro.cmd"), []byte(body), []byte(windowsLauncherHeader))
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }

func batchQuote(value string) (string, error) {
	if strings.ContainsAny(value, "\"\r\n\x00") {
		return "", errors.New("Windows launcher paths cannot contain quotes, newlines or NUL")
	}
	return `"` + strings.ReplaceAll(value, "%", "%%") + `"`, nil
}

func writeOwnedLauncher(file string, body, header []byte) error {
	info, err := os.Lstat(file)
	if os.IsNotExist(err) {
		return writeNew(file, body, 0700)
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("launcher destination must be a regular generated file")
	}
	previous, err := regularBytes(file, 64<<10)
	if err != nil {
		return err
	}
	if !bytes.HasPrefix(previous, header) {
		return fmt.Errorf("existing launcher is not owned by this pilot: %s", filepath.Base(file))
	}
	return atomicFile(file, body, 0700)
}
