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

	"github.com/Ciago-Technologies/local-ci/internal/workflow"
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

	le, execs, syms := gitFacts(abs)
	fp := &Fingerprint{
		CapturedAt:       time.Now().UTC(),
		OS:               runtime.GOOS,
		Arch:             runtime.GOARCH,
		Timezone:         time.Now().Location().String(),
		Locale:           locale(),
		Runtimes:         runtimes(),
		EnvNames:         envNames(),
		Lockfiles:        lockfiles(abs),
		GitCommit:        gitCommit(abs),
		CaseSensitive:    caseSensitive(abs),
		CPUCount:         runtime.NumCPU(),
		MemoryMB:         totalMemoryMB(),
		RunnerImage:      runnerImage(),
		OSRelease:        osRelease(),
		Services:         servicesFromWorkflows(abs),
		ContainerImage:   dockerfileBase(abs),
		LineEndings:      le,
		ExecutableFiles:  execs,
		Symlinks:         syms,
		LockfilePackages: lockfilePackages(abs),
		Tools:            tools(),
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

// runnerImage reads the CI runner image from the environment.
// GitHub-hosted runners set ImageOS (e.g. "ubuntu24") and
// ImageVersion (e.g. "20251123.1"). Empty on a laptop or on
// self-hosted runners that don't set these vars.
func runnerImage() string {
	imageOS := os.Getenv("ImageOS")
	if imageOS == "" {
		return ""
	}
	if v := os.Getenv("ImageVersion"); v != "" {
		return imageOS + "/" + v
	}
	return imageOS
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
	// rustc --version prints "rustc 1.83.0 (.....)".
	if v := commandVersion("rustc", "--version"); v != "" {
		if ver := versionWord(v, "rustc"); ver != "" {
			out["rust"] = ver
		}
	}
	// java -version prints to stderr: openjdk version "17.0.9" ...
	if v := commandVersion("java", "-version"); v != "" {
		if ver := quotedVersion(v); ver != "" {
			out["java"] = ver
		}
	}
	// ruby --version prints "ruby 3.3.0p0 (.....) ...".
	if v := commandVersion("ruby", "--version"); v != "" {
		if ver := versionWord(v, "ruby"); ver != "" {
			out["ruby"] = strings.TrimSuffix(ver, "p0")
		}
	}
	// dotnet --version prints "8.0.100".
	if v := commandVersion("dotnet", "--version"); v != "" {
		out["dotnet"] = v
	}
	// bun --version prints just the version, e.g. "1.2.19".
	if v := commandVersion("bun", "--version"); v != "" {
		out["bun"] = strings.TrimPrefix(v, "v")
	}
	// deno --version prints "deno 2.1.4 (stable, ...)".
	if v := commandVersion("deno", "--version"); v != "" {
		if ver := versionWord(v, "deno"); ver != "" {
			out["deno"] = ver
		}
	}
	return out
}

// tools records versions of common CI tools that are on PATH.
// Scoped to a short allowlist — a full package inventory is
// deliberately not done (too big, too noisy).
func tools() map[string]string {
	out := map[string]string{}
	if v := commandVersion("git", "--version"); v != "" {
		if ver := versionWord(v, "git"); ver != "" {
			out["git"] = ver
		}
	}
	if v := commandVersion("docker", "--version"); v != "" {
		// "Docker version 24.0.7, build ..." -> "24.0.7"
		if ver := versionWord(v, "version"); ver != "" {
			out["docker"] = strings.TrimSuffix(ver, ",")
		}
	}
	if v := commandVersion("make", "--version"); v != "" {
		// "GNU Make 4.3" -> "4.3"
		if ver := versionWord(v, "Make"); ver != "" {
			out["make"] = ver
		}
	}
	if v := commandVersion("gcc", "--version"); v != "" {
		if ver := versionWord(v, "gcc"); ver != "" {
			out["gcc"] = ver
		}
	}
	if v := commandVersion("curl", "--version"); v != "" {
		if ver := versionWord(v, "curl"); ver != "" {
			out["curl"] = ver
		}
	}
	return out
}

// versionWord returns the word after prefix in s, e.g.
// versionWord("rustc 1.83.0 (x)", "rustc") == "1.83.0".
func versionWord(s, prefix string) string {
	fields := strings.Fields(s)
	for i, f := range fields {
		if f == prefix && i+1 < len(fields) {
			return fields[i+1]
		}
	}
	return ""
}

// quotedVersion returns the first double-quoted string in s, e.g.
// quotedVersion(`openjdk version "17.0.9" 2024-01-16`) == "17.0.9".
func quotedVersion(s string) string {
	start := strings.IndexByte(s, '"')
	if start < 0 {
		return ""
	}
	rest := s[start+1:]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// commandVersion runs name with args and returns the first line of
// combined output (stdout + stderr), trimmed. Several runtimes
// report their version on stderr (java, old python), so stdout
// alone is not enough. Empty string when the command is missing
// or fails — a missing runtime is not an error.
func commandVersion(name string, args ...string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	cmd := exec.Command(path, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(out))
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	return strings.TrimSpace(line)
}

// osRelease returns the OS distribution release, e.g. "ubuntu 24.04".
// Linux reads /etc/os-release (ID + VERSION_ID); macOS uses sw_vers.
// Empty when it cannot be determined.
func osRelease() string {
	switch runtime.GOOS {
	case "linux":
		data, err := os.ReadFile("/etc/os-release")
		if err != nil {
			return ""
		}
		var id, ver string
		for _, line := range strings.Split(string(data), "\n") {
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			v = strings.Trim(v, `"`)
			switch k {
			case "ID":
				id = v
			case "VERSION_ID":
				ver = v
			}
		}
		if id == "" {
			return ""
		}
		if ver != "" {
			return id + " " + ver
		}
		return id
	case "darwin":
		name := commandVersion("sw_vers", "-productName")
		ver := commandVersion("sw_vers", "-productVersion")
		if name == "" {
			return ""
		}
		return strings.ToLower(name) + " " + ver
	default:
		return ""
	}
}

// servicesFromWorkflows reads the repo's .github/workflows services:
// blocks and returns service name -> image, e.g. postgres ->
// "postgres:16". It records what the workflow asks for, not a live
// probe — a stopped local database has no version to report.
func servicesFromWorkflows(root string) map[string]string {
	usage, err := workflow.ParseDir(filepath.Join(root, ".github", "workflows"))
	if err != nil || !usage.HasWorkflows() {
		return nil
	}
	if len(usage.ServiceImages) == 0 {
		return nil
	}
	return usage.ServiceImages
}

// DockerfileBase returns the base image from dir's Dockerfile
// (the first FROM line), e.g. "node:20-alpine". Empty when there
// is no Dockerfile. Used by `localci run` for image selection.
func DockerfileBase(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	return dockerfileBase(abs)
}

// dockerfileBase returns the base image from the repo's Dockerfile
// (the first FROM line), e.g. "node:20-alpine". Empty when there
// is no Dockerfile.
func dockerfileBase(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "Dockerfile"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToUpper(line), "FROM ") {
			continue
		}
		rest := strings.TrimSpace(line[len("FROM "):])
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			return ""
		}
		img := fields[0]
		// FROM [--platform=...] image [AS name]
		if strings.HasPrefix(img, "--platform=") && len(fields) > 1 {
			img = fields[1]
		}
		return img
	}
	return ""
}

// gitFacts samples tracked files for line endings and counts
// executable files and symlinks. It returns ("", 0, 0) when the
// directory is not a git checkout or git is unavailable.
func gitFacts(dir string) (lineEndings string, execFiles, symlinks int) {
	git, err := exec.LookPath("git")
	if err != nil {
		return "", 0, 0
	}
	out, err := exec.Command(git, "-C", dir, "ls-files", "-s").Output()
	if err != nil {
		return "", 0, 0
	}
	var files []string
	for _, line := range strings.Split(string(out), "\n") {
		// Format: "<mode> <sha> <stage>\t<path>"
		tab := strings.IndexByte(line, '\t')
		if tab < 0 {
			continue
		}
		mode := strings.Fields(line[:tab])
		if len(mode) == 0 {
			continue
		}
		switch mode[0] {
		case "120000":
			symlinks++
		case "100755":
			execFiles++
		}
		files = append(files, line[tab+1:])
	}
	if len(files) == 0 {
		return "", execFiles, symlinks
	}
	// Sample up to 100 files, first 8 KiB each, for line endings.
	var sawLF, sawCRLF bool
	checked := 0
	for _, f := range files {
		if checked >= 100 {
			break
		}
		// Skip likely-binary paths by extension.
		ext := strings.ToLower(filepath.Ext(f))
		switch ext {
		case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".ico", ".pdf",
			".zip", ".gz", ".tar", ".xz", ".woff", ".woff2", ".ttf", ".otf", ".mp3", ".mp4":
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil || len(data) == 0 {
			continue
		}
		checked++
		if len(data) > 8192 {
			data = data[:8192]
		}
		if strings.IndexByte(string(data), 0) >= 0 {
			continue // binary content
		}
		if strings.Contains(string(data), "\r\n") {
			sawCRLF = true
		}
		if strings.Contains(string(data), "\n") {
			// A lone LF: strip CRLFs first, then look for \n.
			stripped := strings.ReplaceAll(string(data), "\r\n", "")
			if strings.Contains(stripped, "\n") {
				sawLF = true
			}
		}
		if sawLF && sawCRLF {
			break
		}
	}
	switch {
	case sawLF && sawCRLF:
		lineEndings = "mixed"
	case sawCRLF:
		lineEndings = "crlf"
	case sawLF:
		lineEndings = "lf"
	}
	return lineEndings, execFiles, symlinks
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
