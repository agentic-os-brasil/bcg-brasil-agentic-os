package pilotdocument

import (
	"bytes"
	"os"
	"testing"
)

func TestPublicPilotDocumentMatchesCanonicalDoclingSampleAndOwnsBytes(t *testing.T) {
	b, err := Bytes()
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := os.ReadFile("../ingest/docling/testdata/sample.docx")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, canonical) {
		t.Fatal("public pilot fixture diverged from canonical marker/table source")
	}
	b[0] ^= 1
	again, err := Bytes()
	if err != nil || !bytes.Equal(again, canonical) {
		t.Fatal("caller changed canonical public fixture")
	}
}
