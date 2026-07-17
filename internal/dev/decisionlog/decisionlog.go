package decisionlog

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	headingPattern = regexp.MustCompile(`^## ([A-Z]{4}) - (.+)$`)
	codePattern    = regexp.MustCompile(`^[A-Z]{4}$`)
)

var requiredFields = []string{
	"Date", "Status", "Owner", "Context", "Decision", "Consequences", "Refs", "Supersedes",
}

var allowedStatuses = map[string]bool{
	"proposed": true, "accepted": true, "superseded": true, "rejected": true,
}

// Entry is one durable project decision.
type Entry struct {
	Code   string
	Title  string
	Fields map[string]string
	Line   int
}

// Parse reads and validates the project decision log.
func Parse(r io.Reader) ([]Entry, error) {
	var entries []Entry
	var current *Entry
	var problems []error
	seen := make(map[string]int)

	scanner := bufio.NewScanner(r)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())

		if matches := headingPattern.FindStringSubmatch(line); matches != nil {
			if current != nil {
				problems = append(problems, validateEntry(*current)...)
				entries = append(entries, *current)
			}
			current = &Entry{Code: matches[1], Title: matches[2], Fields: make(map[string]string), Line: lineNumber}
			if previousLine, exists := seen[current.Code]; exists {
				problems = append(problems, fmt.Errorf("line %d: duplicate decision %s (first seen at line %d)", lineNumber, current.Code, previousLine))
			}
			seen[current.Code] = lineNumber
			continue
		}

		if current == nil || !strings.HasPrefix(line, "- ") {
			continue
		}
		parts := strings.SplitN(strings.TrimPrefix(line, "- "), ":", 2)
		if len(parts) != 2 {
			problems = append(problems, fmt.Errorf("line %d: malformed field in %s", lineNumber, current.Code))
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if _, exists := current.Fields[key]; exists {
			problems = append(problems, fmt.Errorf("line %d: duplicate field %q in %s", lineNumber, key, current.Code))
			continue
		}
		current.Fields[key] = value
	}

	if err := scanner.Err(); err != nil {
		problems = append(problems, fmt.Errorf("read decision log: %w", err))
	}
	if current != nil {
		problems = append(problems, validateEntry(*current)...)
		entries = append(entries, *current)
	}
	if len(entries) == 0 {
		problems = append(problems, errors.New("decision log contains no four-letter entries"))
	}
	problems = append(problems, validateSupersedes(entries)...)
	return entries, errors.Join(problems...)
}

func validateSupersedes(entries []Entry) []error {
	known := make(map[string]bool, len(entries))
	for _, entry := range entries {
		known[entry.Code] = true
	}
	var problems []error
	for _, entry := range entries {
		target := entry.Fields["Supersedes"]
		if target == "" || target == "none" {
			continue
		}
		if !codePattern.MatchString(target) {
			problems = append(problems, fmt.Errorf("line %d: %s Supersedes value %q must be one four-letter code or none", entry.Line, entry.Code, target))
			continue
		}
		if target == entry.Code {
			problems = append(problems, fmt.Errorf("line %d: %s cannot supersede itself", entry.Line, entry.Code))
			continue
		}
		if !known[target] {
			problems = append(problems, fmt.Errorf("line %d: %s supersedes unknown decision %s", entry.Line, entry.Code, target))
		}
	}
	return problems
}

// ParseFile opens and validates a decision log.
func ParseFile(path string) ([]Entry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open decision log: %w", err)
	}
	defer file.Close()
	return Parse(file)
}

// Available checks that code is structurally valid and not already in use.
func Available(entries []Entry, code string) error {
	if !codePattern.MatchString(code) {
		return fmt.Errorf("decision code %q must contain exactly four uppercase letters", code)
	}
	for _, entry := range entries {
		if entry.Code == code {
			return fmt.Errorf("decision code %s is already used by %q", code, entry.Title)
		}
	}
	return nil
}

func validateEntry(entry Entry) []error {
	var problems []error
	for _, field := range requiredFields {
		if strings.TrimSpace(entry.Fields[field]) == "" {
			problems = append(problems, fmt.Errorf("line %d: %s missing required field %q", entry.Line, entry.Code, field))
		}
	}
	if status := entry.Fields["Status"]; status != "" && !allowedStatuses[status] {
		statuses := make([]string, 0, len(allowedStatuses))
		for allowed := range allowedStatuses {
			statuses = append(statuses, allowed)
		}
		sort.Strings(statuses)
		problems = append(problems, fmt.Errorf("line %d: %s status %q must be one of %s", entry.Line, entry.Code, status, strings.Join(statuses, ", ")))
	}
	if date := entry.Fields["Date"]; date != "" {
		if _, err := time.Parse("2006-01-02", date); err != nil {
			problems = append(problems, fmt.Errorf("line %d: %s date %q must use YYYY-MM-DD", entry.Line, entry.Code, date))
		}
	}
	return problems
}
