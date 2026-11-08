package zipmigration

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"testing"
)

func TestExternalMissingRequiresOnlyAbsence(t *testing.T) {
	closeFailure := errors.New("synthetic close failure")
	busy := errors.New("synthetic busy object")
	cases := []struct {
		name    string
		err     error
		missing bool
	}{
		{"nil", nil, false},
		{"other", errors.New("unavailable"), false},
		{"permission", fs.ErrPermission, false},
		{"direct", fs.ErrNotExist, true},
		{"path wrapper", &os.PathError{Op: "NtCreateFile", Path: "synthetic", Err: fs.ErrNotExist}, true},
		{"formatted wrapper", fmt.Errorf("native admission: %w", fs.ErrNotExist), true},
		{"joined native absence", errors.Join(&os.PathError{Op: "NtCreateFile", Path: "synthetic", Err: fs.ErrNotExist}, nil), true},
		{"two absences", errors.Join(fs.ErrNotExist, fmt.Errorf("parent: %w", fs.ErrNotExist)), true},
		{"nested joins", fmt.Errorf("native cleanup: %w", errors.Join(errors.Join(fs.ErrNotExist, nil), fs.ErrNotExist)), true},
		{"missing and permission", errors.Join(fs.ErrNotExist, fs.ErrPermission), false},
		{"missing and busy", errors.Join(fs.ErrNotExist, busy), false},
		{"missing and close failure", errors.Join(fs.ErrNotExist, closeFailure), false},
		{"wrapped mixed join", fmt.Errorf("native cleanup: %w", errors.Join(fs.ErrNotExist, fmt.Errorf("close: %w", closeFailure))), false},
		{"nested mixed joins", errors.Join(errors.Join(fs.ErrNotExist, fs.ErrPermission), fs.ErrNotExist), false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := externalMissing(test.err); got != test.missing {
				t.Fatalf("absence=%t, want %t: %v", got, test.missing, test.err)
			}
		})
	}
}
