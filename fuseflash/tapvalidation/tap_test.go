package tapvalidation

import (
	"strings"
	"testing"
)

const simpleTAP = `TAP version 13
1..3
ok 1 node ID generated
ok 2 phonetic round-trip
ok 3 block header sync`

const failingTAP = `TAP version 13
1..2
ok 1 first test
not ok 2 second test`

const skipTAP = `TAP version 13
1..2
ok 1 real test
ok 2 skipped test # SKIP not implemented yet`

const todoTAP = `TAP version 13
1..2
ok 1 real test
not ok 2 pending test # TODO implement later`

const bailOutTAP = `TAP version 13
1..2
ok 1 first test
Bail out! database unavailable`

const noVersionTAP = `1..1
ok 1 no version header`

const mismatchedPlanTAP = `TAP version 13
1..3
ok 1 only one test`

func TestParseSimple(t *testing.T) {
	report, err := ParseString(simpleTAP)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	if report.Version != 13 {
		t.Errorf("version: want 13, got %d", report.Version)
	}
	if !report.Plan.set {
		t.Error("plan not parsed")
	}
	if report.Plan.From != 1 || report.Plan.To != 3 {
		t.Errorf("plan: want 1..3, got %d..%d", report.Plan.From, report.Plan.To)
	}
	if len(report.Results) != 3 {
		t.Errorf("results: want 3, got %d", len(report.Results))
	}
	for _, r := range report.Results {
		if !r.OK {
			t.Errorf("result %d should be OK", r.Number)
		}
	}
}

func TestValidatePass(t *testing.T) {
	report, _ := ParseString(simpleTAP)
	if err := report.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestValidateFail(t *testing.T) {
	report, _ := ParseString(failingTAP)
	if err := report.Validate(); err == nil {
		t.Error("expected validation error for failing TAP, got nil")
	}
}

func TestValidateSkip(t *testing.T) {
	report, _ := ParseString(skipTAP)
	if err := report.Validate(); err != nil {
		t.Errorf("SKIP test should not cause failure: %v", err)
	}
	if report.Results[1].IsSkip() != true {
		t.Error("expected result 2 to be SKIP")
	}
}

func TestValidateTodo(t *testing.T) {
	report, _ := ParseString(todoTAP)
	if err := report.Validate(); err != nil {
		t.Errorf("TODO not-ok should not cause failure: %v", err)
	}
	if report.Results[1].IsTodo() != true {
		t.Error("expected result 2 to be TODO")
	}
}

func TestBailOut(t *testing.T) {
	report, _ := ParseString(bailOutTAP)
	if !report.BailOut {
		t.Error("expected bail out to be set")
	}
	if err := report.Validate(); err == nil {
		t.Error("expected error for bail out")
	}
}

func TestNoVersionHeader(t *testing.T) {
	report, err := ParseString(noVersionTAP)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	if report.Version != 0 {
		t.Errorf("version: want 0 (absent), got %d", report.Version)
	}
	// Should still validate as long as plan and tests match.
	if err := report.Validate(); err != nil {
		t.Errorf("no version should still validate: %v", err)
	}
}

func TestMismatchedPlan(t *testing.T) {
	report, _ := ParseString(mismatchedPlanTAP)
	if err := report.Validate(); err == nil {
		t.Error("expected error for mismatched plan/test count")
	}
}

func TestPassCount(t *testing.T) {
	report, _ := ParseString(simpleTAP)
	if report.PassCount() != 3 {
		t.Errorf("PassCount: want 3, got %d", report.PassCount())
	}
}

func TestFailCount(t *testing.T) {
	report, _ := ParseString(failingTAP)
	if report.FailCount() != 1 {
		t.Errorf("FailCount: want 1, got %d", report.FailCount())
	}
}

func TestParseEmptyStream(t *testing.T) {
	report, err := ParseString("")
	if err != nil {
		t.Fatalf("ParseString empty: %v", err)
	}
	if err := report.Validate(); err == nil {
		t.Error("empty stream should not validate (missing plan)")
	}
}

func TestParseAutoNumbering(t *testing.T) {
	tap := strings.Join([]string{
		"TAP version 13",
		"1..2",
		"ok first",
		"ok second",
	}, "\n")
	report, err := ParseString(tap)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	if report.Results[0].Number != 1 || report.Results[1].Number != 2 {
		t.Errorf("auto-numbering: got %d, %d", report.Results[0].Number, report.Results[1].Number)
	}
}
