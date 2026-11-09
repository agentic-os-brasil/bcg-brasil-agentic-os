package docling

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"path"
	"strings"
	"unicode/utf8"
)

const (
	xlsxContentNamespace = "http://schemas.openxmlformats.org/package/2006/content-types"
	xlsxPackageRelations = "http://schemas.openxmlformats.org/package/2006/relationships"
	xlsxOfficeRelations  = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	xlsxWorkbookType     = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"
	xlsxWorksheetType    = "application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"
)

// The supported OPC profile is explicit: one canonical XLSX workbook and unique
// canonical worksheet targets. Refusing other layouts prevents a guessed ZIP
// filename from standing in for the actual manifest/relationship-selected input.
func validateXLSXParts(parts map[string][]byte) error {
	types := map[string]string{}
	defaults := map[string]bool{}
	workbooks := 0
	err := walkXLSXPart(parts["[Content_Types].xml"], xml.Name{Space: xlsxContentNamespace, Local: "Types"}, func(v xml.StartElement, parent xml.Name, depth int) error {
		if depth == 1 {
			return nil
		}
		if depth != 2 || v.Name.Space != xlsxContentNamespace {
			return errors.New("xlsx_manifest")
		}
		ct, _ := xlsxAttribute(v.Attr, "ContentType")
		isWorkbook := ct == xlsxWorkbookType || ct == "application/vnd.ms-excel.sheet.macroEnabled.main+xml" || ct == "application/vnd.openxmlformats-officedocument.spreadsheetml.template.main+xml" || ct == "application/vnd.ms-excel.template.macroEnabled.main+xml"
		switch v.Name.Local {
		case "Default":
			ext, _ := xlsxAttribute(v.Attr, "Extension")
			if ext == "" || ct == "" || defaults[ext] || isWorkbook {
				return errors.New("xlsx_manifest_default")
			}
			defaults[ext] = true
		case "Override":
			name, _ := xlsxAttribute(v.Attr, "PartName")
			if !strings.HasPrefix(name, "/") || !xlsxCanonicalPart(name[1:]) || ct == "" || types[name] != "" {
				return errors.New("xlsx_manifest_override")
			}
			types[name] = ct
			if isWorkbook {
				workbooks++
				if name != "/xl/workbook.xml" || ct != xlsxWorkbookType {
					return errors.New("xlsx_workbook_selection")
				}
			}
		default:
			return errors.New("xlsx_manifest_child")
		}
		return nil
	})
	if err != nil {
		return err
	}
	if workbooks != 1 {
		return errors.New("xlsx_workbook_selection")
	}

	type relation struct{ target, kind, mode string }
	rels := map[string]relation{}
	err = walkXLSXPart(parts["xl/_rels/workbook.xml.rels"], xml.Name{Space: xlsxPackageRelations, Local: "Relationships"}, func(v xml.StartElement, parent xml.Name, depth int) error {
		if depth == 1 {
			return nil
		}
		if depth != 2 || v.Name != (xml.Name{Space: xlsxPackageRelations, Local: "Relationship"}) {
			return errors.New("xlsx_relationship")
		}
		id, _ := xlsxAttribute(v.Attr, "Id")
		target, _ := xlsxAttribute(v.Attr, "Target")
		kind, _ := xlsxAttribute(v.Attr, "Type")
		mode, _ := xlsxAttribute(v.Attr, "TargetMode")
		if id == "" || target == "" || kind == "" {
			return errors.New("xlsx_relationship")
		}
		if _, exists := rels[id]; exists {
			return errors.New("xlsx_relationship_duplicate")
		}
		for _, a := range v.Attr {
			if a.Name.Space == "xmlns" || a.Name.Local == "xmlns" {
				continue
			}
			if a.Name.Space != "" || (a.Name.Local != "Id" && a.Name.Local != "Target" && a.Name.Local != "Type" && a.Name.Local != "TargetMode") {
				return errors.New("xlsx_relationship_attribute")
			}
		}
		rels[id] = relation{target, kind, mode}
		return nil
	})
	if err != nil {
		return err
	}

	targets := []string{}
	usedIDs, usedTargets := map[string]bool{}, map[string]bool{}
	sheetContainers := 0
	err = walkXLSXPart(parts["xl/workbook.xml"], xml.Name{Space: xlsxNamespace, Local: "workbook"}, func(v xml.StartElement, parent xml.Name, depth int) error {
		if depth == 1 {
			return nil
		}
		if depth == 2 && v.Name.Space != xlsxNamespace {
			return errors.New("xlsx_workbook_namespace")
		}
		if v.Name.Local == "sheets" {
			if depth != 2 || v.Name.Space != xlsxNamespace {
				return errors.New("xlsx_sheets")
			}
			sheetContainers++
			if sheetContainers != 1 {
				return errors.New("xlsx_sheets_duplicate")
			}
		}
		if parent == (xml.Name{Space: xlsxNamespace, Local: "sheets"}) || v.Name.Local == "sheet" {
			if depth != 3 || parent != (xml.Name{Space: xlsxNamespace, Local: "sheets"}) || v.Name != (xml.Name{Space: xlsxNamespace, Local: "sheet"}) {
				return errors.New("xlsx_sheet")
			}
			id := ""
			for _, a := range v.Attr {
				if a.Name.Space == xlsxOfficeRelations && a.Name.Local == "id" {
					id = a.Value
				} else if a.Name.Local == "id" {
					return errors.New("xlsx_sheet_id")
				}
			}
			r, exists := rels[id]
			if id == "" || usedIDs[id] || !exists || r.kind != xlsxOfficeRelations+"/worksheet" || (r.mode != "" && r.mode != "Internal") {
				return errors.New("xlsx_sheet_relationship")
			}
			target := "xl/" + r.target
			if strings.HasPrefix(r.target, "/") {
				target = r.target[1:]
			}
			if !xlsxCanonicalPart(target) || !strings.HasPrefix(target, "xl/worksheets/") || !strings.HasSuffix(target, ".xml") || types["/"+target] != xlsxWorksheetType || parts[target] == nil || usedTargets[target] {
				return errors.New("xlsx_sheet_target")
			}
			usedIDs[id], usedTargets[target] = true, true
			targets = append(targets, target)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if sheetContainers != 1 || len(targets) == 0 {
		return errors.New("xlsx_sheets")
	}
	var budget xlsxWorkbookBudget
	for _, target := range targets {
		if err := validateWorksheet(parts[target], &budget); err != nil {
			return err
		}
	}
	return nil
}

func xlsxCanonicalPart(name string) bool {
	return relativeName(name) && path.Clean(name) == name && !strings.ContainsAny(name, "\\%?#")
}

// Bounded XML inspection for the three OPC selection parts. It checks the same
// byte/depth/token envelope before callbacks see namespace-sensitive structure.
func walkXLSXPart(data []byte, root xml.Name, visit func(xml.StartElement, xml.Name, int) error) error {
	if len(data) == 0 || !utf8.Valid(data) {
		return errors.New("xlsx_part_missing")
	}
	d := xml.NewDecoder(bytes.NewReader(data))
	stack := []xml.Name{}
	seen, nodes := false, 0
	for {
		token, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		nodes++
		if nodes > maximumFormatItems {
			return errors.New("xlsx_part_nodes")
		}
		switch v := token.(type) {
		case xml.Directive:
			return errors.New("xlsx_part_directive")
		case xml.CharData:
			if len(stack) == 0 && strings.TrimSpace(string(v)) != "" {
				return errors.New("xlsx_part_text")
			}
		case xml.StartElement:
			parent := xml.Name{}
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			if len(stack) == 0 {
				if seen || v.Name != root {
					return errors.New("xlsx_part_root")
				}
				seen = true
			}
			stack = append(stack, v.Name)
			if len(stack) > 128 {
				return errors.New("xlsx_part_depth")
			}
			attrs := map[xml.Name]bool{}
			for _, a := range v.Attr {
				if attrs[a.Name] {
					return errors.New("xlsx_part_attribute")
				}
				attrs[a.Name] = true
			}
			if err := visit(v, parent, len(stack)); err != nil {
				return err
			}
		case xml.EndElement:
			if len(stack) == 0 {
				return errors.New("xlsx_part_root")
			}
			stack = stack[:len(stack)-1]
		}
	}
	if !seen || len(stack) != 0 {
		return errors.New("xlsx_part_root")
	}
	return nil
}
