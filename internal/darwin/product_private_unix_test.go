//go:build !windows

package darwin_test

import "os"

func exposeProductCache(path string) error { return os.Chmod(path, 0644) }

func productCacheProtection(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	return info.Mode().String(), nil
}
