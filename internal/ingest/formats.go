package ingest

import "errors"

// FormatRoute is selected by canonical policy, never by a model or a provider.
// All routes run within the same authenticated model-free Docling runtime pack.
type FormatRoute struct {
	Route          string   `json:"route"`
	Parser         string   `json:"parser"`
	Fidelity       Fidelity `json:"fidelity"`
	FallbackReason string   `json:"fallback_reason,omitempty"`
}

// SelectFormat preserves native Docling parsing where available. A JSON export
// candidate must still pass DoclingDocument validation inside the signed pack.
func SelectFormat(extension string, doclingJSONCandidate bool) (FormatRoute, error) {
	r := FormatRoute{Route: "docling_local", Fidelity: FidelityStructured}
	switch extension {
	case ".docx":
		r.Parser = "docling.docx"
	case ".csv":
		r.Parser = "docling.csv"
	case ".html", ".htm":
		r.Parser = "docling.html"
	case ".epub":
		r.Parser = "docling.epub"
	case ".md":
		r.Parser = "docling.markdown"
	case ".xlsx":
		r.Parser = "docling.xlsx"
	case ".json":
		if doclingJSONCandidate {
			r.Parser = "docling.json"
			break
		}
		fallthrough
	case ".txt", ".xml":
		r.Route, r.Parser, r.Fidelity = "maestro_literal_v1", "maestro.literal_utf8.v1", FidelityTextual
		r.FallbackReason = "native_generic_literal_unsupported"
	case ".xls":
		r.Route, r.Parser, r.Fidelity = "maestro_biff_v1", "xlrd.biff", FidelityTextual
		r.FallbackReason = "native_xls_requires_unmanaged_office"
	default:
		return FormatRoute{}, errors.New("format_unsupported")
	}
	return r, nil
}
