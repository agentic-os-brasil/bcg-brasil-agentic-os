//go:build windows

package scheduler

import "errors"

// PublishNewPrivateFile remains unavailable until Windows mutation custody is qualified.
func PublishNewPrivateFile(path string, body []byte) error {
	return errors.New("exclusive private publication unavailable")
}
