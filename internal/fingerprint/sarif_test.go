package fingerprint

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestToSARIF_FixtureDiffs(t *testing.T) {
	local := loadFixture(t, "local.json")
	ci := loadFixture(t, "ci.json")
	diffs := Diff(local, ci)

	data, err := ToSARIF(diffs)
	if err != nil {
		t.Fatal(err)
	}
	var report map[string]interface{}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("SARIF is not valid JSON: %v\n%s", err, data)
	}
	if report["version"] != "2.1.0" {
		t.Errorf("SARIF version = %v, want 2.1.0", report["version"])
	}
	s := string(data)
	// STRIPE_KEY is critical -> error, Node is warning.
	if !strings.Contains(s, "STRIPE_KEY") {
		t.Error("SARIF missing STRIPE_KEY")
	}
	if !strings.Contains(s, `"level": "error"`) {
		t.Error("SARIF missing an error level (want STRIPE_KEY as error)")
	}
	if !strings.Contains(s, `"level": "warning"`) {
		t.Error("SARIF missing a warning level (want Node as warning)")
	}
	// Message carries the fix.
	if !strings.Contains(s, "Fix:") {
		t.Error("SARIF messages missing Fix:")
	}
	// Rule id is sanitised, no colon.
	if strings.Contains(s, `"ruleId": "env:STRIPE_KEY"`) {
		t.Error("SARIF ruleId still contains a colon")
	}
	if !strings.Contains(s, "env-STRIPE_KEY") {
		t.Error("SARIF missing rule env-STRIPE_KEY")
	}
	// Result count matches diff count.
	runs, _ := report["runs"].([]interface{})
	if len(runs) != 1 {
		t.Fatalf("SARIF runs = %d, want 1", len(runs))
	}
	run, _ := runs[0].(map[string]interface{})
	results, _ := run["results"].([]interface{})
	if len(results) != len(diffs) {
		t.Errorf("SARIF results = %d, want %d", len(results), len(diffs))
	}
}

func TestToSARIF_Empty(t *testing.T) {
	data, err := ToSARIF(nil)
	if err != nil {
		t.Fatal(err)
	}
	var report map[string]interface{}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("empty SARIF is not valid JSON: %v", err)
	}
}

func TestSarifLevel(t *testing.T) {
	if SarifLevel("critical") != "error" || SarifLevel("warning") != "warning" || SarifLevel("info") != "note" {
		t.Errorf("SarifLevel mapping wrong")
	}
}
