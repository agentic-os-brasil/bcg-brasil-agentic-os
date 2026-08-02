// Package macosadapter owns the filesystem-only LaunchAgent contract. It does
// not call launchctl; native qualification is a separate attended probe.
package macosadapter

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var labelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{0,127}$`)
var ErrOptInRequired = errors.New("LaunchAgent installation requires explicit Canary opt-in")

type Spec struct {
	Label           string
	Program         string
	Arguments       []string
	StartInterval   int
	RunAtLoad       bool
	Disabled        bool
	StandardOutPath string
	StandardErrPath string
}

type Status struct {
	State    string `json:"state"`
	Path     string `json:"path"`
	Label    string `json:"label"`
	Disabled bool   `json:"disabled"`
}

func UserLaunchAgentsPath(home string) (string, error) {
	if strings.TrimSpace(home) == "" || !filepath.IsAbs(home) {
		return "", errors.New("absolute user home is required")
	}
	canonical, err := canonicalLaunchAgentHome(home)
	if err != nil {
		return "", err
	}
	return filepath.Join(canonical, "Library", "LaunchAgents"), nil
}

func Render(spec Spec) ([]byte, error) {
	if err := validateSpec(spec); err != nil {
		return nil, err
	}
	var body bytes.Buffer
	body.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<plist version=\"1.0\"><dict>\n")
	writeString(&body, "Label", spec.Label)
	writeBool(&body, "Disabled", spec.Disabled)
	body.WriteString("<key>ProgramArguments</key><array>\n")
	writeStringValue(&body, spec.Program)
	for _, argument := range spec.Arguments {
		writeStringValue(&body, argument)
	}
	body.WriteString("</array>\n")
	writeBool(&body, "RunAtLoad", spec.RunAtLoad)
	body.WriteString("<key>StartInterval</key><integer>" + strconv.Itoa(spec.StartInterval) + "</integer>\n")
	if spec.StandardOutPath != "" {
		writeString(&body, "StandardOutPath", spec.StandardOutPath)
	}
	if spec.StandardErrPath != "" {
		writeString(&body, "StandardErrorPath", spec.StandardErrPath)
	}
	body.WriteString("</dict></plist>\n")
	if err := Parse(body.Bytes()); err != nil {
		return nil, err
	}
	return body.Bytes(), nil
}

// ResolveExecutable returns the exact physical Darwin executable that may be
// embedded in ProgramArguments. A caller may not smuggle a mutable symlink or
// a non-executable fixture into a per-user scheduler definition.
func ResolveExecutable(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, `\`) || !pathpkg.IsAbs(value) || strings.ContainsRune(value, '\x00') {
		return "", errors.New("absolute Darwin executable path is required")
	}
	info, err := os.Lstat(value)
	if err != nil {
		return "", fmt.Errorf("inspect LaunchAgent executable: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("LaunchAgent executable must not be a symlink")
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return "", errors.New("LaunchAgent executable must be a regular executable file")
	}
	physical, err := filepath.EvalSymlinks(value)
	if err != nil {
		return "", fmt.Errorf("resolve LaunchAgent executable: %w", err)
	}
	physical, err = filepath.Abs(filepath.Clean(physical))
	if err != nil {
		return "", err
	}
	if strings.Contains(physical, `\`) || !pathpkg.IsAbs(physical) {
		return "", errors.New("resolved LaunchAgent executable is not a Darwin path")
	}
	return physical, nil
}

func Parse(body []byte) error {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	keys := map[string]bool{}
	root := false
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("invalid LaunchAgent plist: %w", err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			if value.Name.Local == "plist" {
				root = true
			}
			if value.Name.Local == "key" {
				var key string
				if err := decoder.DecodeElement(&key, &value); err != nil {
					return err
				}
				keys[key] = true
			}
		}
	}
	if !root || !keys["Label"] || !keys["ProgramArguments"] || !keys["StartInterval"] {
		return errors.New("LaunchAgent plist is missing required keys")
	}
	return nil
}

// launchAgentProgram reads only the bounded identity field used to bind a
// native launchctl record to the managed plist. It deliberately does not
// expose arbitrary plist values to the lifecycle layer.
func launchAgentProgram(home, label string) (string, error) {
	status, err := ReadStatus(home, label)
	if err != nil {
		return "", err
	}
	if status.State == "not_installed" {
		return "", os.ErrNotExist
	}
	info, err := os.Lstat(status.Path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", errors.New("LaunchAgent plist must be a regular file")
	}
	body, err := readLaunchAgentBody(status.Path)
	if err != nil {
		return "", err
	}
	return parseProgramArgumentIdentity(body)
}

func parseProgramArgumentIdentity(body []byte) (string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	currentKey := ""
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", errors.New("LaunchAgent identity plist is invalid")
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "key":
			var key string
			if err := decoder.DecodeElement(&key, &start); err != nil {
				return "", errors.New("LaunchAgent identity key is invalid")
			}
			currentKey = key
		case "array":
			if currentKey != "ProgramArguments" {
				continue
			}
			var first string
			stringCount := 0
			for {
				arrayToken, arrayErr := decoder.Token()
				if arrayErr != nil {
					return "", errors.New("LaunchAgent ProgramArguments array is invalid")
				}
				switch value := arrayToken.(type) {
				case xml.StartElement:
					if value.Name.Local != "string" {
						return "", errors.New("LaunchAgent ProgramArguments identity is invalid")
					}
					var argument string
					if err := decoder.DecodeElement(&argument, &value); err != nil {
						return "", errors.New("LaunchAgent ProgramArguments identity is invalid")
					}
					stringCount++
					if stringCount == 1 {
						first = argument
					}
				case xml.EndElement:
					if value.Name.Local == "array" {
						if first == "" || strings.Contains(first, `\`) || !pathpkg.IsAbs(first) {
							return "", errors.New("LaunchAgent ProgramArguments identity is not a Darwin path")
						}
						return first, nil
					}
				}
			}
		}
	}
	return "", errors.New("LaunchAgent plist has no ProgramArguments identity")
}

func validateSpec(spec Spec) error {
	// ProgramArguments are a Darwin contract and therefore use POSIX path
	// semantics even when this package is compiled by a Windows CI worker.
	if !labelPattern.MatchString(spec.Label) || strings.Contains(spec.Program, `\`) || !pathpkg.IsAbs(spec.Program) || strings.Contains(spec.Program, "\x00") || spec.StartInterval <= 0 || spec.StartInterval > 86400 {
		return errors.New("invalid LaunchAgent identity or interval")
	}
	values := append(append([]string{spec.Program}, spec.Arguments...), spec.StandardOutPath, spec.StandardErrPath)
	for _, value := range values {
		if strings.Contains(value, "\x00") || strings.Contains(value, "{{") || strings.Contains(value, "}}") {
			return errors.New("LaunchAgent values must be concrete and interpolation-free")
		}
	}
	for _, value := range []string{spec.StandardOutPath, spec.StandardErrPath} {
		// Diagnostic paths point at the host fixture filesystem in adapter
		// tests, while native Darwin paths remain POSIX absolute paths. Raw
		// Windows separators are never normalized into a Darwin plist.
		if strings.Contains(value, `\`) {
			return errors.New("LaunchAgent diagnostic paths must use Darwin POSIX separators")
		}
		if value != "" && !filepath.IsAbs(value) && !pathpkg.IsAbs(value) {
			return errors.New("LaunchAgent diagnostics paths must be absolute")
		}
	}
	return nil
}

func writeString(buffer *bytes.Buffer, key, value string) {
	buffer.WriteString("<key>" + key + "</key>")
	writeStringValue(buffer, value)
}
func writeStringValue(buffer *bytes.Buffer, value string) {
	buffer.WriteString("<string>")
	_ = xml.EscapeText(buffer, []byte(value))
	buffer.WriteString("</string>\n")
}
func writeBool(buffer *bytes.Buffer, key string, value bool) {
	state := "false"
	if value {
		state = "true"
	}
	buffer.WriteString("<key>" + key + "</key><" + state + "/>\n")
}

func Install(home string, spec Spec, optIn bool) (Status, error) {
	if !optIn {
		return Status{}, ErrOptInRequired
	}
	directory, err := UserLaunchAgentsPath(home)
	if err != nil {
		return Status{}, err
	}
	body, err := Render(spec)
	if err != nil {
		return Status{}, err
	}
	if err := rejectLaunchAgentSymlinkAncestors(directory); err != nil {
		return Status{}, err
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return Status{}, err
	}
	for _, path := range []string{home, filepath.Dir(directory), directory} {
		info, statErr := os.Lstat(path)
		if statErr != nil {
			return Status{}, statErr
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return Status{}, errors.New("LaunchAgent path contains a symlink or non-directory")
		}
	}
	path := filepath.Join(directory, spec.Label+".plist")
	if info, statErr := os.Lstat(path); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return Status{}, errors.New("LaunchAgent plist must be a regular file")
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return Status{}, statErr
	}
	temporary, err := os.CreateTemp(directory, "."+spec.Label+".plist.tmp-")
	if err != nil {
		return Status{}, err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err == nil {
		_, err = temporary.Write(body)
	}
	if err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return Status{}, err
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return Status{}, err
	}
	return Status{State: "adapter_installed_native_qualification_pending", Path: path, Label: spec.Label, Disabled: spec.Disabled}, nil
}

// Verify compares the installed plist with the deterministic expected bytes.
// This binds program, workspace argument, schedule and diagnostics together;
// a merely parseable plist is not sufficient evidence of managed identity.
func Verify(home string, expected Spec) (Status, error) {
	want, err := Render(expected)
	if err != nil {
		return Status{}, err
	}
	status, err := ReadStatus(home, expected.Label)
	if err != nil {
		return Status{}, err
	}
	if status.State == "not_installed" {
		return status, os.ErrNotExist
	}
	body, err := readLaunchAgentBody(status.Path)
	if err != nil {
		return Status{}, err
	}
	if !bytes.Equal(body, want) {
		return status, errors.New("LaunchAgent binding does not match the exact managed specification")
	}
	return status, nil
}

func ReadStatus(home, label string) (Status, error) {
	directory, err := UserLaunchAgentsPath(home)
	if err != nil {
		return Status{}, err
	}
	if !labelPattern.MatchString(label) {
		return Status{}, errors.New("invalid LaunchAgent label")
	}
	path := filepath.Join(directory, label+".plist")
	if err := rejectLaunchAgentSymlinkAncestors(directory); err != nil {
		return Status{}, err
	}
	if info, statErr := os.Lstat(path); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return Status{}, errors.New("LaunchAgent plist must be a regular file")
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return Status{}, statErr
	}
	body, err := readLaunchAgentBody(path)
	if errors.Is(err, os.ErrNotExist) {
		return Status{State: "not_installed", Path: path, Label: label}, nil
	}
	if err != nil {
		return Status{}, err
	}
	if err := Parse(body); err != nil {
		return Status{}, err
	}
	return Status{State: "adapter_installed_native_qualification_pending", Path: path, Label: label, Disabled: bytes.Contains(body, []byte("<key>Disabled</key><true/>"))}, nil
}

func canonicalLaunchAgentHome(home string) (string, error) {
	absolute, err := filepath.Abs(filepath.Clean(home))
	if err != nil {
		return "", err
	}
	temporary, err := filepath.Abs(filepath.Clean(os.TempDir()))
	if err != nil {
		return "", err
	}
	physicalTemporary, err := filepath.EvalSymlinks(temporary)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(temporary, absolute)
	if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return filepath.Join(physicalTemporary, relative), nil
	}
	return absolute, nil
}

func rejectLaunchAgentSymlinkAncestors(path string) error {
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return err
	}
	volume := filepath.VolumeName(absolute)
	remainder := strings.TrimPrefix(absolute, volume)
	remainder = strings.TrimPrefix(remainder, string(filepath.Separator))
	current := volume + string(filepath.Separator)
	for _, component := range strings.Split(remainder, string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("LaunchAgent path cannot traverse symlinked ancestors")
		}
	}
	return nil
}

func Pause(home, label string) (Status, error)  { return setDisabled(home, label, true) }
func Resume(home, label string) (Status, error) { return setDisabled(home, label, false) }

func Uninstall(home, label string) error {
	status, err := ReadStatus(home, label)
	if err != nil {
		return err
	}
	if status.State == "not_installed" {
		return nil
	}
	info, err := os.Lstat(status.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("LaunchAgent plist must be a regular file")
	}
	return os.Remove(status.Path)
}

func setDisabled(home, label string, disabled bool) (Status, error) {
	status, err := ReadStatus(home, label)
	if err != nil {
		return Status{}, err
	}
	if status.State == "not_installed" {
		return status, nil
	}
	directory := filepath.Dir(status.Path)
	path := status.Path
	body, err := os.ReadFile(path)
	if err != nil {
		return Status{}, err
	}
	if err := Parse(body); err != nil {
		return Status{}, err
	}
	old, next := []byte("<key>Disabled</key><true/>"), []byte("<key>Disabled</key><false/>")
	if !disabled {
		body = bytes.Replace(body, old, next, 1)
	} else if bytes.Contains(body, next) {
		body = bytes.Replace(body, next, old, 1)
	} else {
		return Status{}, errors.New("LaunchAgent plist has no Disabled key")
	}
	temporary, err := os.CreateTemp(directory, "."+label+".pause.tmp-")
	if err != nil {
		return Status{}, err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err == nil {
		_, err = temporary.Write(body)
	}
	if err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return Status{}, err
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return Status{}, err
	}
	return ReadStatus(home, label)
}
