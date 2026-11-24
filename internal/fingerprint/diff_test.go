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

	// The three actionable mismatches from the fixture.
	if d, ok := byField["runtime:node"]; !ok || d.A != "20.11.0" || d.B != "20.9.0" {
		t.Errorf("runtime:node diff = %+v, want 20.11.0 vs 20.9.0", d)
	}
	if d, ok := byField["timezone"]; !ok || d.A != "Asia/Kolkata" || d.B != "UTC" {
		t.Errorf("timezone diff = %+v, want Asia/Kolkata vs UTC", d)
	}
	if d, ok := byField["env:STRIPE_KEY"]; !ok || d.Detail != "missing in CI" {
		t.Errorf("env:STRIPE_KEY diff = %+v, want missing in CI", d)
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
