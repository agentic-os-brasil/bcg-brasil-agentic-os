package docling

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strconv"
	"strings"
)

const (
	xlsxNamespace = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
	// Pinned Docling 2.145.0 _RangeBudget counts area minus one anchor,
	// shared by every worksheet. Refuse instead of silently clipping it.
	maximumAdditionalMergedCells    = 100000
	maximumAdditionalHyperlinkCells = 10000
	xlsxValue                       = 1
	xlsxMerged                      = 2
	xlsxVisited                     = 4
)

type xlsxWorkbookBudget struct {
	inputCells, positions, projected int64
	merged, hyperlinks               int64
}
type xlsxPoint struct{ row, col int }
type xlsxRegion struct{ first, last xlsxPoint }

func (r xlsxRegion) area() int64 {
	return int64(r.last.row-r.first.row+1) * int64(r.last.col-r.first.col+1)
}

func xlsxAttribute(attrs []xml.Attr, name string) (string, bool) {
	for _, a := range attrs {
		if a.Name.Space == "" && a.Name.Local == name {
			return a.Value, true
		}
	}
	return "", false
}
func xlsxReference(value string) (xlsxRegion, error) {
	parts := strings.Split(value, ":")
	if len(parts) > 2 {
		return xlsxRegion{}, errors.New("worksheet_range")
	}
	r, c, err := cellPosition(parts[0])
	if err != nil {
		return xlsxRegion{}, err
	}
	region := xlsxRegion{xlsxPoint{r, c}, xlsxPoint{r, c}}
	if len(parts) == 2 {
		r, c, err = cellPosition(parts[1])
		if err != nil {
			return xlsxRegion{}, err
		}
		region.last = xlsxPoint{r, c}
	}
	if region.last.row < region.first.row || region.last.col < region.first.col {
		return xlsxRegion{}, errors.New("worksheet_range")
	}
	return region, nil
}

// validateWorksheet follows bounded SpreadsheetML coordinate inference, then
// bounds connected value/range rectangles without allocating those rectangles.
// Separate sparse tables stay separate; hyperlink-created values and blank
// merged positions conservatively participate in the connectivity check.
func validateWorksheet(data []byte, budget *xlsxWorkbookBudget) error {
	d := xml.NewDecoder(bytes.NewReader(data))
	nodes := 0
	stack := []xml.Name{}
	points := map[xlsxPoint]uint8{}
	add := func(p xlsxPoint, flag uint8) error {
		if _, exists := points[p]; !exists && int64(len(points)) >= maximumFormatItems-budget.positions {
			return errors.New("worksheet_positions")
		}
		points[p] |= flag
		return nil
	}
	type effect struct {
		region xlsxRegion
		flag   uint8
	}
	ranges := []effect{}
	row, column, rowDepth, cellDepth, valueDepth := 0, 0, 0, 0, 0
	var cell xlsxPoint
	hasValue, rootSeen := false, false
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
			return errors.New("worksheet_nodes")
		}
		switch v := token.(type) {
		case xml.Directive:
			return errors.New("worksheet_directive")
		case xml.CharData:
			if valueDepth != 0 && len(v) > 0 {
				hasValue = true
			}
		case xml.StartElement:
			parent := xml.Name{}
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			stack = append(stack, v.Name)
			depth := len(stack)
			if depth > 128 {
				return errors.New("worksheet_depth")
			}
			if depth == 1 {
				if rootSeen || v.Name.Space != xlsxNamespace || v.Name.Local != "worksheet" {
					return errors.New("worksheet_root")
				}
				rootSeen = true
			}
			// openpyxl parses every direct row child as a cell. Refuse
			// lookalike/extension children rather than leaving geometry unchecked.
			if rowDepth != 0 && depth == rowDepth+1 && (v.Name.Space != xlsxNamespace || v.Name.Local != "c") {
				return errors.New("worksheet_cell")
			}
			if v.Name.Space != xlsxNamespace {
				// Serialisable.from_tree resolves range children by localname.
				// Refuse lookalikes rather than leaving downstream ranges unchecked.
				if v.Name.Local == "mergeCell" || v.Name.Local == "hyperlink" {
					return errors.New("worksheet_range_namespace")
				}
				continue
			}
			switch v.Name.Local {
			case "row":
				if rowDepth != 0 || parent.Space != xlsxNamespace || parent.Local != "sheetData" {
					return errors.New("worksheet_row")
				}
				if text, explicit := xlsxAttribute(v.Attr, "r"); explicit {
					row, err = strconv.Atoi(text)
					if err != nil {
						return errors.New("worksheet_row")
					}
				} else {
					row++
				}
				if row < 1 || row > 1048576 {
					return errors.New("worksheet_row")
				}
				column = 0
				rowDepth = depth
			case "c":
				if rowDepth == 0 || depth != rowDepth+1 || cellDepth != 0 {
					return errors.New("worksheet_cell")
				}
				budget.inputCells++
				if budget.inputCells > maximumFormatItems {
					return errors.New("worksheet_cells")
				}
				if reference, explicit := xlsxAttribute(v.Attr, "r"); explicit {
					r, c, err := cellPosition(reference)
					if err != nil {
						return err
					}
					cell = xlsxPoint{r, c}
					column = c
				} else {
					column++
					if column > 16384 {
						return errors.New("worksheet_reference")
					}
					cell = xlsxPoint{row, column}
				}
				cellDepth = depth
				hasValue = false
			case "v":
				if cellDepth != 0 && depth == cellDepth+1 {
					valueDepth = depth
				}
			case "is":
				if cellDepth != 0 && depth == cellDepth+1 {
					hasValue = true
				}
			case "dimension", "mergeCell", "hyperlink":
				ref, exists := xlsxAttribute(v.Attr, "ref")
				if !exists {
					return errors.New("worksheet_reference")
				}
				region, err := xlsxReference(ref)
				if err != nil {
					return err
				}
				if v.Name.Local == "dimension" {
					if region.area() > maximumFormatItems {
						return errors.New("worksheet_range")
					}
					continue
				}
				additional := region.area() - 1
				if v.Name.Local == "mergeCell" {
					if additional > maximumAdditionalMergedCells-budget.merged {
						return errors.New("worksheet_merge_budget")
					}
					budget.merged += additional
					ranges = append(ranges, effect{region, xlsxMerged})
				} else {
					if additional > maximumAdditionalHyperlinkCells-budget.hyperlinks {
						return errors.New("worksheet_hyperlink_budget")
					}
					budget.hyperlinks += additional
					ranges = append(ranges, effect{region, xlsxValue})
				}
			}
		case xml.EndElement:
			depth := len(stack)
			if depth == cellDepth && cellDepth != 0 {
				if hasValue {
					if err := add(cell, xlsxValue); err != nil {
						return err
					}
				}
				cellDepth = 0
				hasValue = false
			}
			if depth == valueDepth {
				valueDepth = 0
			}
			if depth == rowDepth {
				rowDepth = 0
			}
			if depth == 0 {
				return errors.New("worksheet_root")
			}
			stack = stack[:depth-1]
		}
	}
	if !rootSeen || len(stack) != 0 || cellDepth != 0 || rowDepth != 0 {
		return errors.New("worksheet_root")
	}
	for _, e := range ranges {
		for r := e.region.first.row; r <= e.region.last.row; r++ {
			for c := e.region.first.col; c <= e.region.last.col; c++ {
				if err := add(xlsxPoint{r, c}, e.flag); err != nil {
					return err
				}
			}
		}
	}
	budget.positions += int64(len(points))
	projected, err := xlsxProjectedCells(points, maximumFormatItems-budget.projected)
	if err != nil {
		return err
	}
	budget.projected += projected
	return nil
}

func xlsxProjectedCells(points map[xlsxPoint]uint8, remaining int64) (int64, error) {
	var total int64
	for start, flags := range points {
		if flags&xlsxValue == 0 || flags&xlsxVisited != 0 {
			continue
		}
		region := xlsxRegion{start, start}
		queue := []xlsxPoint{start}
		points[start] |= xlsxVisited
		for next := 0; next < len(queue); next++ {
			p := queue[next]
			if p.row < region.first.row {
				region.first.row = p.row
			}
			if p.col < region.first.col {
				region.first.col = p.col
			}
			if p.row > region.last.row {
				region.last.row = p.row
			}
			if p.col > region.last.col {
				region.last.col = p.col
			}
			if region.area() > remaining-total {
				return 0, errors.New("worksheet_geometry")
			}
			for _, neighbor := range []xlsxPoint{{p.row - 1, p.col}, {p.row + 1, p.col}, {p.row, p.col - 1}, {p.row, p.col + 1}} {
				flags := points[neighbor]
				if flags&(xlsxValue|xlsxMerged) == 0 || flags&xlsxVisited != 0 {
					continue
				}
				points[neighbor] = flags | xlsxVisited
				queue = append(queue, neighbor)
			}
		}
		total += region.area()
	}
	return total, nil
}
