package fingerprint

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Severities, most to least dangerous:
//   - "critical": will very likely break CI (missing env var or
//     runtime, major version change, lockfile drift).
//   - "warning": may break CI (minor version drift, OS/timezone/
//     locale mismatch, different commit).
//   - "info": usually harmless (CPU / memory — a laptop and a CI
//     runner rarely have the same hardware).
const (
	SeverityCritical = "critical"
	SeverityWarning  = "warning"
	SeverityInfo     = "info"
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
	// Severity is "critical", "warning", or "info".
	Severity string `json:"severity"`
	// Fix is a concrete suggested fix, when one applies.
	Fix string `json:"fix,omitempty"`
}

// Diff compares a (local) with b (CI) and returns every difference,
// sorted by field for stable output. Each difference carries a
// severity and a suggested fix.
func Diff(a, b *Fingerprint) []Difference {
	var out []Difference
	add := func(field, av, bv, detail, severity string) {
		d := Difference{Field: field, A: av, B: bv, Detail: detail, Severity: severity}
		d.Fix = Suggest(d)
		out = append(out, d)
	}
	if a.OS != b.OS {
		add("os", a.OS, b.OS, "", SeverityWarning)
	}
	if a.Arch != b.Arch {
		add("arch", a.Arch, b.Arch, "", SeverityWarning)
	}
	if a.Timezone != b.Timezone {
		add("timezone", a.Timezone, b.Timezone, "", SeverityWarning)
	}
	if a.Locale != b.Locale {
		add("locale", a.Locale, b.Locale, "", SeverityWarning)
	}
	if a.GitCommit != b.GitCommit {
		add("git_commit", shortSHA(a.GitCommit), shortSHA(b.GitCommit), "different commits — are you comparing the same push?", SeverityWarning)
	}
	if a.CaseSensitive != b.CaseSensitive {
		add("case_sensitive", strconv.FormatBool(a.CaseSensitive), strconv.FormatBool(b.CaseSensitive), "a file that differs only by case will fail on one side", SeverityCritical)
	}
	if a.CPUCount != b.CPUCount {
		add("cpu_count", strconv.Itoa(a.CPUCount), strconv.Itoa(b.CPUCount), "hardware differs — usually harmless", SeverityInfo)
	}
	if a.MemoryMB != b.MemoryMB && a.MemoryMB != 0 && b.MemoryMB != 0 {
		add("memory_mb", strconv.FormatInt(a.MemoryMB, 10), strconv.FormatInt(b.MemoryMB, 10), "hardware differs — usually harmless", SeverityInfo)
	}

	// Runtimes: union of names.
	for _, name := range unionKeys(a.Runtimes, b.Runtimes) {
		av, aok := a.Runtimes[name]
		bv, bok := b.Runtimes[name]
		switch {
		case !aok:
			add("runtime:"+name, "(missing)", bv, "missing locally", SeverityCritical)
		case !bok:
			add("runtime:"+name, av, "(missing)", "missing in CI", SeverityCritical)
		case av != bv:
			add("runtime:"+name, av, bv, "", runtimeSeverity(av, bv))
		}
	}

	// Env names: names only, never values.
	for _, name := range unionStrings(a.EnvNames, b.EnvNames) {
		inA := contains(a.EnvNames, name)
		inB := contains(b.EnvNames, name)
		switch {
		case inA && !inB:
			add("env:"+name, "set", "(missing)", "missing in CI", SeverityCritical)
		case !inA && inB:
			add("env:"+name, "(missing)", "set", "missing locally", SeverityCritical)
		}
	}

	// Env values: only for vars the user explicitly opted in to
	// with --include-values. A wrong value (e.g. NODE_ENV) is a
	// critical mismatch — the name matched, the value didn't.
	for _, name := range unionKeys(a.EnvValues, b.EnvValues) {
		av, aok := a.EnvValues[name]
		bv, bok := b.EnvValues[name]
		switch {
		case aok && bok && av != bv:
			add("env_value:"+name, av, bv, "value differs", SeverityCritical)
		case aok && !bok:
			add("env_value:"+name, av, "(missing)", "value not recorded in CI — re-capture with --include-values "+name, SeverityWarning)
		case !aok && bok:
			add("env_value:"+name, "(missing)", bv, "value not recorded locally — re-capture with --include-values "+name, SeverityWarning)
		}
	}

	// Lockfiles: union of paths, compare hashes (first 12 chars in output).
	for _, p := range unionKeys(a.Lockfiles, b.Lockfiles) {
		av, aok := a.Lockfiles[p]
		bv, bok := b.Lockfiles[p]
		switch {
		case !aok:
			add("lockfile:"+p, "(missing)", shortHash(bv), "missing locally", SeverityCritical)
		case !bok:
			add("lockfile:"+p, shortHash(av), "(missing)", "missing in CI", SeverityCritical)
		case av != bv:
			add("lockfile:"+p, shortHash(av), shortHash(bv), "lockfile changed — run the package manager install step again", SeverityCritical)
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Field < out[j].Field })
	return out
}

// HasActionable reports whether any difference is likely to cause a
// CI failure — i.e. any critical difference.
func HasActionable(diffs []Difference) bool {
	for _, d := range diffs {
		if d.Severity == SeverityCritical {
			return true
		}
	}
	return false
}

// HasCritical is the same as HasActionable, with a clearer name.
func HasCritical(diffs []Difference) bool { return HasActionable(diffs) }

// WorkflowUsage is the minimal interface Diff needs from a parsed
// workflow directory, so this package does not depend on YAML.
type WorkflowUsage interface {
	// UsedRuntime reports whether any workflow uses this runtime.
	UsedRuntime(name string) bool
	// UsedEnv reports whether any workflow references this env var.
	UsedEnv(name string) bool
	// RunsOn reports the runner labels the workflows ask for.
	RunsOnLabels() []string
	// HasWorkflows reports whether any workflow file was parsed at all.
	HasWorkflows() bool
}

// ApplyWorkflow splits diffs into shown and hidden, based on what
// the workflows actually use. Hidden diffs are the false positives
// Phase 3 removes: a mismatch in something no job touches.
//
// Rules:
//   - cpu/memory: always hidden (info only).
//   - runtime:X: shown only if a workflow uses X.
//   - env:X: shown only if a workflow references X.
//   - timezone: shown only if a workflow sets TZ; locale: only if
//     it sets LANG / LC_ALL / LC_CTYPE.
//   - os/arch: shown only if the CI platform doesn't match any
//     runs-on label (e.g. workflow says macos-latest but CI ran
//     ubuntu) — a macOS laptop vs an Ubuntu runner is expected.
//   - lockfile, git_commit, case_sensitive, env_value: always shown.
//   - No workflows found: everything is shown (fail-open).
func ApplyWorkflow(diffs []Difference, usage WorkflowUsage) (shown, hidden []Difference) {
	if usage == nil || !usage.HasWorkflows() {
		return diffs, nil
	}
	for _, d := range diffs {
		if workflowUses(d, usage) {
			shown = append(shown, d)
		} else {
			hidden = append(hidden, d)
		}
	}
	return shown, hidden
}

func workflowUses(d Difference, usage WorkflowUsage) bool {
	switch {
	case d.Field == "cpu_count" || d.Field == "memory_mb":
		return false
	case strings.HasPrefix(d.Field, "runtime:"):
		return usage.UsedRuntime(strings.TrimPrefix(d.Field, "runtime:"))
	case strings.HasPrefix(d.Field, "env:"):
		return usage.UsedEnv(strings.TrimPrefix(d.Field, "env:"))
	case strings.HasPrefix(d.Field, "env_value:"):
		return true
	case strings.HasPrefix(d.Field, "lockfile:"):
		return true
	case d.Field == "git_commit" || d.Field == "case_sensitive":
		return true
	case d.Field == "timezone":
		return usage.UsedEnv("TZ") || usage.UsedEnv("TIMEZONE")
	case d.Field == "locale":
		return usage.UsedEnv("LANG") || usage.UsedEnv("LC_ALL") || usage.UsedEnv("LC_CTYPE")
	case d.Field == "os" || d.Field == "arch":
		return !runsOnMatches(usage.RunsOnLabels(), d.B)
	}
	return true
}

// runsOnMatches reports whether a runs-on label implies the given
// platform string (the CI side's OS or arch). "ubuntu-latest" implies
// linux/amd64, "macos-latest" implies darwin/arm64, and so on.
func runsOnMatches(labels []string, platform string) bool {
	for _, label := range labels {
		l := strings.ToLower(label)
		switch {
		case strings.Contains(l, "ubuntu") && (platform == "linux" || platform == "amd64"):
			return true
		case strings.Contains(l, "macos") && (platform == "darwin" || platform == "arm64"):
			return true
		case strings.Contains(l, "windows") && (platform == "windows" || platform == "amd64"):
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
	return FormatHumanWithHidden(diffs, nil, aLabel, bLabel)
}

// FormatHumanWithHidden is FormatHuman plus a note about diffs
// hidden by workflow-aware filtering.
func FormatHumanWithHidden(diffs, hidden []Difference, aLabel, bLabel string) string {
	if len(diffs) == 0 {
		out := "No differences — local and CI fingerprints match.\n"
		if len(hidden) > 0 {
			out += fmt.Sprintf("%d difference(s) hidden — not used by any workflow job.\n", len(hidden))
		}
		return out
	}
	var critical, warning, info int
	for _, d := range diffs {
		switch d.Severity {
		case SeverityCritical:
			critical++
		case SeverityWarning:
			warning++
		default:
			info++
		}
	}
	out := fmt.Sprintf("Found %d difference(s) — %s vs %s (%d critical, %d warning, %d info):\n",
		len(diffs), aLabel, bLabel, critical, warning, info)
	for _, d := range diffs {
		line := fmt.Sprintf("  [%s] %s: %s = %s, %s = %s", d.Severity, d.Field, aLabel, d.A, bLabel, d.B)
		if d.Detail != "" {
			line += fmt.Sprintf("  (%s)", d.Detail)
		}
		out += line + "\n"
		if d.Fix != "" {
			out += fmt.Sprintf("      Fix: %s\n", d.Fix)
		}
	}
	if len(hidden) > 0 {
		out += fmt.Sprintf("%d difference(s) hidden — not used by any workflow job (use --show-unused to see them).\n", len(hidden))
	}
	return out
}
