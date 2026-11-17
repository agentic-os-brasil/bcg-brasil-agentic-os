//go:build !windows

package docling

import "context"

func qualificationStatus(context.Context, Options, verifiedPack) string { return "" }
func qualifyPlaced(context.Context, Options) error                      { return nil }
