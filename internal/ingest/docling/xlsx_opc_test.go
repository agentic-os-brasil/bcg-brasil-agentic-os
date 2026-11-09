package docling_test

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest"
)

// Edits preserve a complete OPC graph; moved parts update both declarations
// and relationships rather than proving refusal with a missing-part stub.
func editXLSX(t *testing.T, body []byte, edit func(map[string]string)) []byte {
	t.Helper()
	z, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	parts := map[string]string{}
	for _, f := range z.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(r)
		_ = r.Close()
		if err != nil {
			t.Fatal(err)
		}
		parts[f.Name] = string(b)
	}
	edit(parts)
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	names := []string{}
	for n := range parts {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		f, err := w.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = io.WriteString(f, parts[n]); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestXLSXConsumedOPCRefusesBeforeAuthentication(t *testing.T) {
	base := workbookFixture(t, worksheetFixture("A1", `<row r="1"><c r="A1"><v>1</v></c></row>`, []string{"A1:A2"}, []string{"B1:B2"}))
	for _, test := range []struct {
		name string
		edit func(map[string]string)
	}{
		{"noncanonical worksheet prefix", func(p map[string]string) {
			p["xl/custom/sheet1.xml"] = p["xl/worksheets/sheet1.xml"]
			delete(p, "xl/worksheets/sheet1.xml")
			p["[Content_Types].xml"] = strings.ReplaceAll(p["[Content_Types].xml"], "/xl/worksheets/sheet1.xml", "/xl/custom/sheet1.xml")
			p["xl/_rels/workbook.xml.rels"] = strings.ReplaceAll(p["xl/_rels/workbook.xml.rels"], "worksheets/sheet1.xml", "custom/sheet1.xml")
		}},
		{"noncanonical worksheet suffix", func(p map[string]string) {
			p["xl/worksheets/sheet1.part"] = p["xl/worksheets/sheet1.xml"]
			delete(p, "xl/worksheets/sheet1.xml")
			p["[Content_Types].xml"] = strings.ReplaceAll(p["[Content_Types].xml"], "/xl/worksheets/sheet1.xml", "/xl/worksheets/sheet1.part")
			p["xl/_rels/workbook.xml.rels"] = strings.ReplaceAll(p["xl/_rels/workbook.xml.rels"], "worksheets/sheet1.xml", "worksheets/sheet1.part")
		}},
		{"alternate selected workbook with canonical decoy", func(p map[string]string) {
			p["xl/actual.xml"] = p["xl/workbook.xml"]
			p["xl/_rels/actual.xml.rels"] = p["xl/_rels/workbook.xml.rels"]
			p["[Content_Types].xml"] = strings.ReplaceAll(p["[Content_Types].xml"], "/xl/workbook.xml", "/xl/actual.xml")
			p["xl/workbook.xml"] = `<workbook xmlns="` + worksheetNamespace + `"><sheets/></workbook>`
		}},
		{"repeated resolved worksheet", func(p map[string]string) {
			p["xl/_rels/workbook.xml.rels"] = strings.ReplaceAll(p["xl/_rels/workbook.xml.rels"], "worksheets/sheet2.xml", "worksheets/sheet1.xml")
		}},
		{"normalized alias repeats worksheet", func(p map[string]string) {
			p["xl/_rels/workbook.xml.rels"] = strings.ReplaceAll(p["xl/_rels/workbook.xml.rels"], "worksheets/sheet2.xml", "worksheets/../worksheets/sheet1.xml")
		}},
		{"absolute target repeats relative target", func(p map[string]string) {
			p["xl/_rels/workbook.xml.rels"] = strings.ReplaceAll(p["xl/_rels/workbook.xml.rels"], "worksheets/sheet2.xml", "/xl/worksheets/sheet1.xml")
		}},
		{"foreign merge child", func(p map[string]string) {
			p["xl/worksheets/sheet1.xml"] = strings.ReplaceAll(p["xl/worksheets/sheet1.xml"], `<mergeCell ref=`, `<q:mergeCell xmlns:q="urn:other" ref=`)
		}},
		{"foreign hyperlink child", func(p map[string]string) {
			p["xl/worksheets/sheet1.xml"] = strings.ReplaceAll(p["xl/worksheets/sheet1.xml"], `<hyperlink ref=`, `<q:hyperlink xmlns:q="urn:other" ref=`)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			r, _, space, id := unseededPolicyWorkspace(t)
			put(t, filepath.Join(space, "coverage.xlsx"), editXLSX(t, base, test.edit))
			got, err := r.Convert(context.Background(), ingest.Request{SourcePath: "coverage.xlsx", WorkspacePath: space, Policy: ingest.DefaultPolicy()}, id)
			if err != nil || got.Result.Status != ingest.StatusBlocked || len(got.Result.Warnings) != 1 || got.Result.Warnings[0] != "source_format_invalid" || len(got.Document) > 0 || len(got.Markdown) > 0 {
				t.Fatalf("unchecked consumed input reached authority: %+v %v", got.Result, err)
			}
		})
	}
}

func TestXLSXCanonicalOPCReachesAuthentication(t *testing.T) {
	inferred := workbookFixture(t, worksheetFixture("", `<row><c><v>11</v></c><c><v>12</v></c></row><row r="3"><c r="D3"><v>31</v></c><c><v>32</v></c></row>`, nil, nil))
	linked := workbookFixture(t, worksheetFixture("", `<row r="1"><c r="A1" t="inlineStr"><is><t>LINK_ANCHOR</t></is></c></row>`, nil, []string{"A1:A4"}))
	for _, test := range []struct {
		name string
		body []byte
	}{
		{"ordinary retained fixture", read(t, "testdata/policy/sheets.xlsx")},
		{"empty retained fixture", read(t, "testdata/policy/empty.xlsx")},
		{"inferred retained shape", inferred},
		{"hyperlink retained shape", linked},
		{"absolute canonical targets", editXLSX(t, linked, func(p map[string]string) {
			p["xl/_rels/workbook.xml.rels"] = strings.ReplaceAll(p["xl/_rels/workbook.xml.rels"], `Target="worksheets/`, `Target="/xl/worksheets/`)
		})},
	} {
		t.Run(test.name, func(t *testing.T) {
			r, _, space, id := unseededPolicyWorkspace(t)
			put(t, filepath.Join(space, "canonical.xlsx"), test.body)
			got, err := r.Convert(context.Background(), ingest.Request{SourcePath: "canonical.xlsx", WorkspacePath: space, Policy: ingest.DefaultPolicy()}, id)
			if err != nil || got.Result.Status != ingest.StatusUnavailable || len(got.Result.Warnings) != 1 || got.Result.Warnings[0] != "authority_unavailable" || len(got.Document) > 0 || len(got.Markdown) > 0 {
				t.Fatalf("canonical OPC was not admitted: %+v %v", got.Result, err)
			}
		})
	}
}

func TestXLSXAmbiguousOPCRefusesBeforeAuthentication(t *testing.T) {
	base := workbookFixture(t, worksheetFixture("A1", `<row r="1"><c r="A1"><v>1</v></c></row>`, nil, nil))
	for _, test := range []struct {
		name string
		edit func(map[string]string)
	}{
		{"default workbook fallback", func(p map[string]string) {
			p["[Content_Types].xml"] = strings.ReplaceAll(p["[Content_Types].xml"], `ContentType="application/xml"`, `ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"`)
		}},
		{"higher priority workbook override", func(p map[string]string) {
			p["xl/template.xml"] = p["xl/workbook.xml"]
			p["xl/_rels/template.xml.rels"] = p["xl/_rels/workbook.xml.rels"]
			p["[Content_Types].xml"] = strings.ReplaceAll(p["[Content_Types].xml"], `</Types>`, `<Override PartName="/xl/template.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.template.main+xml"/></Types>`)
		}},
		{"duplicate relationship id", func(p map[string]string) {
			p["xl/_rels/workbook.xml.rels"] = strings.ReplaceAll(p["xl/_rels/workbook.xml.rels"], `Id="rId2"`, `Id="rId1"`)
		}},
		{"same sheet occurrence twice", func(p map[string]string) {
			p["xl/workbook.xml"] = strings.ReplaceAll(p["xl/workbook.xml"], `r:id="rId2"`, `r:id="rId1"`)
		}},
		{"missing selected worksheet", func(p map[string]string) { delete(p, "xl/worksheets/sheet1.xml") }},
		{"missing workbook relationships", func(p map[string]string) { delete(p, "xl/_rels/workbook.xml.rels") }},
		{"foreign sheets container", func(p map[string]string) {
			p["xl/workbook.xml"] = strings.ReplaceAll(strings.ReplaceAll(p["xl/workbook.xml"], `<sheets>`, `<q:sheets xmlns:q="urn:other">`), `</sheets>`, `</q:sheets>`)
		}},
		{"foreign sheet child", func(p map[string]string) {
			p["xl/workbook.xml"] = strings.ReplaceAll(p["xl/workbook.xml"], `<sheet name=`, `<q:sheet xmlns:q="urn:other" name=`)
		}},
		{"foreign relationship child", func(p map[string]string) {
			p["xl/_rels/workbook.xml.rels"] = strings.ReplaceAll(p["xl/_rels/workbook.xml.rels"], `<Relationship Id=`, `<q:Relationship xmlns:q="urn:other" Id=`)
		}},
		{"external worksheet mode", func(p map[string]string) {
			p["xl/_rels/workbook.xml.rels"] = strings.ReplaceAll(p["xl/_rels/workbook.xml.rels"], `Id="rId1"`, `Id="rId1" TargetMode="External"`)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			r, _, space, id := unseededPolicyWorkspace(t)
			put(t, filepath.Join(space, "ambiguous.xlsx"), editXLSX(t, base, test.edit))
			got, err := r.Convert(context.Background(), ingest.Request{SourcePath: "ambiguous.xlsx", WorkspacePath: space, Policy: ingest.DefaultPolicy()}, id)
			if err != nil || got.Result.Status != ingest.StatusBlocked || len(got.Result.Warnings) != 1 || got.Result.Warnings[0] != "source_format_invalid" || len(got.Document) > 0 || len(got.Markdown) > 0 {
				t.Fatalf("ambiguous OPC reached authority: %+v %v", got.Result, err)
			}
		})
	}
}
