package fingerprint

import (
	"fmt"
	"sort"
	"strconv"
)

// Difference is one mismatch between two fingerprints.
// A is the first file (usually local), B is the second (usually CI).
type Difference struct {
	Field string `json:"field"`
	// A and B hold the differing values, as human-readable strings.
	A string `json:"a,omitempty"`
	B string `json:"b,omitempty"`
	// Detail explains the mismatch, e.g. "missing in CI".
	Detail string `json:"detail,omitempty"`
	// Severity is "actionable" for likely causes of a CI failure,
	// "info" for hardware differences that are usually harmless
	// (a laptop and a CI runner rarely have the same CPU / memory).
	Severity string `json:"severity"`
}

// Diff compares a (local) with b (CI) and returns every difference,
// sorted by field for stable output.
func Diff(a, b *Fingerprint) []Difference {
	var out []Difference
	add := func(field, av, bv, detail, severity string) {
		out = append(out, Difference{Field: field, A: av, B: bv, Detail: detail, Severity: severity})
	}
	if a.OS != b.OS {
		add("os", a.OS, b.OS, "", "actionable")
	}
	if a.Arch != b.Arch {
		add("arch", a.Arch, b.Arch, "", "actionable")
	}
	if a.Timezone != b.Timezone {
		add("timezone", a.Timezone, b.Timezone, "", "actionable")
	}
	if a.Locale != b.Locale {
		add("locale", a.Locale, b.Locale, "", "actionable")
	}
	if a.GitCommit != b.GitCommit {
		add("git_commit", shortSHA(a.GitCommit), shortSHA(b.GitCommit), "different commits — are you comparing the same push?", "actionable")
	}
	if a.CaseSensitive != b.CaseSensitive {
		add("case_sensitive", strconv.FormatBool(a.CaseSensitive), strconv.FormatBool(b.CaseSensitive), "a file that differs only by case will fail on one side", "actionable")
	}
	if a.CPUCount != b.CPUCount {
		add("cpu_count", strconv.Itoa(a.CPUCount), strconv.Itoa(b.CPUCount), "hardware differs — usually harmless", "info")
	}
	if a.MemoryMB != b.MemoryMB && a.MemoryMB != 0 && b.MemoryMB != 0 {
		add("memory_mb", strconv.FormatInt(a.MemoryMB, 10), strconv.FormatInt(b.MemoryMB, 10), "hardware differs — usually harmless", "info")
	}

	// Runtimes: union of names.
	for _, name := range unionKeys(a.Runtimes, b.Runtimes) {
		av, aok := a.Runtimes[name]
		bv, bok := b.Runtimes[name]
		switch {
		case !aok:
			add("runtime:"+name, "(missing)", bv, "missing locally", "actionable")
		case !bok:
			add("runtime:"+name, av, "(missing)", "missing in CI", "actionable")
		case av != bv:
			add("runtime:"+name, av, bv, "", "actionable")
		}
	}

	// Env names: names only, never values.
	for _, name := range unionStrings(a.EnvNames, b.EnvNames) {
		inA := contains(a.EnvNames, name)
		inB := contains(b.EnvNames, name)
		switch {
		case inA && !inB:
			add("env:"+name, "set", "(missing)", "missing in CI", "actionable")
		case !inA && inB:
			add("env:"+name, "(missing)", "set", "missing locally", "actionable")
		}
	}

	// Lockfiles: union of paths, compare hashes (first 12 chars in output).
	for _, p := range unionKeys(a.Lockfiles, b.Lockfiles) {
		av, aok := a.Lockfiles[p]
		bv, bok := b.Lockfiles[p]
		switch {
		case !aok:
			add("lockfile:"+p, "(missing)", shortHash(bv), "missing locally", "actionable")
		case !bok:
			add("lockfile:"+p, shortHash(av), "(missing)", "missing in CI", "actionable")
		case av != bv:
			add("lockfile:"+p, shortHash(av), shortHash(bv), "lockfile changed — run the package manager install step again", "actionable")
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Field < out[j].Field })
	return out
}

// HasActionable reports whether any difference is likely to cause a CI failure.
func HasActionable(diffs []Difference) bool {
	for _, d := range diffs {
		if d.Severity == "actionable" {
			return true
		}
	}
	return false
}

func unionKeys(a, b map[string]string) []string {
	seen := map[string]bool{}
	var out []string
	for k := range a {
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	for k := range b {
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func unionStrings(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range append(append([]string{}, a...), b...) {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func shortSHA(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

func shortHash(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

// FormatHuman renders diffs as the CLI's human output.
// aLabel / bLabel are e.g. "local" and "CI".
func FormatHuman(diffs []Difference, aLabel, bLabel string) string {
	if len(diffs) == 0 {
		return "No differences — local and CI fingerprints match.\n"
	}
	out := fmt.Sprintf("Found %d difference(s) — %s vs %s:\n", len(diffs), aLabel, bLabel)
	for _, d := range diffs {
		line := fmt.Sprintf("  %s: %s = %s, %s = %s", d.Field, aLabel, d.A, bLabel, d.B)
		if d.Detail != "" {
			line += fmt.Sprintf("  (%s)", d.Detail)
		}
		if d.Severity == "info" {
			line += "  [info]"
		}
		out += line + "\n"
	}
	return out
}
