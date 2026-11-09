package docling

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

const maximumFormatItems = 1000000

func validateFormat(body []byte, extension string, maximum int64) error {
	if len(body) == 0 {
		return errors.New("empty_source")
	}
	switch extension {
	case ".docx", ".xlsx", ".epub":
		return validateArchive(body, extension, maximum)
	case ".xls":
		if !bytes.HasPrefix(body, []byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1}) {
			return errors.New("biff_container_invalid")
		}
		return nil
	}
	if !utf8.Valid(body) || bytes.IndexByte(body, 0) >= 0 {
		return errors.New("utf8_required")
	}
	switch extension {
	case ".json":
		d := json.NewDecoder(bytes.NewReader(body))
		d.UseNumber()
		depth, count := 0, 0
		for {
			token, err := d.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			count++
			if count > maximumFormatItems {
				return errors.New("json_nodes")
			}
			if delim, ok := token.(json.Delim); ok {
				if delim == '{' || delim == '[' {
					depth++
				} else {
					depth--
				}
				if depth > 128 {
					return errors.New("json_depth")
				}
			}
		}
		if !json.Valid(body) {
			return errors.New("json_invalid")
		}
	case ".xml":
		d := xml.NewDecoder(bytes.NewReader(body))
		depth, count, roots := 0, 0, 0
		for {
			token, err := d.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			count++
			if count > maximumFormatItems {
				return errors.New("xml_nodes")
			}
			switch v := token.(type) {
			case xml.Directive:
				return errors.New("xml_directive")
			case xml.StartElement:
				if depth == 0 {
					roots++
				}
				depth++
				if depth > 128 {
					return errors.New("xml_depth")
				}
			case xml.EndElement:
				depth--
			case xml.CharData:
				if depth == 0 && strings.TrimSpace(string(v)) != "" {
					return errors.New("xml_outside_root")
				}
			}
		}
		if roots != 1 || depth != 0 {
			return errors.New("xml_root")
		}
	case ".csv":
		r := csv.NewReader(bytes.NewReader(body))
		r.FieldsPerRecord = -1
		count := 0
		for {
			row, err := r.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			count += len(row)
			if count > maximumFormatItems {
				return errors.New("csv_cells")
			}
		}
	case ".html", ".htm", ".md", ".txt":
	default:
		return errors.New("format_unsupported")
	}
	return nil
}

func validateDOCX(body []byte, maximum int64) error { return validateArchive(body, ".docx", maximum) }

func validateArchive(body []byte, extension string, maximum int64) error {
	a, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return err
	}
	if len(a.File) > 4096 {
		return errors.New("zip_members")
	}
	seen := map[string]bool{}
	var total uint64
	xlsxParts := map[string][]byte{}
	for _, f := range a.File {
		name := strings.TrimSuffix(f.Name, "/")
		key := strings.ToLower(name)
		if !relativeName(name) || seen[key] || f.Mode()&os.ModeSymlink != 0 || (!f.FileInfo().IsDir() && !f.Mode().IsRegular()) {
			return errors.New("zip_alias")
		}
		seen[key] = true
		if f.FileInfo().IsDir() {
			continue
		}
		if strings.HasSuffix(key, ".emf") || strings.HasSuffix(key, ".wmf") || strings.HasSuffix(key, "vbaproject.bin") {
			return errors.New("unmanaged_embedded_format")
		}
		if f.UncompressedSize64 > uint64(maximum) || total > uint64(maximum)-f.UncompressedSize64 {
			return errors.New("zip_expansion")
		}
		total += f.UncompressedSize64
		r, err := f.Open()
		if err != nil {
			return err
		}
		data, readErr := io.ReadAll(io.LimitReader(r, int64(f.UncompressedSize64)+1))
		closeErr := r.Close()
		if readErr != nil || closeErr != nil || uint64(len(data)) != f.UncompressedSize64 {
			return errors.New("zip_member_invalid")
		}
		if strings.HasSuffix(key, ".xml") || strings.HasSuffix(key, ".rels") || strings.HasSuffix(key, ".opf") || strings.HasSuffix(key, ".xhtml") {
			upper := bytes.ToUpper(data)
			if !utf8.Valid(data) || bytes.Contains(upper, []byte("<!ENTITY")) || (extension != ".epub" && bytes.Contains(upper, []byte("<!DOCTYPE"))) {
				return errors.New("zip_xml_unsafe")
			}
		}
		if extension == ".xlsx" {
			xlsxParts[f.Name] = data
		}
	}
	switch extension {
	case ".docx":
		if !seen["[content_types].xml"] || !seen["word/document.xml"] {
			return errors.New("docx_missing_parts")
		}
	case ".xlsx":
		return validateXLSXParts(xlsxParts)
	case ".epub":
		if !seen["mimetype"] || !seen["meta-inf/container.xml"] {
			return errors.New("epub_missing_parts")
		}
	}
	return nil
}

var cellReference = regexp.MustCompile(`^([A-Z]{1,3})([1-9][0-9]{0,6})$`)

func cellPosition(reference string) (int, int, error) {
	m := cellReference.FindStringSubmatch(reference)
	if m == nil {
		return 0, 0, errors.New("worksheet_reference")
	}
	column := 0
	for _, letter := range m[1] {
		column = column*26 + int(letter-'A'+1)
	}
	row, _ := strconv.Atoi(m[2])
	if column > 16384 || row > 1048576 {
		return 0, 0, errors.New("worksheet_reference")
	}
	return row, column, nil
}
