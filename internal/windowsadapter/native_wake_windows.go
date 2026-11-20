//go:build windows

package windowsadapter

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"os/exec"
	"path/filepath"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

func runNativeWake(ctx context.Context, script string, input []byte) ([]byte, error) {
	dir, err := windows.GetSystemDirectory()
	if err != nil {
		return nil, err
	}
	units := utf16.Encode([]rune(script))
	encoded := make([]byte, len(units)*2)
	for i, v := range units {
		binary.LittleEndian.PutUint16(encoded[i*2:], v)
	}
	cmd := exec.CommandContext(ctx, filepath.Join(dir, "WindowsPowerShell", "v1.0", "powershell.exe"), "-NoLogo", "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(encoded))
	cmd.Stdin = bytes.NewReader(input)
	var output nativeWakeOutput
	cmd.Stdout = &output
	if err := cmd.Run(); err != nil {
		return nil, errors.New("native task operation refused or acknowledgement unavailable; preserve task and inspect exact owned state")
	}
	return output.Bytes(), nil
}

type nativeWakeOutput struct{ bytes.Buffer }

func (b *nativeWakeOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 16384 {
		return 0, errors.New("native task output exceeds bound")
	}
	return b.Buffer.Write(p)
}
