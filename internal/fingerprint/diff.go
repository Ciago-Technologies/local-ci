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
	if a.RunnerImage != b.RunnerImage && (a.RunnerImage != "" || b.RunnerImage != "") {
		add("runner_image", orMissing(a.RunnerImage), orMissing(b.RunnerImage), "runner image changed — the baseline may be stale", SeverityWarning)
	}
	if a.OSRelease != b.OSRelease && (a.OSRelease != "" || b.OSRelease != "") {
		add("os_release", orMissing(a.OSRelease), orMissing(b.OSRelease), "OS release differs — system packages may differ too", SeverityWarning)
	}
	if a.ContainerImage != b.ContainerImage && (a.ContainerImage != "" || b.ContainerImage != "") {
		add("container_image", orMissing(a.ContainerImage), orMissing(b.ContainerImage), "Dockerfile base image differs", SeverityWarning)
	}
	if a.LineEndings != b.LineEndings && (a.LineEndings != "" || b.LineEndings != "") {
		add("line_endings", orMissing(a.LineEndings), orMissing(b.LineEndings), "line endings differ — check .gitattributes / core.autocrlf", SeverityWarning)
	}
	if a.ExecutableFiles != b.ExecutableFiles {
		add("executable_files", strconv.Itoa(a.ExecutableFiles), strconv.Itoa(b.ExecutableFiles), "executable file count differs — a chmod may have been lost", SeverityWarning)
	}
	if a.Symlinks != b.Symlinks {
		add("symlinks", strconv.Itoa(a.Symlinks), strconv.Itoa(b.Symlinks), "symlink count differs — symlinks break on Windows runners", SeverityWarning)
	}

	// Services: union of names, from the workflow's services: block.
	for _, name := range unionKeys(a.Services, b.Services) {
		av, aok := a.Services[name]
		bv, bok := b.Services[name]
		switch {
		case !aok:
			add("service:"+name, "(missing)", bv, "service only in CI", SeverityWarning)
		case !bok:
			add("service:"+name, av, "(missing)", "service only locally", SeverityWarning)
		case av != bv:
			add("service:"+name, av, bv, "service image differs", SeverityWarning)
		}
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
	// When both sides recorded package versions, the detail names
	// the packages that changed instead of just saying "hash changed".
	for _, p := range unionKeys(a.Lockfiles, b.Lockfiles) {
		av, aok := a.Lockfiles[p]
		bv, bok := b.Lockfiles[p]
		switch {
		case !aok:
			add("lockfile:"+p, "(missing)", shortHash(bv), "missing locally", SeverityCritical)
		case !bok:
			add("lockfile:"+p, shortHash(av), "(missing)", "missing in CI", SeverityCritical)
		case av != bv:
			detail := "lockfile changed — run the package manager install step again"
			if changes := lockfilePackageChanges(a.LockfilePackages[p], b.LockfilePackages[p]); len(changes) > 0 {
				detail = "lockfile changed: " + strings.Join(changes, ", ")
			}
			add("lockfile:"+p, shortHash(av), shortHash(bv), detail, SeverityCritical)
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

// IgnoreFields drops diffs whose field is in ignore (e.g.
// "git_commit"). A baseline is an environment snapshot, not a
// commit pointer: in CI the baseline's commit always differs from
// the PR head, so the Action ignores git_commit — otherwise every
// PR would fail on the commit alone. Standalone `localci diff`
// keeps git_commit unless asked, via --ignore.
func IgnoreFields(diffs []Difference, ignore []string) []Difference {
	if len(ignore) == 0 {
		return diffs
	}
	ignored := map[string]bool{}
	for _, f := range ignore {
		ignored[strings.TrimSpace(f)] = true
	}
	var out []Difference
	for _, d := range diffs {
		if !ignored[d.Field] {
			out = append(out, d)
		}
	}
	return out
}

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
//   - runner_image: always hidden. GitHub updates runner images
//     roughly weekly, so a baseline's image drifts on every PR;
//     showing it would fail every PR on image drift alone (dogfood
//     caught this: baseline ubuntu26/20261116.88.1 vs CI
//     ubuntu26/20261116.202.1, two days apart). The os field and
//     runtime versions still catch meaningful platform change, and
//     the baseline-staleness warning covers age. Standalone diff
//     (no --workflows) still shows it.
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
	case d.Field == "cpu_count" || d.Field == "memory_mb" ||
		d.Field == "runner_image":
		return false
	case strings.HasPrefix(d.Field, "runtime:"):
		return usage.UsedRuntime(strings.TrimPrefix(d.Field, "runtime:"))
	case strings.HasPrefix(d.Field, "env:"):
		return usage.UsedEnv(strings.TrimPrefix(d.Field, "env:"))
	case strings.HasPrefix(d.Field, "env_value:"):
		return true
	case strings.HasPrefix(d.Field, "lockfile:"):
		return true
	case strings.HasPrefix(d.Field, "service:"):
		return true
	case d.Field == "git_commit" || d.Field == "case_sensitive" ||
		d.Field == "container_image" || d.Field == "line_endings" ||
		d.Field == "executable_files" || d.Field == "symlinks":
		return true
	case d.Field == "os_release":
		return !osReleaseMatches(usage.RunsOnLabels(), d.B)
	case d.Field == "timezone":
		return usage.UsedEnv("TZ") || usage.UsedEnv("TIMEZONE")
	case d.Field == "locale":
		return usage.UsedEnv("LANG") || usage.UsedEnv("LC_ALL") || usage.UsedEnv("LC_CTYPE")
	case d.Field == "os" || d.Field == "arch":
		return !runsOnMatches(usage.RunsOnLabels(), d.B)
	}
	return true
}

// osReleaseMatches reports whether a runs-on label implies the
// given OS release string (the CI side), e.g. "ubuntu 24.04"
// matches "ubuntu-latest", "macos 15.1" matches "macos-latest".
// A macOS laptop vs an Ubuntu runner is expected, not a finding.
func osReleaseMatches(labels []string, osRelease string) bool {
	fields := strings.Fields(strings.ToLower(osRelease))
	if len(fields) == 0 {
		return false
	}
	distro := fields[0]
	for _, label := range labels {
		if strings.Contains(strings.ToLower(label), distro) {
			return true
		}
	}
	return false
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

// lockfilePackageChanges describes which packages changed between
// two versions of one lockfile, e.g. ["react 18.2.0 → 18.3.0",
// "lodash added", "moment removed"]. At most 5 entries, plus a
// "+N more" tail. Empty when there is nothing to compare.
func lockfilePackageChanges(a, b map[string]string) []string {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	var changes []string
	for _, name := range unionKeys(a, b) {
		av, aok := a[name]
		bv, bok := b[name]
		switch {
		case !aok:
			changes = append(changes, name+" added ("+bv+")")
		case !bok:
			changes = append(changes, name+" removed")
		case av != bv:
			changes = append(changes, name+" "+av+" → "+bv)
		}
	}
	if len(changes) > 5 {
		changes = append(changes[:5], fmt.Sprintf("+%d more", len(changes)-5))
	}
	return changes
}

func orMissing(s string) string {
	if s == "" {
		return "(missing)"
	}
	return s
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
