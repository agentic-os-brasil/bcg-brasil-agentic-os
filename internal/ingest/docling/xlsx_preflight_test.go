package docling_test

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest"
)

const worksheetNamespace = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"

func worksheetFixture(dimension, rows string, merges, links []string) string {
	var s strings.Builder
	fmt.Fprintf(&s, `<worksheet xmlns="%s">`, worksheetNamespace)
	if dimension != "" {
		fmt.Fprintf(&s, `<dimension ref="%s"/>`, dimension)
	}
	s.WriteString("<sheetData>" + rows + "</sheetData>")
	if len(merges) > 0 {
		fmt.Fprintf(&s, `<mergeCells count="%d">`, len(merges))
		for _, ref := range merges {
			fmt.Fprintf(&s, `<mergeCell ref="%s"/>`, ref)
		}
		s.WriteString("</mergeCells>")
	}
	if len(links) > 0 {
		s.WriteString("<hyperlinks>")
		for _, ref := range links {
			fmt.Fprintf(&s, `<hyperlink ref="%s" location="INTERNAL_TARGET"/>`, ref)
		}
		s.WriteString("</hyperlinks>")
	}
	s.WriteString("</worksheet>")
	return s.String()
}

func workbookFixture(t *testing.T, sheets ...string) []byte {
	t.Helper()
	raw := read(t, "testdata/policy/sheets.xlsx")
	src, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	for _, f := range src.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(r)
		_ = r.Close()
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 2; i++ {
			if f.Name == fmt.Sprintf("xl/worksheets/sheet%d.xml", i+1) {
				sheet := worksheetFixture("A1", "", nil, nil)
				if i < len(sheets) {
					sheet = sheets[i]
				}
				body = []byte(sheet)
			}
		}
		writer, err := w.Create(f.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestXLSXNativeBudgetsRefuseBeforeAuthentication(t *testing.T) {
	anchor := `<row r="1"><c r="A1" t="inlineStr"><is><t>CLIP_ANCHOR</t></is></c></row>`
	for _, test := range []struct {
		name   string
		sheets []string
	}{
		{"single merge", []string{worksheetFixture("A1", anchor, []string{"A1:A100002"}, nil)}},
		{"cross sheet merges", []string{worksheetFixture("A1", anchor, []string{"A1:A60001"}, nil), worksheetFixture("A1", anchor, []string{"A1:A40002"}, nil)}},
		{"hyperlink", []string{worksheetFixture("A1", anchor, nil, []string{"A1:A10002"})}},
		{"cross sheet hyperlinks", []string{worksheetFixture("A1", anchor, nil, []string{"A1:A6001"}), worksheetFixture("A1", anchor, nil, []string{"A1:A4002"})}},
	} {
		t.Run(test.name, func(t *testing.T) {
			r, _, space, id := unseededPolicyWorkspace(t)
			put(t, filepath.Join(space, "limits.xlsx"), workbookFixture(t, test.sheets...))
			got, err := r.Convert(context.Background(), ingest.Request{SourcePath: "limits.xlsx", WorkspacePath: space, Policy: ingest.DefaultPolicy()}, id)
			if err != nil || got.Result.Status != ingest.StatusBlocked || len(got.Result.Warnings) != 1 || got.Result.Warnings[0] != "source_format_invalid" || len(got.Document) > 0 || len(got.Markdown) > 0 {
				t.Fatalf("native clipping input reached authentication: %+v %v", got.Result, err)
			}
		})
	}
}

func TestXLSXActualGeometryRefusesBeforeAuthentication(t *testing.T) {
	// Construct only bounded XML. Never pass this allocation-bomb U shape to
	// an authenticated parser: 36,382 values enclose 163,840,000 output cells.
	var u strings.Builder
	u.WriteString(`<row r="1">`)
	for i := 0; i < 16384; i++ {
		u.WriteString(`<c><v>1</v></c>`)
	}
	u.WriteString(`</row>`)
	for row := 2; row <= 10000; row++ {
		fmt.Fprintf(&u, `<row r="%d"><c r="A%d"><v>1</v></c><c r="XFD%d"><v>1</v></c></row>`, row, row, row)
	}
	for _, dimension := range []string{"", "A1"} {
		t.Run("dimension="+dimension, func(t *testing.T) {
			r, _, space, id := unseededPolicyWorkspace(t)
			put(t, filepath.Join(space, "geometry.xlsx"), workbookFixture(t, worksheetFixture(dimension, u.String(), nil, nil)))
			got, err := r.Convert(context.Background(), ingest.Request{SourcePath: "geometry.xlsx", WorkspacePath: space, Policy: ingest.DefaultPolicy()}, id)
			if err != nil || got.Result.Status != ingest.StatusBlocked || len(got.Result.Warnings) != 1 || got.Result.Warnings[0] != "source_format_invalid" {
				t.Fatalf("actual geometry bypass reached authentication: %+v %v", got.Result, err)
			}
		})
	}
}

func TestXLSXBoundedInputsReachAuthentication(t *testing.T) {
	anchor := `<row r="1"><c r="A1"><v>1</v></c></row>`
	for _, test := range []struct {
		name   string
		sheets []string
	}{
		{"merge exact workbook budget", []string{worksheetFixture("A1", anchor, []string{"A1:A60001"}, nil), worksheetFixture("A1", anchor, []string{"A1:A40001"}, nil)}},
		{"hyperlink exact workbook budget", []string{worksheetFixture("A1", anchor, nil, []string{"A1:A6001"}), worksheetFixture("A1", anchor, nil, []string{"A1:A4001"})}},
		{"single anchors charge zero", []string{worksheetFixture("A1", anchor, []string{"A1:A100001", "B1"}, []string{"C1:C10001", "D1"})}},
		{"inferred coordinates", []string{worksheetFixture("", `<row><c><v>11</v></c><c><v>12</v></c></row><row r="3"><c r="D3"><v>31</v></c><c><v>32</v></c></row>`, nil, nil)}},
		{"separated sparse values", []string{worksheetFixture("", anchor+`<row r="10000"><c r="XFD10000"><v>2</v></c></row>`, nil, nil)}},
		{"distant formatting is not value geometry", []string{worksheetFixture("", anchor+`<row r="10000"><c r="XFD10000" s="0"/></row>`, nil, nil)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			r, _, space, id := unseededPolicyWorkspace(t)
			put(t, filepath.Join(space, "bounded.xlsx"), workbookFixture(t, test.sheets...))
			got, err := r.Convert(context.Background(), ingest.Request{SourcePath: "bounded.xlsx", WorkspacePath: space, Policy: ingest.DefaultPolicy()}, id)
			if err != nil || got.Result.Status != ingest.StatusUnavailable || len(got.Result.Warnings) != 1 || got.Result.Warnings[0] != "authority_unavailable" {
				t.Fatalf("bounded input was not admitted to authentication: %+v %v", got.Result, err)
			}
		})
	}
}

func TestXLSXCoordinatesAndRangeEffectsRefuseBeforeAuthentication(t *testing.T) {
	var top strings.Builder
	top.WriteString(`<row r="1">`)
	for i := 0; i < 16384; i++ {
		top.WriteString(`<c><v>1</v></c>`)
	}
	top.WriteString(`</row>`)
	for _, test := range []struct {
		name, rows    string
		merges, links []string
	}{
		{"column overflow", `<row r="1"><c r="XFE1"><v>1</v></c></row>`, nil, nil},
		{"row overflow", `<row r="1"><c r="A1048577"><v>1</v></c></row>`, nil, nil},
		{"inferred row overflow", `<row r="1048576"/><row><c><v>1</v></c></row>`, nil, nil},
		{"ambiguous cell outside row", `<c><v>1</v></c>`, nil, nil},
		{"foreign namespace row child", `<row r="1"><q:c xmlns:q="urn:other" r="A1"><v>1</v></q:c></row>`, nil, nil},
		{"noncell row child", `<row r="1"><other r="A1"><v>1</v></other></row>`, nil, nil},
		{"hyperlink completes broad connected rectangle", top.String(), nil, []string{"A2:A1000", "XFD2:XFD1000"}},
		{"merge completes broad connected rectangle", top.String(), []string{"A2:A100", "XFD2:XFD100"}, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			r, _, space, id := unseededPolicyWorkspace(t)
			put(t, filepath.Join(space, "effects.xlsx"), workbookFixture(t, worksheetFixture("A1", test.rows, test.merges, test.links)))
			got, err := r.Convert(context.Background(), ingest.Request{SourcePath: "effects.xlsx", WorkspacePath: space, Policy: ingest.DefaultPolicy()}, id)
			if err != nil || got.Result.Status != ingest.StatusBlocked || len(got.Result.Warnings) != 1 || got.Result.Warnings[0] != "source_format_invalid" {
				t.Fatalf("unsafe coordinates/effects reached authentication: %+v %v", got.Result, err)
			}
		})
	}
}

func TestXLSXProjectedBudgetIsWorkbookWide(t *testing.T) {
	// These are preflight-only inputs. The boundary controls inspect sparse
	// XML; no parser is asked to allocate either rectangular table.
	u := func(width int, last string) string {
		var b strings.Builder
		b.WriteString(`<row r="1">`)
		for i := 0; i < width; i++ {
			b.WriteString(`<c><v>1</v></c>`)
		}
		b.WriteString(`</row>`)
		for row := 2; row <= 1000; row++ {
			fmt.Fprintf(&b, `<row r="%d"><c r="A%d"><v>1</v></c><c r="%s%d"><v>1</v></c></row>`, row, row, last, row)
		}
		return b.String()
	}
	for _, test := range []struct {
		name    string
		width   int
		last    string
		blocked bool
	}{
		{"exact million", 500, "SF", false},
		{"aggregate exceeds million", 601, "WC", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			r, _, space, id := unseededPolicyWorkspace(t)
			sheet := worksheetFixture("", u(test.width, test.last), nil, nil)
			put(t, filepath.Join(space, "aggregate.xlsx"), workbookFixture(t, sheet, sheet))
			got, err := r.Convert(context.Background(), ingest.Request{SourcePath: "aggregate.xlsx", WorkspacePath: space, Policy: ingest.DefaultPolicy()}, id)
			status, reason := ingest.StatusUnavailable, "authority_unavailable"
			if test.blocked {
				status, reason = ingest.StatusBlocked, "source_format_invalid"
			}
			if err != nil || got.Result.Status != status || len(got.Result.Warnings) != 1 || got.Result.Warnings[0] != reason {
				t.Fatalf("workbook projection budget: %+v %v", got.Result, err)
			}
		})
	}
}
