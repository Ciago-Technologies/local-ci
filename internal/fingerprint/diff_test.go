package fingerprint

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func loadFixture(t *testing.T, name string) *Fingerprint {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var fp Fingerprint
	if err := json.Unmarshal(data, &fp); err != nil {
		t.Fatal(err)
	}
	return &fp
}

func TestDiff_KnownMismatch(t *testing.T) {
	local := loadFixture(t, "local.json")
	ci := loadFixture(t, "ci.json")

	diffs := Diff(local, ci)
	byField := map[string]Difference{}
	for _, d := range diffs {
		byField[d.Field] = d
	}

	// Severity ranking: a missing secret is critical, a Node
	// minor-version drift is only a warning.
	if d, ok := byField["runtime:node"]; !ok || d.A != "20.11.0" || d.B != "20.9.0" {
		t.Errorf("runtime:node diff = %+v, want 20.11.0 vs 20.9.0", d)
	} else if d.Severity != SeverityWarning {
		t.Errorf("runtime:node severity = %q, want warning (same major version)", d.Severity)
	}
	if d, ok := byField["timezone"]; !ok || d.A != "Asia/Kolkata" || d.B != "UTC" {
		t.Errorf("timezone diff = %+v, want Asia/Kolkata vs UTC", d)
	}
	if d, ok := byField["env:STRIPE_KEY"]; !ok || d.Detail != "missing in CI" {
		t.Errorf("env:STRIPE_KEY diff = %+v, want missing in CI", d)
	} else if d.Severity != SeverityCritical {
		t.Errorf("env:STRIPE_KEY severity = %q, want critical", d.Severity)
	}
	if d, ok := byField["lockfile:package-lock.json"]; !ok || d.A == d.B {
		t.Errorf("lockfile diff = %+v, want differing hashes", d)
	}
	if !HasActionable(diffs) {
		t.Error("HasActionable = false, want true for this fixture")
	}
	// Hardware differences are info-only, not actionable on their own.
	if d, ok := byField["cpu_count"]; !ok || d.Severity != "info" {
		t.Errorf("cpu_count diff = %+v, want severity info", d)
	}
	// Every critical difference carries a fix.
	for _, d := range diffs {
		if d.Severity == SeverityCritical && d.Fix == "" {
			t.Errorf("critical diff %s has no fix", d.Field)
		}
	}
}

func TestDiff_RuntimeMajorMismatchIsCritical(t *testing.T) {
	a := &Fingerprint{Runtimes: map[string]string{"node": "20.11.0"}}
	b := &Fingerprint{Runtimes: map[string]string{"node": "18.19.0"}}
	diffs := Diff(a, b)
	if len(diffs) != 1 || diffs[0].Severity != SeverityCritical {
		t.Errorf("major version drift diffs = %+v, want one critical", diffs)
	}
}

func TestDiff_EnvValuesOptIn(t *testing.T) {
	a := &Fingerprint{EnvValues: map[string]string{"NODE_ENV": "development"}}
	b := &Fingerprint{EnvValues: map[string]string{"NODE_ENV": "production"}}
	diffs := Diff(a, b)
	if len(diffs) != 1 || diffs[0].Field != "env_value:NODE_ENV" || diffs[0].Severity != SeverityCritical {
		t.Errorf("env value diffs = %+v, want one critical env_value:NODE_ENV", diffs)
	}
}

// fakeUsage implements WorkflowUsage for tests.
type fakeUsage struct {
	runtimes map[string]bool
	env      map[string]bool
	runsOn   []string
	found    bool
}

func (f fakeUsage) UsedRuntime(n string) bool { return f.runtimes[n] }
func (f fakeUsage) UsedEnv(n string) bool     { return f.env[n] }
func (f fakeUsage) RunsOnLabels() []string    { return f.runsOn }
func (f fakeUsage) HasWorkflows() bool        { return f.found }

func TestApplyWorkflow_HidesUnused(t *testing.T) {
	local := loadFixture(t, "local.json")
	ci := loadFixture(t, "ci.json")
	diffs := Diff(local, ci)
	if len(diffs) != 7 {
		t.Fatalf("fixture diffs = %d, want 7", len(diffs))
	}
	// Workflow uses node + STRIPE_KEY only, runs on ubuntu.
	usage := fakeUsage{
		runtimes: map[string]bool{"node": true},
		env:      map[string]bool{"STRIPE_KEY": true},
		runsOn:   []string{"ubuntu-latest"},
		found:    true,
	}
	shown, hidden := ApplyWorkflow(diffs, usage)
	shownFields := map[string]bool{}
	for _, d := range shown {
		shownFields[d.Field] = true
	}
	for _, want := range []string{"runtime:node", "env:STRIPE_KEY", "lockfile:package-lock.json"} {
		if !shownFields[want] {
			t.Errorf("shown missing %s: %v", want, shown)
		}
	}
	hiddenFields := map[string]bool{}
	for _, d := range hidden {
		hiddenFields[d.Field] = true
	}
	for _, want := range []string{"timezone", "locale", "cpu_count", "memory_mb"} {
		if !hiddenFields[want] {
			t.Errorf("hidden missing %s: %v", want, hidden)
		}
	}
	if len(shown) != 3 || len(hidden) != 4 {
		t.Errorf("shown=%d hidden=%d, want 3 and 4", len(shown), len(hidden))
	}
}

func TestApplyWorkflow_NoWorkflowsShowsAll(t *testing.T) {
	local := loadFixture(t, "local.json")
	ci := loadFixture(t, "ci.json")
	diffs := Diff(local, ci)
	shown, hidden := ApplyWorkflow(diffs, fakeUsage{found: false})
	if len(shown) != len(diffs) || len(hidden) != 0 {
		t.Errorf("no workflows: shown=%d hidden=%d, want all %d shown", len(shown), len(hidden), len(diffs))
	}
}

func TestDiff_Identical(t *testing.T) {
	local := loadFixture(t, "local.json")
	// Compare a fingerprint with itself: no differences.
	if diffs := Diff(local, local); len(diffs) != 0 {
		t.Errorf("Diff(x, x) = %v, want none", diffs)
	}
}

func TestDiff_HumanOutput(t *testing.T) {
	local := loadFixture(t, "local.json")
	ci := loadFixture(t, "ci.json")
	out := FormatHuman(Diff(local, ci), "local", "CI")
	for _, want := range []string{"runtime:node", "20.11.0", "20.9.0", "Asia/Kolkata", "STRIPE_KEY"} {
		if !containsString(out, want) {
			t.Errorf("human output missing %q:\n%s", want, out)
		}
	}
	if got := FormatHuman(nil, "local", "CI"); !containsString(got, "No differences") {
		t.Errorf("empty diff output = %q, want 'No differences'", got)
	}
}

func containsString(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
