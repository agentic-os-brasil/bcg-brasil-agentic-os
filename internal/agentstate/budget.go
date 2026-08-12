package agentstate

// composedRuneLen computes the rune length of a rendered snapshot body given
// its ordered sections. Each section contributes its label, a separator and
// its body; the exact rendering is defined here so budget checks and
// on-disk rendering remain consistent.
func composedRuneLen(sections []Section) int {
	total := 0
	for index, section := range sections {
		if index > 0 {
			total += runeLen(sectionSeparator)
		}
		total += runeLen(sectionHeaderPrefix)
		total += runeLen(section.Label)
		total += runeLen(sectionHeaderSuffix)
		total += runeLen(section.Body)
	}
	return total
}

const (
	sectionSeparator    = "\n\n"
	sectionHeaderPrefix = "## "
	sectionHeaderSuffix = "\n"
)

// compact drops whole oldest sections until the composed rune length fits
// within budget. Ordering is preserved for the surviving sections; no
// individual section is truncated mid-body. If the newest single section is
// itself larger than the budget, an error is returned rather than a silent
// mid-section truncation.
func compact(sections []Section, budget int) ([]Section, error) {
	if budget <= 0 {
		return nil, errBudgetNonPositive
	}
	// Sections are assumed to be ordered oldest-first.
	current := sections
	for composedRuneLen(current) > budget {
		if len(current) <= 1 {
			return nil, errSingleSectionOverBudget
		}
		current = current[1:]
	}
	// Return a fresh slice so callers cannot mutate the store's copy.
	out := make([]Section, len(current))
	copy(out, current)
	return out, nil
}

// mergeSection appends or replaces a section with the given label. The
// caller enforces idempotency by comparing bodies before invoking this
// helper. Sections retain oldest-first ordering; a replaced section keeps
// its original position, while a new section is appended to the tail.
func mergeSection(sections []Section, incoming Section) []Section {
	for index, existing := range sections {
		if existing.Label == incoming.Label {
			out := make([]Section, len(sections))
			copy(out, sections)
			out[index] = incoming
			return out
		}
	}
	out := make([]Section, 0, len(sections)+1)
	out = append(out, sections...)
	out = append(out, incoming)
	return out
}

// renderBody produces the on-disk textual body for a snapshot. It matches
// the rendering assumed by composedRuneLen so budget accounting is exact.
func renderBody(sections []Section) string {
	if len(sections) == 0 {
		return ""
	}
	total := composedRuneLen(sections)
	// Preallocate a byte buffer roughly the size of the rendered runes; UTF-8
	// bodies may exceed this, so builders grow as needed.
	buf := make([]byte, 0, total)
	for index, section := range sections {
		if index > 0 {
			buf = append(buf, sectionSeparator...)
		}
		buf = append(buf, sectionHeaderPrefix...)
		buf = append(buf, section.Label...)
		buf = append(buf, sectionHeaderSuffix...)
		buf = append(buf, section.Body...)
	}
	return string(buf)
}
