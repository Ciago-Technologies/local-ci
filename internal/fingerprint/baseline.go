package fingerprint

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// BaselineMaxAgeDays is when a baseline counts as stale. A baseline
// older than this was captured against a different world — a newer
// runner image, newer runtimes — and its differences may be noise.
const BaselineMaxAgeDays = 30

// StalenessWarning returns a warning when the baseline fingerprint
// is older than maxAgeDays, or "" when it is fresh. It is a warning,
// not a difference: a stale baseline can still be correct, it just
// deserves a re-capture.
func StalenessWarning(baseline *Fingerprint, maxAgeDays int) string {
	if baseline == nil || baseline.CapturedAt.IsZero() {
		return ""
	}
	age := time.Since(baseline.CapturedAt)
	if age < time.Duration(maxAgeDays)*24*time.Hour {
		return ""
	}
	days := int(age.Hours() / 24)
	return fmt.Sprintf("Note: baseline is %d days old (captured %s) — re-capture it: localci capture -o .localci/local.json",
		days, baseline.CapturedAt.Format("2006-01-02"))
}

// PickBaseline chooses, from the *.json files in dir, the baseline
// that best matches the CI fingerprint: same OS and arch, same
// runtime major versions. This is matrix support — one baseline per
// matrix cell (e.g. .localci/ubuntu-node20.json,
// .localci/macos-node22.json) instead of one baseline for the whole
// repo. Files are matched by content, not by name, so any naming
// convention works.
//
// It returns the chosen file's path and parsed fingerprint. When
// nothing in the dir matches the CI environment at all, it returns
// an error — diffing against a wildly wrong baseline is worse
// than no baseline.
func PickBaseline(dir string, ci *Fingerprint) (string, *Fingerprint, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", nil, fmt.Errorf("reading baseline dir %s: %w", dir, err)
	}
	type candidate struct {
		path  string
		fp    *Fingerprint
		score int
	}
	var cands []candidate
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var fp Fingerprint
		if err := json.Unmarshal(data, &fp); err != nil {
			continue
		}
		cands = append(cands, candidate{path: path, fp: &fp, score: baselineScore(&fp, ci)})
	}
	if len(cands) == 0 {
		return "", nil, fmt.Errorf("no baseline fingerprints found in %s (expected .localci/*.json)", dir)
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].score != cands[j].score {
			return cands[i].score > cands[j].score
		}
		// Prefer local.json on a tie — it is the documented default.
		iLocal := filepath.Base(cands[i].path) == "local.json"
		jLocal := filepath.Base(cands[j].path) == "local.json"
		if iLocal != jLocal {
			return iLocal
		}
		return cands[i].path < cands[j].path
	})
	best := cands[0]
	if best.score <= 0 {
		return "", nil, fmt.Errorf("no baseline in %s matches this CI environment (os %s/%s, runtimes %v) — capture one with: localci capture -o %s/<name>.json",
			dir, ci.OS, ci.Arch, ci.Runtimes, dir)
	}
	return best.path, best.fp, nil
}

// baselineScore ranks how well a baseline matches the CI fingerprint.
// OS match is worth 2, arch 1, each runtime with the same major
// version 2, each runtime present at all 1. Arch alone does not
// count: a Windows/amd64 baseline must not "match" a Linux/amd64
// run just because both are amd64 — at least the OS or a runtime
// must overlap, otherwise the score is 0.
func baselineScore(base, ci *Fingerprint) int {
	score := 0
	matched := false
	if base.OS != "" && base.OS == ci.OS {
		score += 2
		matched = true
	}
	if base.Arch != "" && base.Arch == ci.Arch {
		score += 1
	}
	for name, bv := range base.Runtimes {
		cv, ok := ci.Runtimes[name]
		if !ok {
			continue
		}
		matched = true
		score += 1
		if majorVersion(bv) != "" && majorVersion(bv) == majorVersion(cv) {
			score += 2
		}
	}
	if !matched {
		return 0
	}
	return score
}
