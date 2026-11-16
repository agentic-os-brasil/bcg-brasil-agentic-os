package docling

import (
	"encoding/xml"
	"errors"
	"path"
	"strings"
)

const pptxNamespace = "http://schemas.openxmlformats.org/presentationml/2006/main"
const pptxPresentationType = "application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"
const pptxSlideType = "application/vnd.openxmlformats-officedocument.presentationml.slide+xml"

type pptxRelation struct{ target, kind string }

// Admit a unique modern OPC presentation, with ordered, uniquely selected slide
// parts. External relationships and active/legacy renderers are outside this profile.
func validatePPTXParts(parts map[string][]byte) error {
	types := map[string]string{}
	mains := 0
	err := walkXLSXPart(parts["[Content_Types].xml"], xml.Name{Space: xlsxContentNamespace, Local: "Types"}, func(v xml.StartElement, _ xml.Name, depth int) error {
		if depth == 1 {
			return nil
		}
		if depth != 2 || v.Name.Space != xlsxContentNamespace {
			return errors.New("pptx_manifest")
		}
		ct, _ := xlsxAttribute(v.Attr, "ContentType")
		if strings.Contains(strings.ToLower(ct), "macroenabled") || strings.Contains(strings.ToLower(ct), "vba") {
			return errors.New("pptx_active_content")
		}
		if v.Name.Local == "Default" {
			ext, _ := xlsxAttribute(v.Attr, "Extension")
			if ext == "" || ct == "" || ct == pptxPresentationType {
				return errors.New("pptx_manifest")
			}
			return nil
		}
		if v.Name.Local != "Override" {
			return errors.New("pptx_manifest")
		}
		name, _ := xlsxAttribute(v.Attr, "PartName")
		if !strings.HasPrefix(name, "/") || !xlsxCanonicalPart(name[1:]) || types[name] != "" || ct == "" {
			return errors.New("pptx_manifest")
		}
		types[name] = ct
		if ct == pptxPresentationType {
			mains++
			if name != "/ppt/presentation.xml" {
				return errors.New("pptx_presentation_selection")
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if mains != 1 {
		return errors.New("pptx_presentation_selection")
	}
	rels := map[string]map[string]pptxRelation{}
	for name, body := range parts {
		if !strings.HasSuffix(name, ".xml") && !strings.HasSuffix(name, ".rels") {
			continue
		}
		if strings.HasSuffix(name, ".xml") {
			if err := validateFormat(body, ".xml", int64(len(body))); err != nil {
				return errors.New("pptx_xml_invalid")
			}
			continue
		}
		current := map[string]pptxRelation{}
		err = walkXLSXPart(body, xml.Name{Space: xlsxPackageRelations, Local: "Relationships"}, func(v xml.StartElement, _ xml.Name, depth int) error {
			if depth == 1 {
				return nil
			}
			if depth != 2 || v.Name != (xml.Name{Space: xlsxPackageRelations, Local: "Relationship"}) {
				return errors.New("pptx_relationship")
			}
			id, _ := xlsxAttribute(v.Attr, "Id")
			target, _ := xlsxAttribute(v.Attr, "Target")
			kind, _ := xlsxAttribute(v.Attr, "Type")
			mode, _ := xlsxAttribute(v.Attr, "TargetMode")
			if id == "" || target == "" || kind == "" || mode != "" || strings.ContainsAny(target, "\\%?#:") {
				return errors.New("pptx_relationship")
			}
			if _, ok := current[id]; ok {
				return errors.New("pptx_relationship_duplicate")
			}
			base := path.Dir(path.Dir(name))
			if name == "_rels/.rels" {
				base = "."
			}
			resolved := path.Clean(path.Join(base, target))
			if strings.HasPrefix(target, "/") {
				resolved = strings.TrimPrefix(target, "/")
			}
			if !xlsxCanonicalPart(resolved) || parts[resolved] == nil {
				return errors.New("pptx_relationship_target")
			}
			current[id] = pptxRelation{resolved, kind}
			return nil
		})
		if err != nil {
			return err
		}
		rels[name] = current
	}
	office := 0
	for _, r := range rels["_rels/.rels"] {
		if r.kind == xlsxOfficeRelations+"/officeDocument" {
			office++
			if r.target != "ppt/presentation.xml" {
				return errors.New("pptx_root_selection")
			}
		}
	}
	if office != 1 {
		return errors.New("pptx_root_selection")
	}
	selected := map[string]bool{}
	ids := map[string]bool{}
	lists := 0
	err = walkXLSXPart(parts["ppt/presentation.xml"], xml.Name{Space: pptxNamespace, Local: "presentation"}, func(v xml.StartElement, parent xml.Name, depth int) error {
		if v.Name.Local == "sldIdLst" {
			if depth != 2 || v.Name.Space != pptxNamespace {
				return errors.New("pptx_slide_list")
			}
			lists++
		}
		if v.Name.Local != "sldId" {
			return nil
		}
		if depth != 3 || parent != (xml.Name{Space: pptxNamespace, Local: "sldIdLst"}) || v.Name.Space != pptxNamespace {
			return errors.New("pptx_slide_selection")
		}
		id, _ := xlsxAttribute(v.Attr, "id")
		rid := ""
		for _, a := range v.Attr {
			if a.Name == (xml.Name{Space: xlsxOfficeRelations, Local: "id"}) {
				rid = a.Value
			}
		}
		r, ok := rels["ppt/_rels/presentation.xml.rels"][rid]
		if !ok || id == "" || ids[id] || selected[r.target] || r.kind != xlsxOfficeRelations+"/slide" || !strings.HasPrefix(r.target, "ppt/slides/") || types["/"+r.target] != pptxSlideType {
			return errors.New("pptx_slide_selection")
		}
		ids[id] = true
		selected[r.target] = true
		return walkXLSXPart(parts[r.target], xml.Name{Space: pptxNamespace, Local: "sld"}, func(xml.StartElement, xml.Name, int) error { return nil })
	})
	if err != nil {
		return err
	}
	if lists != 1 || len(selected) == 0 {
		return errors.New("pptx_slide_selection")
	}
	for name, ct := range types {
		if ct == pptxSlideType && !selected[strings.TrimPrefix(name, "/")] {
			return errors.New("pptx_unselected_slide")
		}
	}
	return nil
}
