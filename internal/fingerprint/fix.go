package fingerprint

import (
	"fmt"
	"strings"
)

// Suggest returns a concrete fix for a difference, or "" when there
// is nothing actionable to say. Fixes are generic (no workflow
// context needed) — the workflow-aware filter decides whether the
// difference is shown at all.
func Suggest(d Difference) string {
	field := d.Field

	// Env vars.
	if name, ok := strings.CutPrefix(field, "env:"); ok {
		switch d.Detail {
		case "missing in CI":
			return fmt.Sprintf("Add %s in repo Settings > Secrets and variables > Actions, then re-run the workflow.", name)
		case "missing locally":
			return fmt.Sprintf("Set %s locally (export %s=... or add it to your .env file — never commit secrets).", name, name)
		}
	}
	if name, ok := strings.CutPrefix(field, "env_value:"); ok {
		return fmt.Sprintf("Set %s to %q on both sides — in the workflow job env for CI, and in your shell or .env locally.", name, d.B)
	}

	// Runtimes.
	if name, ok := strings.CutPrefix(field, "runtime:"); ok {
		switch d.Detail {
		case "missing in CI":
			return fmt.Sprintf("Add a setup step for %s %s in .github/workflows (e.g. actions/setup-%s with version %s).", name, d.A, name, d.A)
		case "missing locally":
			return fmt.Sprintf("Install %s %s locally to match CI.", name, d.B)
		default:
			if d.Severity == "critical" {
				return fmt.Sprintf("Pin %s-version: %s in the workflow's setup step and install %s %s locally — the major version changed.", name, d.B, name, d.B)
			}
			return fmt.Sprintf("Pin %s-version: %s in the workflow, or update your local %s to %s.", name, d.B, name, d.B)
		}
	}

	// Lockfiles.
	if _, ok := strings.CutPrefix(field, "lockfile:"); ok {
		return "Re-run the install step locally (npm ci / go mod tidy / pip install) and commit the updated lockfile."
	}

	switch field {
	case "timezone":
		return "Set TZ: UTC in the workflow job env, or run tests locally with TZ set to match CI."
	case "locale":
		return "Set LANG / LC_ALL in the workflow job env to match your local locale."
	case "os", "arch":
		return "The workflow runs on a different platform — if the code is platform-specific, test on a matching runner or in a container."
	case "git_commit":
		return "Compare fingerprints from the same commit — re-capture after pulling."
	case "case_sensitive":
		return "Rename the file so its case matches on both sides; check `git ls-files` for names differing only by case."
	case "cpu_count", "memory_mb":
		return "Usually harmless — ignore unless the job ran out of memory, then split the job or use a larger runner."
	}
	return ""
}

// majorVersion returns the first numeric component of a version
// string ("20" for "20.11.0", "1" for "1.23.0"), or "" when it
// cannot be parsed.
func majorVersion(v string) string {
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "go")
	parts := strings.SplitN(v, ".", 2)
	if len(parts) == 0 {
		return ""
	}
	major := parts[0]
	for _, c := range major {
		if c < '0' || c > '9' {
			return ""
		}
	}
	return major
}

// runtimeSeverity ranks a version mismatch: a major version change
// is critical, a minor/patch change is a warning.
func runtimeSeverity(a, b string) string {
	ma, mb := majorVersion(a), majorVersion(b)
	if ma != "" && mb != "" && ma != mb {
		return "critical"
	}
	return "warning"
}
