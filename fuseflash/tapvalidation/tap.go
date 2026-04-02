// Package tapvalidation implements a parser and validator for the Test Anything
// Protocol (TAP) output format, as used by test harnesses across many languages.
// See https://testanything.org/ for the specification.
package tapvalidation

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// Version is the TAP version reported in a TAP header line ("TAP version 13").
const Version = 13

// Result represents the outcome of a single TAP test point.
type Result struct {
	Number      int
	OK          bool
	Description string
	Directive   string // SKIP or TODO, if present
	Reason      string // reason following the directive
}

// IsSkip reports whether the result carries a SKIP directive.
func (r Result) IsSkip() bool { return strings.EqualFold(r.Directive, "SKIP") }

// IsTodo reports whether the result carries a TODO directive.
func (r Result) IsTodo() bool { return strings.EqualFold(r.Directive, "TODO") }

// Report is the complete result of parsing a TAP stream.
type Report struct {
	Version  int
	Plan     Plan
	Results  []Result
	BailOut  bool
	BailText string
}

// Plan holds the expected test count from the TAP plan line ("1..N").
type Plan struct {
	From int
	To   int
	set  bool
}

// PassCount returns the number of passing (ok) results that are not SKIP/TODO.
func (r *Report) PassCount() int {
	n := 0
	for _, res := range r.Results {
		if res.OK && !res.IsSkip() && !res.IsTodo() {
			n++
		}
	}
	return n
}

// FailCount returns the number of failing (not ok) results that are not TODO.
func (r *Report) FailCount() int {
	n := 0
	for _, res := range r.Results {
		if !res.OK && !res.IsTodo() {
			n++
		}
	}
	return n
}

// Validate checks the report for structural errors:
//   - plan must be present
//   - actual test count must match the plan
//   - no bail-out may be present
//   - no failing non-TODO tests
func (r *Report) Validate() error {
	if r.BailOut {
		return fmt.Errorf("tap: bail out: %s", r.BailText)
	}
	if !r.Plan.set {
		return errors.New("tap: missing plan line")
	}
	expected := r.Plan.To - r.Plan.From + 1
	if len(r.Results) != expected {
		return fmt.Errorf("tap: plan says %d tests, got %d", expected, len(r.Results))
	}
	if fc := r.FailCount(); fc > 0 {
		return fmt.Errorf("tap: %d test(s) failed", fc)
	}
	return nil
}

var (
	rePlan      = regexp.MustCompile(`^(\d+)\.\.(\d+)$`)
	reTestPoint = regexp.MustCompile(`^(ok|not ok)\s*(\d+)?\s*([^#]*)?\s*(?:#\s*(.*))?$`)
	reDirective = regexp.MustCompile(`(?i)^(SKIP|TODO)\s*(.*)$`)
	reVersion   = regexp.MustCompile(`^TAP version (\d+)$`)
	reBailOut   = regexp.MustCompile(`^Bail out!\s*(.*)$`)
)

// Parse reads a TAP stream from r and returns a Report.
func Parse(r io.Reader) (*Report, error) {
	report := &Report{}
	scanner := bufio.NewScanner(r)
	lineNum := 0
	nextTest := 1

	for scanner.Scan() {
		line := scanner.Text()
		lineNum++

		// Version header
		if m := reVersion.FindStringSubmatch(line); m != nil {
			v, _ := strconv.Atoi(m[1])
			report.Version = v
			continue
		}

		// Bail out
		if m := reBailOut.FindStringSubmatch(line); m != nil {
			report.BailOut = true
			report.BailText = m[1]
			return report, nil
		}

		// Plan
		if m := rePlan.FindStringSubmatch(line); m != nil {
			from, _ := strconv.Atoi(m[1])
			to, _ := strconv.Atoi(m[2])
			report.Plan = Plan{From: from, To: to, set: true}
			continue
		}

		// Test point
		if m := reTestPoint.FindStringSubmatch(line); m != nil {
			res := Result{
				OK:          m[1] == "ok",
				Description: strings.TrimSpace(m[3]),
			}
			if m[2] != "" {
				num, _ := strconv.Atoi(m[2])
				res.Number = num
			} else {
				res.Number = nextTest
			}
			nextTest = res.Number + 1

			// Parse directive
			if m[4] != "" {
				if dm := reDirective.FindStringSubmatch(strings.TrimSpace(m[4])); dm != nil {
					res.Directive = strings.ToUpper(dm[1])
					res.Reason = dm[2]
				}
			}
			report.Results = append(report.Results, res)
			continue
		}

		// Diagnostic lines (# ...) and YAML blocks are valid TAP but are
		// intentionally not captured — they do not affect validation.
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("tap parse: %w", err)
	}
	return report, nil
}

// ParseString is a convenience wrapper around Parse for string inputs.
func ParseString(s string) (*Report, error) {
	return Parse(strings.NewReader(s))
}
