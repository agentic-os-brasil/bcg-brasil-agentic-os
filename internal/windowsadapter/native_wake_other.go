//go:build !windows

package windowsadapter

import "context"

func runNativeWake(context.Context, string, []byte) ([]byte, error) { return nil, ErrNativeUnavailable }
