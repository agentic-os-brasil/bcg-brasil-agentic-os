// Package sessionresolve authorizes bounded, explicit reads of pointers that a
// Session Context Packet already exposed. Hooks never call this package.
package sessionresolve

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/sessionctx"
)

const MaximumBytes = 8192

const (
	SessionPurpose              = "session"
	OwnerPersonalContextPurpose = "owner-personal-context"
	personalContextPointer      = "owner/self/personal-context.md"
)

type Result struct {
	Pointer string `json:"pointer"`
	Purpose string `json:"purpose"`
	State   string `json:"state"`
	Body    string `json:"body,omitempty"`
}

func Resolve(dataRoot, pointer, purpose string, packet sessionctx.Packet, budget int) (Result, error) {
	if purpose != SessionPurpose && purpose != OwnerPersonalContextPurpose {
		return Result{}, errors.New("purpose must be session or owner-personal-context")
	}
	if budget <= 0 || budget > MaximumBytes {
		return Result{}, fmt.Errorf("budget must be between 1 and %d bytes", MaximumBytes)
	}
	if pointer == packet.Owner.OperatingState.Path && pointer != "" && packet.Owner.OperatingState.Available {
		if purpose != SessionPurpose {
			return Result{}, errors.New("operating state is only authorized for purpose=session")
		}
		return read(dataRoot, pointer, purpose, budget)
	}
	for _, facet := range packet.Owner.Facets {
		if facet.Path == pointer && facet.Available {
			if pointer == personalContextPointer && purpose != OwnerPersonalContextPurpose {
				return Result{}, errors.New("personal context requires purpose=owner-personal-context")
			}
			if pointer != personalContextPointer && purpose == OwnerPersonalContextPurpose {
				return Result{}, errors.New("purpose=owner-personal-context is limited to personal context")
			}
			return read(dataRoot, pointer, purpose, budget)
		}
	}
	return Result{}, errors.New("pointer is unavailable or not authorized for this session")
}

func read(root, pointer, purpose string, budget int) (Result, error) {
	clean := filepath.Clean(filepath.FromSlash(pointer))
	if filepath.IsAbs(clean) || clean == "." || clean == ".." || len(clean) >= 3 && clean[:3] == ".."+string(filepath.Separator) {
		return Result{}, errors.New("pointer is not a safe local relative path")
	}
	path := filepath.Join(root, clean)
	if err := rejectSymlinks(root, clean); err != nil {
		return Result{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return Result{}, err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, int64(budget)+1))
	if err != nil {
		return Result{}, err
	}
	if len(body) > budget {
		return Result{Pointer: pointer, Purpose: purpose, State: "budget_exceeded"}, nil
	}
	return Result{Pointer: pointer, Purpose: purpose, State: "available", Body: string(body)}, nil
}

func rejectSymlinks(root, relative string) error {
	current := root
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("pointer may not traverse a symlink")
		}
	}
	return nil
}
