package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"testing"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
)

func TestInstalledRuntimeLocalCommandsCannotGrantUninstalledAuthority(t *testing.T) {
	for _, command := range []string{"runtime-review", "runtime-apply", "maestro_apply", "install"} {
		t.Run(command, func(t *testing.T) {
			var output bytes.Buffer
			if err := runInstalledRuntime(context.Background(), command, product.Options{}, "", "", false, &output); err == nil {
				t.Fatal("uninstalled/unconfirmed command admitted")
			}
		})
	}
}

func TestRuntimeApplyErrorMessagePreservesPrivacy(t *testing.T) {
	const generic = "maestro: operation unavailable; inspect the selected binding before retrying"
	for _, test := range []struct {
		name string
		err  error
		want string
	}{
		{"wrapped path", fmt.Errorf("private wrapper /owner/home: %w", &os.PathError{Op: "openat", Path: "/private/input", Err: syscall.EACCES}), fmt.Sprintf("%s [filesystem operation=openat errno=%d]", generic, syscall.EACCES)},
		{"wrapped link", fmt.Errorf("private wrapper: %w", &os.LinkError{Op: "rename", Old: "/private/old", New: "/private/new", Err: syscall.EXDEV}), fmt.Sprintf("%s [filesystem operation=rename errno=%d]", generic, syscall.EXDEV)},
		{"bare errno", syscall.EACCES, fmt.Sprintf("%s [filesystem operation=filesystem errno=%d]", generic, syscall.EACCES)},
		{"private operation", &os.PathError{Op: "open\n/private/input", Path: "/private/input", Err: syscall.EACCES}, fmt.Sprintf("%s [filesystem operation=filesystem errno=%d]", generic, syscall.EACCES)},
		{"unknown link operation", &os.LinkError{Op: "/private/operation", Old: "/private/old", New: "/private/new", Err: syscall.EXDEV}, fmt.Sprintf("%s [filesystem operation=filesystem errno=%d]", generic, syscall.EXDEV)},
		{"private logical error", errors.New("private input /owner/home"), generic},
		{"path without errno", &os.PathError{Op: "open", Path: "/private/input", Err: errors.New("private refusal")}, generic},
		{"joined path errno", errors.Join(&os.PathError{Op: "open", Path: "/private/input", Err: syscall.EACCES}, syscall.EXDEV), fmt.Sprintf("%s [filesystem operation=open errno=%d]", generic, syscall.EACCES)},
		{"joined logical path", errors.Join(&os.PathError{Op: "open", Path: "/private/input", Err: errors.New("private refusal")}, syscall.EXDEV), generic},
		{"joined logical link", errors.Join(&os.LinkError{Op: "rename", Old: "/private/old", New: "/private/new", Err: errors.New("private refusal")}, syscall.EACCES), generic},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := runtimeApplyErrorMessage(test.err); got != test.want {
				t.Fatalf("runtime-apply diagnostic = %q; want %q", got, test.want)
			}
		})
	}
}
