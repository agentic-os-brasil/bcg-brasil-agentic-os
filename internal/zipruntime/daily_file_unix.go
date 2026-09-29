//go:build !windows

package zipruntime

import (
	"errors"
	"os"
	"syscall"
)

// O_NONBLOCK prevents FIFO replacement between Lstat and open from hanging a
// hook. O_NOFOLLOW and descriptor validation close final-component alias races.
func dailyOpenRegular(path string) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("nonregular daily input")
	}
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	s, err := f.Stat()
	if err != nil || !s.Mode().IsRegular() {
		f.Close()
		return nil, errors.New("nonregular daily input")
	}
	stat, ok := s.Sys().(*syscall.Stat_t)
	if !ok || stat.Nlink != 1 {
		f.Close()
		return nil, errors.New("linked daily input")
	}
	return f, nil
}
