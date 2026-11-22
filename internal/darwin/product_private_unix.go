//go:build !windows

package darwin

func holdProductPrivateFiles(string) (*productPrivateFiles, func() error, error) {
	return nil, func() error { return nil }, nil
}
