// Package pilotdocument owns only the fixed public synthetic conversion input.
// Its bytes and marker are test data, never installer or runtime authority.
package pilotdocument

import (
	"crypto/sha256"
	_ "embed"
	"errors"
	"fmt"
)

const Name = "maestro-pilot.docx"
const SHA256 = "6bd282a12170035e8af5352f2de5a21f4fe4bc5c7919283482a26703c206bf81"

// Identical public source: internal/ingest/docling/testdata/sample.docx.
//
//go:embed maestro-pilot.docx
var source []byte

func Bytes() ([]byte, error) {
	if fmt.Sprintf("%x", sha256.Sum256(source)) != SHA256 {
		return nil, errors.New("canonical public pilot document changed")
	}
	return append([]byte(nil), source...), nil
}
