// Package fingerprint defines the environment fingerprint LocalCI
// captures locally and in CI, and compares with diff.
//
// Privacy rule: env var names only, never values. This is enforced
// by the type itself — there is no field for a secret value.
package fingerprint

import "time"

// Fingerprint is a snapshot of one environment: a laptop or a CI runner.
type Fingerprint struct {
	// CapturedAt is when this fingerprint was taken, in UTC.
	CapturedAt time.Time `json:"captured_at"`

	// GitCommit is the checked-out commit, if this is a git repo.
	GitCommit string `json:"git_commit,omitempty"`

	// OS is runtime.GOOS, Arch is runtime.GOARCH.
	OS   string `json:"os"`
	Arch string `json:"arch"`

	// Timezone is the local zone name, e.g. Asia/Kolkata or UTC.
	Timezone string `json:"timezone"`
	// Locale is LANG / LC_ALL as reported by the OS, if set.
	Locale string `json:"locale,omitempty"`

	// Runtimes maps a runtime name ("go", "node", "python") to its version.
	Runtimes map[string]string `json:"runtimes,omitempty"`

	// EnvNames is the sorted list of environment variable names.
	// Values are never collected.
	EnvNames []string `json:"env_names"`

	// Lockfiles maps a lockfile path to its SHA-256 hash.
	Lockfiles map[string]string `json:"lockfiles,omitempty"`

	// CaseSensitive reports whether the filesystem distinguishes
	// "README.md" from "readme.md".
	CaseSensitive bool `json:"case_sensitive"`

	// CPUCount is runtime.NumCPU() — logical CPUs visible to the process.
	CPUCount int `json:"cpu_count,omitempty"`
	// MemoryMB is total system memory in MiB, 0 when it cannot be determined.
	MemoryMB int64 `json:"memory_mb,omitempty"`
}
