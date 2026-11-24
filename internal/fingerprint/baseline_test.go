package fingerprint

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPickBaseline_MatrixCell(t *testing.T) {
	// CI runs on Ubuntu, Node 20 — should pick ubuntu-node20, not
	// ubuntu-node22 (wrong major) or macos-node20 (wrong OS).
	ci := &Fingerprint{
		OS:       "linux",
		Arch:     "amd64",
		Runtimes: map[string]string{"go": "1.23.0", "node": "20.9.0", "python": "3.12.0"},
	}
	path, fp, err := PickBaseline(filepath.Join("testdata", "baselines"), ci)
	if err != nil {
		t.Fatalf("PickBaseline: %v", err)
	}
	if got := filepath.Base(path); got != "ubuntu-node20.json" {
		t.Errorf("picked %s, want ubuntu-node20.json", got)
	}
	if fp.Runtimes["node"] != "20.11.0" {
		t.Errorf("picked baseline node = %s, want 20.11.0", fp.Runtimes["node"])
	}
}

func TestPickBaseline_NoMatch(t *testing.T) {
	ci := &Fingerprint{
		OS:       "windows",
		Arch:     "amd64",
		Runtimes: map[string]string{"dotnet": "8.0.0"},
	}
	_, _, err := PickBaseline(filepath.Join("testdata", "baselines"), ci)
	if err == nil {
		t.Error("PickBaseline with no matching baseline: want error, got none")
	}
}

func TestPickBaseline_MissingDir(t *testing.T) {
	_, _, err := PickBaseline(filepath.Join("testdata", "no-such-dir"), &Fingerprint{})
	if err == nil {
		t.Error("PickBaseline with missing dir: want error, got none")
	}
}

func TestStalenessWarning(t *testing.T) {
	old := &Fingerprint{CapturedAt: time.Now().AddDate(0, 0, -40).UTC()}
	w := StalenessWarning(old, BaselineMaxAgeDays)
	if !strings.Contains(w, "40 days old") {
		t.Errorf("stale baseline warning = %q, want '40 days old'", w)
	}
	fresh := &Fingerprint{CapturedAt: time.Now().AddDate(0, 0, -5).UTC()}
	if w := StalenessWarning(fresh, BaselineMaxAgeDays); w != "" {
		t.Errorf("fresh baseline warning = %q, want empty", w)
	}
}

func TestDiff_RunnerImage(t *testing.T) {
	a := &Fingerprint{RunnerImage: "ubuntu24/20250101.1"}
	b := &Fingerprint{RunnerImage: "ubuntu24/20251123.1"}
	diffs := Diff(a, b)
	if len(diffs) != 1 || diffs[0].Field != "runner_image" {
		t.Fatalf("runner image diffs = %+v, want one runner_image diff", diffs)
	}
	if diffs[0].Severity != SeverityWarning {
		t.Errorf("runner_image severity = %q, want warning", diffs[0].Severity)
	}
	if diffs[0].Fix == "" {
		t.Error("runner_image diff has no fix")
	}
	// No runner image on either side: no diff.
	if diffs := Diff(&Fingerprint{}, &Fingerprint{}); len(diffs) != 0 {
		t.Errorf("empty fingerprints diff = %v, want none", diffs)
	}
}

func TestCapture_RunnerImage(t *testing.T) {
	t.Setenv("ImageOS", "ubuntu24")
	t.Setenv("ImageVersion", "20251123.1")
	fp, err := Capture(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if fp.RunnerImage != "ubuntu24/20251123.1" {
		t.Errorf("RunnerImage = %q, want ubuntu24/20251123.1", fp.RunnerImage)
	}
}
