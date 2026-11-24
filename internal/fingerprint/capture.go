package fingerprint

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Known lockfiles, by base name. Values are hashed, never read for content.
var lockfileNames = map[string]bool{
	"go.sum":            true,
	"package-lock.json": true,
	"yarn.lock":         true,
	"pnpm-lock.yaml":    true,
	"poetry.lock":       true,
	"Pipfile.lock":      true,
	"Cargo.lock":        true,
	"Gemfile.lock":      true,
	"composer.lock":     true,
}

// skipDirs are never walked when looking for lockfiles.
var skipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	".localci":     true,
}

// CaptureOptions tunes what Capture records.
type CaptureOptions struct {
	// IncludeValues names env vars whose values may be recorded
	// (e.g. NODE_ENV, TZ). Names only by default; this is an
	// explicit opt-in list, never a wildcard.
	IncludeValues []string `json:"-"`
}

// Capture takes a fingerprint of dir (usually the repo root).
// It never records env var values, file contents, or secrets.
func Capture(dir string) (*Fingerprint, error) {
	return CaptureWithOptions(dir, CaptureOptions{})
}

// CaptureWithOptions is Capture with explicit options.
func CaptureWithOptions(dir string, opts CaptureOptions) (*Fingerprint, error) {
	if dir == "" {
		dir = "."
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}

	fp := &Fingerprint{
		CapturedAt:    time.Now().UTC(),
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		Timezone:      time.Now().Location().String(),
		Locale:        locale(),
		Runtimes:      runtimes(),
		EnvNames:      envNames(),
		Lockfiles:     lockfiles(abs),
		GitCommit:     gitCommit(abs),
		CaseSensitive: caseSensitive(abs),
		CPUCount:      runtime.NumCPU(),
		MemoryMB:      totalMemoryMB(),
	}
	if len(opts.IncludeValues) > 0 {
		fp.EnvValues = map[string]string{}
		for _, name := range opts.IncludeValues {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if v, ok := os.LookupEnv(name); ok {
				fp.EnvValues[name] = v
			}
		}
		if len(fp.EnvValues) == 0 {
			fp.EnvValues = nil
		}
	}
	return fp, nil
}

func locale() string {
	for _, k := range []string{"LANG", "LC_ALL", "LC_CTYPE"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

func runtimes() map[string]string {
	out := map[string]string{}
	// Go is always known: this binary's own toolchain version.
	if v := runtime.Version(); v != "" {
		out["go"] = strings.TrimPrefix(v, "go")
	}
	if v := commandVersion("node", "--version"); v != "" {
		out["node"] = strings.TrimPrefix(v, "v")
	}
	// Prefer python3, fall back to python.
	if v := commandVersion("python3", "--version"); v != "" {
		out["python"] = strings.TrimPrefix(v, "Python ")
	} else if v := commandVersion("python", "--version"); v != "" {
		out["python"] = strings.TrimPrefix(v, "Python ")
	}
	return out
}

// commandVersion runs name with args and returns the first line of
// combined output, trimmed. Empty string when the command is missing
// or fails — a missing runtime is not an error.
func commandVersion(name string, args ...string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	cmd := exec.Command(path, args...)
	out, err := cmd.Output()
	if err != nil {
		// python --version writes to stderr on old versions.
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			out = ee.Stderr
		} else {
			return ""
		}
	}
	line := strings.TrimSpace(string(out))
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	return strings.TrimSpace(line)
}

func envNames() []string {
	names := []string{}
	for _, kv := range os.Environ() {
		if i := strings.IndexByte(kv, '='); i >= 0 {
			names = append(names, kv[:i]) // name only, never the value
		}
	}
	sort.Strings(names)
	return names
}

func lockfiles(root string) map[string]string {
	out := map[string]string{}
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable subtree: skip, don't fail the capture
		}
		if d.IsDir() {
			if path != root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !lockfileNames[d.Name()] {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		if h, err := sha256File(path); err == nil {
			out[filepath.ToSlash(rel)] = h
		}
		return nil
	})
	return out
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func gitCommit(dir string) string {
	path, err := exec.LookPath("git")
	if err != nil {
		return ""
	}
	cmd := exec.Command(path, "-C", dir, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// caseSensitive reports whether dir's filesystem distinguishes case.
// It creates a temp dir inside dir, writes "LocalCITest", and tries
// to open the lower-case name. The temp dir is always removed.
func caseSensitive(dir string) bool {
	tmp, err := os.MkdirTemp(dir, ".localci-case-*")
	if err != nil {
		return true // assume the common case; don't fail the capture
	}
	defer os.RemoveAll(tmp)
	upper := filepath.Join(tmp, "LocalCITest")
	if err := os.WriteFile(upper, []byte("x"), 0o600); err != nil {
		return true
	}
	lower := filepath.Join(tmp, "localcitest")
	_, err = os.Stat(lower)
	return err != nil // if lower-case name is not found, fs is case-sensitive
}

func totalMemoryMB() int64 {
	switch runtime.GOOS {
	case "linux":
		return linuxMemoryMB()
	case "darwin":
		return darwinMemoryMB()
	default:
		return 0
	}
}

func linuxMemoryMB() int64 {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0
		}
		kb, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0
		}
		return kb / 1024
	}
	return 0
}

func darwinMemoryMB() int64 {
	out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return 0
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return 0
	}
	return n / 1024 / 1024
}
