//go:build !windows

package packaging

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"time"
)

func validateNativeScratchIdentity(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) {
		return errors.New("native scratch must belong to the current owner")
	}
	return nil
}
func configureNativeProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}

func nativeManagedDirectory() (string, error) {
	if runtime.GOOS == "darwin" {
		return "/Library/Application Support/ClaudeCode", nil
	}
	return "/etc/claude-code", nil
}
func nativeSourceDirectoryIdentity(file *os.File) (string, error) {
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() {
		return "", errors.New("native source directory identity unavailable")
	}
	return fmt.Sprintf("%d:%d", stat.Dev, stat.Ino), nil
}

func nativeProcessBoundary() string {
	return "Timeout cancels the qualification-owned Unix process group. Independently detached host descendants are not attested; a timeout never qualifies and reports cleanup unavailable. No global process cleanup occurs."
}
func runOwnedNativeCommand(ctx context.Context, path string, args []string, cwd string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Dir = cwd
	cmd.Stderr = io.Discard
	cmd.WaitDelay = 2 * time.Second
	configureNativeProcess(cmd)
	var stdout boundedNativeOutput
	cmd.Stdout = &stdout
	err := cmd.Run()
	return stdout.Bytes(), err
}
func makeNativePrivateDirectory(path string) (func(), error) {
	if err := os.Mkdir(path, 0700); err != nil {
		return nil, err
	}
	return func() {}, nil
}
