//go:build windows

package datahome

import "errors"

func initializeFresh(Options, Result) error {
	return errors.New("fresh bootstrap requires verified Windows ownership and ACLs")
}
