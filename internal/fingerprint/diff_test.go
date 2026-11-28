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

func TestDiff_LockfilePackageDetail(t *testing.T) {
	a := &Fingerprint{
		Lockfiles: map[string]string{"package-lock.json": "aaa"},
		LockfilePackages: map[string]map[string]string{
			"package-lock.json": {"react": "18.2.0", "lodash": "4.17.21", "moment": "2.29.0"},
		},
	}
	b := &Fingerprint{
		Lockfiles: map[string]string{"package-lock.json": "bbb"},
		LockfilePackages: map[string]map[string]string{
			"package-lock.json": {"react": "18.3.0", "lodash": "4.17.21", "axios": "1.7.0"},
		},
	}
	diffs := Diff(a, b)
	if len(diffs) != 1 {
		t.Fatalf("lockfile diffs = %v, want 1", diffs)
	}
	d := diffs[0]
	for _, want := range []string{"react 18.2.0 → 18.3.0", "axios added", "moment removed"} {
		if !containsString(d.Detail, want) {
			t.Errorf("lockfile detail = %q, want it to contain %q", d.Detail, want)
		}
	}
	if d.Severity != SeverityCritical {
		t.Errorf("lockfile severity = %q, want critical", d.Severity)
	}
}

func TestDiff_NewFields(t *testing.T) {
	a := &Fingerprint{
		OSRelease: "ubuntu 22.04", ContainerImage: "node:20-alpine",
		LineEndings: "lf", ExecutableFiles: 3, Symlinks: 1,
		Services: map[string]string{"postgres": "postgres:15"},
	}
	b := &Fingerprint{
		OSRelease: "ubuntu 24.04", ContainerImage: "node:22-alpine",
		LineEndings: "crlf", ExecutableFiles: 2, Symlinks: 0,
		Services: map[string]string{"postgres": "postgres:16"},
	}
	diffs := Diff(a, b)
	byField := map[string]Difference{}
	for _, d := range diffs {
		byField[d.Field] = d
	}
	for _, f := range []string{"os_release", "container_image", "line_endings", "executable_files", "symlinks", "service:postgres"} {
		d, ok := byField[f]
		if !ok {
			t.Errorf("missing diff for %s: %v", f, diffs)
			continue
		}
		if d.Severity != SeverityWarning {
			t.Errorf("%s severity = %q, want warning", f, d.Severity)
		}
		if d.Fix == "" {
			t.Errorf("%s has no fix", f)
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

func TestApplyWorkflow_HidesRunnerImage(t *testing.T) {
	// Dogfood: the baseline's runner image drifted two days after
	// capture (ubuntu26/20261116.88.1 vs ...202.1) and showed as a
	// warning diff — GitHub updates images roughly weekly, so
	// showing it would fail every PR on image drift alone.
	local := loadFixture(t, "local.json")
	ci := loadFixture(t, "ci.json")
	local.RunnerImage = "ubuntu26/20261116.88.1"
	ci.RunnerImage = "ubuntu26/20261116.202.1"
	diffs := Diff(local, ci)
	found := false
	for _, d := range diffs {
		if d.Field == "runner_image" {
			found = true
		}
	}
	if !found {
		t.Fatal("want a runner_image diff before filtering")
	}
	usage := fakeUsage{
		runtimes: map[string]bool{"node": true},
		env:      map[string]bool{"STRIPE_KEY": true},
		runsOn:   []string{"ubuntu-latest"},
		found:    true,
	}
	shown, hidden := ApplyWorkflow(diffs, usage)
	for _, d := range shown {
		if d.Field == "runner_image" {
			t.Error("runner_image diff shown by the workflow filter")
		}
	}
	hiddenFields := map[string]bool{}
	for _, d := range hidden {
		hiddenFields[d.Field] = true
	}
	if !hiddenFields["runner_image"] {
		t.Error("runner_image diff not hidden")
	}
	// Standalone diff (no workflows) still shows it.
	shown, _ = ApplyWorkflow(diffs, fakeUsage{found: false})
	found = false
	for _, d := range shown {
		if d.Field == "runner_image" {
			found = true
		}
	}
	if !found {
		t.Error("runner_image diff hidden without workflows (fail-open broken)")
	}
}

func TestIgnoreFields_GitCommit(t *testing.T) {
	// Dogfood: a baseline's commit always differs from the PR head,
	// so the Action ignores git_commit — otherwise every PR fails
	// on the commit alone.
	local := loadFixture(t, "local.json")
	ci := loadFixture(t, "ci.json")
	// Give the two sides different commits.
	ci.GitCommit = "different00000000000000000000000000000000"
	diffs := Diff(local, ci)
	found := false
	for _, d := range diffs {
		if d.Field == "git_commit" {
			found = true
		}
	}
	if !found {
		t.Fatal("want a git_commit diff before ignoring")
	}
	kept := IgnoreFields(diffs, []string{"git_commit"})
	for _, d := range kept {
		if d.Field == "git_commit" {
			t.Error("git_commit diff survived --ignore")
		}
	}
	if len(kept) != len(diffs)-1 {
		t.Errorf("IgnoreFields kept %d diffs, want %d", len(kept), len(diffs)-1)
	}
	// Empty ignore list changes nothing.
	if got := IgnoreFields(diffs, nil); len(got) != len(diffs) {
		t.Errorf("empty ignore changed the diffs")
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

func TestDiff_ToolsBackwardCompatible(t *testing.T) {
	// Old baseline has no tools field; new CI fingerprint does.
	// That must not flag every tool as missing in the baseline.
	a := &Fingerprint{}
	b := &Fingerprint{Tools: map[string]string{"git": "2.43.0"}}
	if diffs := Diff(a, b); len(diffs) != 0 {
		t.Errorf("old baseline vs new CI diffs = %v, want none for tools", diffs)
	}
	// Both sides new: a real drift is shown.
	a.Tools = map[string]string{"git": "2.42.0"}
	diffs := Diff(a, b)
	if len(diffs) != 1 || diffs[0].Field != "tool:git" {
		t.Errorf("tool drift diffs = %v, want one tool:git", diffs)
	}
}
