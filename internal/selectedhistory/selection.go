// Package selectedhistory reads only an explicitly selected, metadata-bound
// batch. It neither discovers history nor retains source bodies.
package selectedhistory

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

const MaxFileBytes = 2 << 20
const MaxTotalBytes = 8 << 20

var ErrSelection = errors.New("selected history unavailable: verify exact scope, limits and unchanged metadata")

type Session struct {
	Path     string `json:"path"`
	ID       string `json:"id"`
	Identity string `json:"identity,omitempty"`
	Bytes    int64  `json:"bytes,omitempty"`
}
type Source struct {
	Kind      string    `json:"kind"`
	Workspace string    `json:"workspace"`
	Root      string    `json:"root"`
	Sessions  []Session `json:"sessions"`
}
type Manifest struct {
	Workspaces []string `json:"workspaces"`
	Sources    []Source `json:"sources"`
}
type Plan struct {
	SchemaVersion int      `json:"schema_version"`
	Manifest      Manifest `json:"manifest"`
	Digest        string   `json:"digest"`
	Count         int      `json:"count"`
	Bytes         int64    `json:"bytes"`
}
type Body struct {
	Session string `json:"session"`
	Kind    string `json:"kind"`
	Body    string `json:"body"`
}
type Audit struct {
	Session string `json:"session"`
	Bytes   int    `json:"bytes"`
	SHA256  string `json:"sha256"`
}
type Batch struct {
	Sessions  []Body  `json:"sessions"`
	Audit     []Audit `json:"audit"`
	Selection string  `json:"selection"`
	Next      int     `json:"next"`
	Complete  bool    `json:"complete"`
}

func hash(b []byte) string     { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }
func planDigest(p Plan) string { p.Digest = ""; raw, _ := json.Marshal(p); return hash(raw) }
func canonicalDirectory(path string) error {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsRune(path, 0) {
		return ErrSelection
	}
	f, err := scheduler.OpenDirectory(path)
	if err != nil {
		return ErrSelection
	}
	return f.Close()
}
func Prepare(manifest Manifest) (Plan, error) {
	p := Plan{SchemaVersion: 1, Manifest: Manifest{Workspaces: append([]string{}, manifest.Workspaces...), Sources: []Source{}}}
	if len(manifest.Workspaces) < 1 || len(manifest.Workspaces) > 4 || len(manifest.Sources) < 1 || len(manifest.Sources) > 4 {
		return p, ErrSelection
	}
	allowed := map[string]bool{}
	for _, path := range manifest.Workspaces {
		if allowed[path] || canonicalDirectory(path) != nil {
			return p, ErrSelection
		}
		allowed[path] = true
	}
	seen := map[string]bool{}
	physicalSeen := map[string]bool{}
	for _, source := range manifest.Sources {
		if !allowed[source.Workspace] || (source.Kind != "claude_code" && source.Kind != "claude_export") || canonicalDirectory(source.Root) != nil || len(source.Sessions) < 1 {
			return p, ErrSelection
		}
		selected := Source{Kind: source.Kind, Workspace: source.Workspace, Root: source.Root, Sessions: []Session{}}
		for _, session := range source.Sessions {
			name := session.Path
			if len(session.ID) < 1 || len(session.ID) > 128 || strings.ContainsRune(session.ID, 0) || name == "" || filepath.IsAbs(name) || filepath.ToSlash(filepath.Clean(name)) != name || strings.ContainsAny(name, "\\*?[]\x00:") || name == ".." || strings.HasPrefix(name, "../") || strings.Contains(name, "/../") {
				return p, ErrSelection
			}
			ext := ".jsonl"
			if source.Kind == "claude_export" {
				ext = ".json"
			}
			if filepath.Ext(name) != ext {
				return p, ErrSelection
			}
			path := filepath.Join(source.Root, filepath.FromSlash(name))
			if seen[path] {
				return p, ErrSelection
			}
			seen[path] = true
			f, err := scheduler.OpenRegularFile(path)
			if err != nil {
				return p, ErrSelection
			}
			info, statErr := f.Stat()
			physical, stamp, identityErr := fileIdentity(f)
			linkErr := scheduler.ValidateSingleLink(f)
			closeErr := f.Close()
			if statErr != nil || identityErr != nil || linkErr != nil || closeErr != nil || info.Size() > MaxFileBytes {
				return p, ErrSelection
			}
			if physicalSeen[physical] {
				return p, ErrSelection
			}
			physicalSeen[physical] = true
			p.Count++
			p.Bytes += info.Size()
			if p.Count > 20 || p.Bytes > MaxTotalBytes {
				return p, ErrSelection
			}
			selected.Sessions = append(selected.Sessions, Session{Path: name, ID: session.ID, Identity: stamp, Bytes: info.Size()})
		}
		p.Manifest.Sources = append(p.Manifest.Sources, selected)
	}
	p.Digest = planDigest(p)
	return p, nil
}
func Read(plan Plan, expected string, start, limit int) (Batch, error) {
	out := Batch{Sessions: []Body{}, Audit: []Audit{}, Selection: expected}
	if plan.SchemaVersion != 1 || plan.Digest != expected || planDigest(plan) != expected || limit < 1 || limit > 5 || start < 0 || start > plan.Count {
		return Batch{}, ErrSelection
	}
	fresh, err := Prepare(plan.Manifest)
	if err != nil || fresh.Digest != expected {
		return Batch{}, ErrSelection
	}
	end := min(start+limit, plan.Count)
	index := 0
	for _, source := range plan.Manifest.Sources {
		for _, session := range source.Sessions {
			current := index
			index++
			if current < start || current >= end {
				continue
			}
			f, e := scheduler.OpenRegularFile(filepath.Join(source.Root, filepath.FromSlash(session.Path)))
			if e != nil {
				return Batch{}, ErrSelection
			}
			_, before, e := fileIdentity(f)
			if e != nil || before != session.Identity || scheduler.ValidateSingleLink(f) != nil {
				f.Close()
				return Batch{}, ErrSelection
			}
			body, e := io.ReadAll(io.LimitReader(f, MaxFileBytes+1))
			_, after, stampErr := fileIdentity(f)
			closeErr := f.Close()
			if e != nil || stampErr != nil || closeErr != nil || before != after || int64(len(body)) != session.Bytes || len(body) > MaxFileBytes {
				return Batch{}, ErrSelection
			}
			body = bytes.TrimPrefix(body, []byte{0xef, 0xbb, 0xbf})
			if !utf8.Valid(body) {
				return Batch{}, ErrSelection
			}
			if source.Kind == "claude_code" {
				scanner := bufio.NewScanner(bytes.NewReader(body))
				scanner.Buffer(make([]byte, 4096), MaxFileBytes+1)
				for scanner.Scan() {
					var row map[string]json.RawMessage
					if json.Unmarshal(scanner.Bytes(), &row) != nil || row == nil {
						return Batch{}, ErrSelection
					}
					if raw, ok := row["cwd"]; ok && string(raw) != "null" {
						var cwd string
						if json.Unmarshal(raw, &cwd) != nil || cwd != source.Workspace {
							return Batch{}, ErrSelection
						}
					}
				}
				if scanner.Err() != nil {
					return Batch{}, ErrSelection
				}
			} else {
				var conversations []json.RawMessage
				if json.Unmarshal(body, &conversations) != nil || conversations == nil || len(conversations) > 20 {
					return Batch{}, ErrSelection
				}
			}
			token := hash([]byte(expected + "\x00" + session.ID + "\x00" + source.Root))
			out.Sessions = append(out.Sessions, Body{token, source.Kind, string(body)})
			out.Audit = append(out.Audit, Audit{token, len(body), hash(body)})
		}
	}
	// A batch never releases a partial result if any selected source changed.
	final, e := Prepare(plan.Manifest)
	if e != nil || final.Digest != expected {
		return Batch{}, ErrSelection
	}
	out.Next = end
	out.Complete = end == plan.Count
	return out, nil
}
