package zipruntime

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"unicode/utf8"
)

// capContext caps the entire emitted packet, including profile and routing text.
func capContext(root, rawCap string, in io.Reader, out io.Writer) error {
	limit, err := strconv.Atoi(rawCap)
	if err != nil || limit < 1 || limit > 65536 {
		return errors.New("invalid context cap")
	}
	body, err := io.ReadAll(io.LimitReader(in, int64(limit)+1))
	if err != nil {
		return err
	}
	extra, err := io.Copy(io.Discard, in)
	if err != nil {
		return err
	}
	original := int64(len(body)) + extra
	truncated := original > int64(limit)
	if truncated {
		notice := []byte("\n[Maestro: contexto truncado ao limite; consulte as fontes sob demanda.]\n<!-- maestro:session-context:end -->\n")
		budget := limit
		if len(notice) <= limit {
			budget -= len(notice)
		} else {
			notice = nil
		}
		body = body[:budget]
		for !utf8.Valid(body) && len(body) > 0 {
			body = body[:len(body)-1]
		}
		body = append(body, notice...)
	}
	// Receipt is diagnostic metadata, never content. Refuse aliased parent paths.
	physical, e := filepath.EvalSymlinks(root)
	if e == nil {
		dest := filepath.Join(physical, "brain/.maestro/context-packet.json")
		resolved, e := resolveProspective(dest)
		if e == nil && resolved == dest {
			if e = os.MkdirAll(filepath.Dir(dest), 0700); e == nil {
				receipt, _ := json.Marshal(map[string]any{"schema_version": 1, "session_stdout_bytes": len(body), "source_bytes": original, "cap_bytes": limit, "truncated": truncated, "evidence": "adapter_command"})
				handle, e := os.OpenRoot(physical)
				if e == nil {
					file, e := handle.OpenFile("brain/.maestro/context-packet.json", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
					if e == nil {
						_, _ = file.Write(receipt)
						_ = file.Close()
					}
					_ = handle.Close()
				}
			}
		}
	}
	_, err = out.Write(body)
	return err
}
