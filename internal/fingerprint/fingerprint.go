// Package fingerprint defines the environment fingerprint LocalCI
// captures locally and in CI, and compares with diff.
//
// Privacy rule: env var names only, never values — unless the user
// explicitly opts in with --include-values for specific, safe vars
// (e.g. NODE_ENV, TZ). Values are never collected by default, and
// there is no way to opt in to all values at once.
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

	// EnvValues holds values for vars the user explicitly opted in
	// to with --include-values (e.g. NODE_ENV, TZ). Empty by default.
	// Never put a secret here — the flag takes a short explicit list.
	EnvValues map[string]string `json:"env_values,omitempty"`

	// Lockfiles maps a lockfile path to its SHA-256 hash.
	Lockfiles map[string]string `json:"lockfiles,omitempty"`

	// CaseSensitive reports whether the filesystem distinguishes
	// "README.md" from "readme.md".
	CaseSensitive bool `json:"case_sensitive"`

	// CPUCount is runtime.NumCPU() — logical CPUs visible to the process.
	CPUCount int `json:"cpu_count,omitempty"`
	// MemoryMB is total system memory in MiB, 0 when it cannot be determined.
	MemoryMB int64 `json:"memory_mb,omitempty"`

	// RunnerImage is the CI runner image, e.g. "ubuntu24/20251123.1".
	// GitHub-hosted runners expose it via the ImageOS / ImageVersion
	// env vars; on a laptop it is empty. GitHub updates runner images
	// regularly — when it changes, the baseline goes stale even if
	// nothing in the repo changed.
	RunnerImage string `json:"runner_image,omitempty"`

	// OSRelease is the OS distribution release, e.g. "ubuntu 24.04"
	// (from /etc/os-release) or "macos 15.1" (from sw_vers). It
	// distinguishes ubuntu-22.04 from 24.04, which plain GOOS cannot.
	OSRelease string `json:"os_release,omitempty"`

	// Services maps a service name to its image, e.g.
	// postgres -> "postgres:16", taken from the repo's
	// .github/workflows services: blocks. It records what the
	// workflow asks for, not a live probe of a running server.
	Services map[string]string `json:"services,omitempty"`

	// ContainerImage is the base image from the repo's Dockerfile
	// (the first FROM line), e.g. "node:20-alpine". Empty when the
	// repo has no Dockerfile.
	ContainerImage string `json:"container_image,omitempty"`

	// LineEndings is "lf", "crlf", or "mixed", sampled from tracked
	// text files. Empty when the repo is not a git checkout.
	LineEndings string `json:"line_endings,omitempty"`
	// ExecutableFiles is how many tracked files are executable.
	ExecutableFiles int `json:"executable_files,omitempty"`
	// Symlinks is how many tracked files are symlinks.
	Symlinks int `json:"symlinks,omitempty"`

	// LockfilePackages maps a lockfile path to package -> version,
	// e.g. "package-lock.json" -> {"react": "18.2.0"}. The hash in
	// Lockfiles says a lockfile changed; this says which packages.
	LockfilePackages map[string]map[string]string `json:"lockfile_packages,omitempty"`
}
