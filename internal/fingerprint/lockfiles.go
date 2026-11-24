package fingerprint

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// lockfilePackages walks root (same skip rules as lockfiles) and
// returns lockfile path -> package -> version for every lockfile
// it can parse. The hash in Lockfiles says a lockfile changed;
// this says which packages changed. A lockfile that cannot be
// parsed contributes nothing — its hash still covers it.
func lockfilePackages(root string) map[string]map[string]string {
	out := map[string]map[string]string{}
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
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
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		pkgs := parseLockfile(d.Name(), data)
		if len(pkgs) == 0 {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		out[filepath.ToSlash(rel)] = pkgs
		return nil
	})
	if len(out) == 0 {
		return nil
	}
	return out
}

// parseLockfile dispatches on the lockfile's base name.
func parseLockfile(name string, data []byte) map[string]string {
	switch name {
	case "package-lock.json":
		return parsePackageLockJSON(data)
	case "yarn.lock":
		return parseYarnLock(data)
	case "pnpm-lock.yaml":
		return parsePnpmLock(data)
	case "Cargo.lock", "poetry.lock":
		return parseTomlPackages(data)
	case "Gemfile.lock":
		return parseGemfileLock(data)
	case "composer.lock":
		return parseComposerLock(data)
	case "go.sum":
		return parseGoSum(data)
	case "Pipfile.lock":
		return parsePipfileLock(data)
	default:
		return nil
	}
}

// parsePackageLockJSON handles npm's package-lock.json, both the
// modern "packages" layout and the legacy "dependencies" layout.
func parsePackageLockJSON(data []byte) map[string]string {
	var doc struct {
		Packages map[string]struct {
			Version string `json:"version"`
		} `json:"packages"`
		Dependencies map[string]struct {
			Version      string `json:"version"`
			Dependencies map[string]struct {
				Version string `json:"version"`
			} `json:"dependencies"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil
	}
	out := map[string]string{}
	for key, pkg := range doc.Packages {
		if pkg.Version == "" || key == "" {
			continue
		}
		// Key is "node_modules/<name>" (possibly nested).
		name := key
		if i := strings.LastIndex(name, "node_modules/"); i >= 0 {
			name = name[i+len("node_modules/"):]
		}
		out[name] = pkg.Version
	}
	if len(out) > 0 {
		return out
	}
	// Legacy layout: flatten nested dependencies recursively.
	type dep struct {
		Version      string         `json:"version"`
		Dependencies map[string]dep `json:"dependencies"`
	}
	var rec func(m map[string]dep)
	rec = func(m map[string]dep) {
		for name, d := range m {
			if d.Version != "" {
				out[name] = d.Version
			}
			rec(d.Dependencies)
		}
	}
	var legacy struct {
		Dependencies map[string]dep `json:"dependencies"`
	}
	// Re-parse into the recursive type (the first unmarshal's
	// Dependencies type is only one level deep).
	if err := json.Unmarshal(data, &legacy); err == nil {
		rec(legacy.Dependencies)
	}
	return out
}

// parseYarnLock handles yarn.lock (v1 format): entries look like
// "react@^18.0.0":\n  version "18.2.0".
func parseYarnLock(data []byte) map[string]string {
	out := map[string]string{}
	var name string
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && strings.HasSuffix(strings.TrimSpace(line), ":") {
			// New entry header: "name@range": or "a@^1", "a@^2":
			key := strings.TrimSpace(line)
			key = strings.TrimSuffix(key, ":")
			key = strings.Trim(key, `"`)
			first := strings.Split(key, `", "`)[0]
			first = strings.Trim(first, `"`)
			if i := strings.LastIndex(first, "@"); i > 0 {
				name = first[:i]
			} else {
				name = ""
			}
			continue
		}
		if name == "" {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "version ") {
			ver := strings.Trim(strings.TrimPrefix(trimmed, "version "), `"`)
			out[name] = ver
			name = "" // one version per entry
		}
	}
	return out
}

// parsePnpmLock handles pnpm-lock.yaml: packages are keys under
// "packages:", e.g. /react@18.2.0: or /@scope/name@1.0.0:.
func parsePnpmLock(data []byte) map[string]string {
	var doc struct {
		Packages map[string]interface{} `yaml:"packages"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil
	}
	out := map[string]string{}
	for key := range doc.Packages {
		key = strings.TrimPrefix(key, "/")
		if i := strings.LastIndex(key, "@"); i > 0 {
			out[key[:i]] = key[i+1:]
		}
	}
	return out
}

// parseTomlPackages handles Cargo.lock and poetry.lock: repeated
// [[package]] blocks with name = "..." and version = "...".
func parseTomlPackages(data []byte) map[string]string {
	out := map[string]string{}
	var name string
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "[[package]]" {
			name = ""
			continue
		}
		k, v, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"`)
		switch k {
		case "name":
			name = v
		case "version":
			if name != "" {
				out[name] = v
			}
		}
	}
	return out
}

var gemSpecRe = regexp.MustCompile(`^    ([A-Za-z0-9_.\-]+) \(([^)]+)\)$`)

// parseGemfileLock handles Gemfile.lock's GEM specs section:
// "    rails (7.1.0)".
func parseGemfileLock(data []byte) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		if m := gemSpecRe.FindStringSubmatch(line); m != nil {
			out[m[1]] = m[2]
		}
	}
	return out
}

// parseComposerLock handles composer.lock (JSON): packages and
// packages-dev arrays of {name, version}.
func parseComposerLock(data []byte) map[string]string {
	var doc struct {
		Packages    []struct{ Name, Version string } `json:"packages"`
		PackagesDev []struct{ Name, Version string } `json:"packages-dev"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil
	}
	out := map[string]string{}
	for _, p := range append(doc.Packages, doc.PackagesDev...) {
		if p.Name != "" && p.Version != "" {
			out[p.Name] = strings.TrimPrefix(p.Version, "v")
		}
	}
	return out
}

// parseGoSum handles go.sum: lines are "module version hash".
// The "/go.mod" hash lines are skipped — the plain version line
// already carries the version.
func parseGoSum(data []byte) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		mod, ver := fields[0], fields[1]
		if strings.HasSuffix(ver, "/go.mod") {
			continue
		}
		out[mod] = ver
	}
	return out
}

// parsePipfileLock handles Pipfile.lock (JSON): default/develop
// maps of name -> {"version": "==1.2.3"}.
func parsePipfileLock(data []byte) map[string]string {
	var doc struct {
		Default map[string]struct {
			Version string `json:"version"`
		} `json:"default"`
		Develop map[string]struct {
			Version string `json:"version"`
		} `json:"develop"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil
	}
	out := map[string]string{}
	for _, m := range []map[string]struct {
		Version string `json:"version"`
	}{doc.Default, doc.Develop} {
		for name, pkg := range m {
			v := strings.TrimPrefix(pkg.Version, "==")
			if v != "" {
				out[name] = v
			}
		}
	}
	return out
}
