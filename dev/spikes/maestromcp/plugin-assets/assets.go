// Package assets contains only the non-production plugin's operator guidance.
package assets

import "embed"

//go:embed skills/*/SKILL.md
var Files embed.FS
