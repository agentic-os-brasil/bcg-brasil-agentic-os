//go:build !windows

package docling

import "context"

func qualificationStatus(ctx context.Context, _ Options, _ verifiedPack) string {
	if ctx.Err() != nil {
		return "conversion_cancelled"
	}
	return ""
}
func qualifyPlaced(context.Context, Options) error { return nil }
