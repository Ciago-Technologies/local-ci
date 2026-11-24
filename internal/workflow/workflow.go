// Package workflow reads .github/workflows/*.yml and reports what a
// job actually uses: runtimes, env vars, runner OS, matrix values,
// and services.
//
// LocalCI's diff is workflow-aware: a mismatch only matters if some
// job uses that runtime or env var. Everything else is noise.
//
// Parsing is best-effort and fail-open: an unparsable workflow file
// is skipped, and a missing workflows directory means "everything
// is used" — never hide a real mismatch because parsing failed.
package workflow

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Usage is what the workflows in a directory collectively use.
type Usage struct {
	// Runtimes maps a runtime name ("node", "go", "python") to the
	// versions the workflows ask for, e.g. node -> {"20", "22"}.
	// Empty version set means "used, version not pinned".
	Runtimes map[string]map[string]bool `json:"runtimes"`
	// Env is the set of env var names referenced by any job or step.
	Env map[string]bool `json:"env"`
	// RunsOn lists runner labels, e.g. "ubuntu-latest".
	RunsOn map[string]bool `json:"runs_on"`
	// Services lists service images, e.g. "postgres:16".
	Services []string `json:"services,omitempty"`
	// Files lists the workflow files that were parsed.
	Files []string `json:"files,omitempty"`
	// Found is false when no workflow files were found at all.
	Found bool `json:"found"`
}

// ParseDir reads all *.yml / *.yaml files under dir and merges them.
func ParseDir(dir string) (*Usage, error) {
	u := &Usage{
		Runtimes: map[string]map[string]bool{},
		Env:      map[string]bool{},
		RunsOn:   map[string]bool{},
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return u, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		var wf map[string]interface{}
		if err := yaml.Unmarshal(data, &wf); err != nil {
			continue // fail-open: skip unparsable files
		}
		u.Found = true
		u.Files = append(u.Files, name)
		parseWorkflow(wf, u)
	}
	sort.Strings(u.Files)
	sort.Strings(u.Services)
	return u, nil
}

func parseWorkflow(wf map[string]interface{}, u *Usage) {
	// Top-level env: applies to all jobs.
	addEnvMap(u, wf["env"])
	jobs, ok := asMap(wf["jobs"])
	if !ok {
		return
	}
	for _, jobRaw := range jobs {
		job, ok := asMap(jobRaw)
		if !ok {
			continue
		}
		if runsOn, ok := job["runs-on"]; ok {
			for _, r := range asStringList(runsOn) {
				u.RunsOn[r] = true
			}
		}
		addEnvMap(u, job["env"])
		parseMatrix(job["strategy"], u)
		parseServices(job["services"], u)
		steps, ok := job["steps"].([]interface{})
		if !ok {
			continue
		}
		for _, stepRaw := range steps {
			step, ok := asMap(stepRaw)
			if !ok {
				continue
			}
			addEnvMap(u, step["env"])
			uses, _ := step["uses"].(string)
			with, _ := asMap(step["with"])
			parseUses(uses, with, u)
			if run, _ := step["run"].(string); run != "" {
				scanRunScript(run, u)
			}
		}
	}
}

// parseUses handles actions/setup-* steps, e.g.
// uses: actions/setup-node@v4, with: node-version: '20'.
func parseUses(uses string, with map[string]interface{}, u *Usage) {
	lower := strings.ToLower(uses)
	var runtime, versionKey string
	switch {
	case strings.Contains(lower, "setup-node"):
		runtime, versionKey = "node", "node-version"
	case strings.Contains(lower, "setup-go"):
		runtime, versionKey = "go", "go-version"
	case strings.Contains(lower, "setup-python"):
		runtime, versionKey = "python", "python-version"
	case strings.Contains(lower, "setup-java"):
		runtime, versionKey = "java", "java-version"
	case strings.Contains(lower, "setup-ruby"):
		runtime, versionKey = "ruby", "ruby-version"
	case strings.Contains(lower, "setup-dotnet"):
		runtime, versionKey = "dotnet", "dotnet-version"
	default:
		return
	}
	u.addRuntime(runtime, "")
	if v, ok := with[versionKey]; ok {
		for _, s := range asStringList(v) {
			u.addRuntime(runtime, strings.TrimSpace(s))
		}
	}
	// setup-node etc. also accept a version via the action name itself
	// in some setups; the `with:` block above covers the common case.
}

func (u *Usage) addRuntime(name, version string) {
	if u.Runtimes[name] == nil {
		u.Runtimes[name] = map[string]bool{}
	}
	if version != "" {
		u.Runtimes[name][version] = true
	}
}

// parseMatrix records strategy.matrix values, e.g.
// node-version: [20, 22] -> node {"20", "22"}.
func parseMatrix(strategyRaw interface{}, u *Usage) {
	strategy, _ := asMap(strategyRaw)
	matrix, ok := asMap(strategy["matrix"])
	if !ok {
		return
	}
	for key, raw := range matrix {
		name := strings.ToLower(key)
		name = strings.TrimSuffix(name, "-version")
		switch name {
		case "node", "go", "python", "java", "ruby", "dotnet":
			for _, v := range asStringList(raw) {
				u.addRuntime(name, strings.TrimSpace(v))
			}
		}
	}
}

func parseServices(servicesRaw interface{}, u *Usage) {
	services, _ := asMap(servicesRaw)
	for _, raw := range services {
		svc, ok := asMap(raw)
		if !ok {
			continue
		}
		if img, _ := svc["image"].(string); img != "" {
			u.Services = append(u.Services, img)
		}
	}
}

// scanRunScript looks for runtime invocations in a `run:` script.
// This is heuristic: `npm test` implies node, `go test` implies go,
// `pytest` / `python -m` implies python. It only ever *adds* usage,
// so a missed heuristic can only cause a false positive, never a
// hidden real mismatch.
func scanRunScript(script string, u *Usage) {
	lower := strings.ToLower(script)
	switch {
	case strings.Contains(lower, "npm "), strings.Contains(lower, "npx "), strings.Contains(lower, "node "):
		u.addRuntime("node", "")
	}
	if strings.Contains(lower, "go test") || strings.Contains(lower, "go build") || strings.Contains(lower, "go run") {
		u.addRuntime("go", "")
	}
	if strings.Contains(lower, "python ") || strings.Contains(lower, "pytest") || strings.Contains(lower, "pip install") {
		u.addRuntime("python", "")
	}
	if strings.Contains(lower, "cargo test") || strings.Contains(lower, "cargo build") {
		u.addRuntime("rust", "")
	}
	if strings.Contains(lower, "mvn ") || strings.Contains(lower, "gradle") {
		u.addRuntime("java", "")
	}
}

// UsedRuntime reports whether any workflow uses this runtime.
func (u *Usage) UsedRuntime(name string) bool {
	if u == nil || !u.Found {
		return true // fail-open: no workflows means everything counts
	}
	_, ok := u.Runtimes[strings.ToLower(name)]
	return ok
}

// UsedEnv reports whether any workflow references this env var name.
func (u *Usage) UsedEnv(name string) bool {
	if u == nil || !u.Found {
		return true
	}
	return u.Env[name]
}

// RunsOnLabels returns the runner labels the workflows ask for.
func (u *Usage) RunsOnLabels() []string {
	var out []string
	for r := range u.RunsOn {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

// HasWorkflows reports whether any workflow file was parsed at all.
func (u *Usage) HasWorkflows() bool {
	if u == nil {
		return false
	}
	return u.Found
}

func addEnvMap(u *Usage, raw interface{}) {
	m, _ := asMap(raw)
	for k := range m {
		u.Env[k] = true
	}
	// Env values can also reference other vars ($VAR / ${{ vars.X }}),
	// but names are what diff compares, so keys are enough here.
}

func asMap(v interface{}) (map[string]interface{}, bool) {
	m, ok := v.(map[string]interface{})
	return m, ok
}

func asStringList(v interface{}) []string {
	switch t := v.(type) {
	case string:
		return []string{t}
	case int:
		return []string{strconv.Itoa(t)}
	case []interface{}:
		var out []string
		for _, item := range t {
			out = append(out, asStringList(item)...)
		}
		return out
	default:
		return nil
	}
}
