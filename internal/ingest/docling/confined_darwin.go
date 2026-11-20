//go:build darwin

package docling

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"time"
)

func runConfinedBridge(ctx context.Context, work, snapshot string, request []byte, stdout, stderr *boundedOutput) error {
	command := exec.CommandContext(ctx, "/usr/bin/sandbox-exec", "-p", "(version 1)(allow default)(deny network*)", filepath.Join(snapshot, "python/bin/python3.12"), "-I", "-S", "-B", filepath.Join(snapshot, "bridge.py"))
	command.Dir = work
	command.Env = bridgeEnvironment(work)
	command.Stdin = bytes.NewReader(request)
	command.Stdout, command.Stderr = stdout, stderr
	command.WaitDelay = time.Second
	return command.Run()
}
