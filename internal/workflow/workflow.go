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
	"fmt"
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
	// ServiceImages maps a service name to its image, e.g.
	// postgres -> "postgres:16".
	ServiceImages map[string]string `json:"service_images,omitempty"`
	// Files lists the workflow files that were parsed.
	Files []string `json:"files,omitempty"`
	// Found is false when no workflow files were found at all.
	Found bool `json:"found"`
}

// ParseDir reads all *.yml / *.yaml files under dir and merges them.
func ParseDir(dir string) (*Usage, error) {
	u := &Usage{
		Runtimes:      map[string]map[string]bool{},
		Env:           map[string]bool{},
		RunsOn:        map[string]bool{},
		ServiceImages: map[string]string{},
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
	u.Services = uniqueStrings(u.Services)
	return u, nil
}

func uniqueStrings(in []string) []string {
	var out []string
	for i, s := range in {
		if i == 0 || s != in[i-1] {
			out = append(out, s)
		}
	}
	return out
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
	case strings.Contains(lower, "setup-bun"):
		runtime, versionKey = "bun", "bun-version"
	case strings.Contains(lower, "setup-deno"):
		runtime, versionKey = "deno", "deno-version"
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
		case "node", "go", "python", "java", "ruby", "dotnet", "bun", "deno":
			for _, v := range asStringList(raw) {
				u.addRuntime(name, strings.TrimSpace(v))
			}
		}
	}
}

func parseServices(servicesRaw interface{}, u *Usage) {
	services, _ := asMap(servicesRaw)
	for name, raw := range services {
		svc, ok := asMap(raw)
		if !ok {
			continue
		}
		if img, _ := svc["image"].(string); img != "" {
			u.Services = append(u.Services, img)
			u.ServiceImages[name] = img
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
	if strings.Contains(lower, "bun test") || strings.Contains(lower, "bun run") || strings.Contains(lower, "bun install") {
		u.addRuntime("bun", "")
	}
	if strings.Contains(lower, "mvn ") || strings.Contains(lower, "gradle") {
		u.addRuntime("java", "")
	}
}

// Step is one workflow step that can be run locally.
type Step struct {
	// Name is the step's name, or "run: <first words>" when unnamed.
	Name string
	// Run is the shell script (only for run: steps).
	Run string
	// Uses is the action reference (only for uses: steps).
	Uses string
	// Env holds the step's literal env vars. Values containing
	// ${{ ... }} expressions are omitted — LocalCI cannot resolve
	// secrets or contexts, and never guesses at them.
	Env map[string]string
	// SkippedEnv names env vars omitted because they contain
	// ${{ ... }} expressions.
	SkippedEnv []string
}

// JobInfo is what localci run needs from a workflow job.
type JobInfo struct {
	// RunsOn is the job's runs-on label.
	RunsOn string
	// Services lists the job's service images.
	Services []string
	// Step is the selected step.
	Step *Step
}

// FindStep returns a job's step by name from a workflow file.
// Only run: steps can be run locally — a uses: step returns an
// error saying so, instead of silently doing nothing.
func FindStep(file, jobName, stepName string) (*JobInfo, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var wf map[string]interface{}
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return nil, err
	}
	jobs, ok := asMap(wf["jobs"])
	if !ok {
		return nil, fmt.Errorf("no jobs in %s", file)
	}
	jobRaw, ok := jobs[jobName]
	if !ok {
		return nil, fmt.Errorf("job %q not found in %s", jobName, file)
	}
	job, ok := asMap(jobRaw)
	if !ok {
		return nil, fmt.Errorf("job %q is not a mapping in %s", jobName, file)
	}
	info := &JobInfo{}
	if runsOn, ok := job["runs-on"]; ok {
		if list := asStringList(runsOn); len(list) > 0 {
			info.RunsOn = list[0]
		}
	}
	svcUsage := &Usage{ServiceImages: map[string]string{}}
	parseServices(job["services"], svcUsage)
	info.Services = svcUsage.Services
	steps, ok := job["steps"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("job %q has no steps in %s", jobName, file)
	}
	for _, stepRaw := range steps {
		step, ok := asMap(stepRaw)
		if !ok {
			continue
		}
		name, _ := step["name"].(string)
		run, _ := step["run"].(string)
		uses, _ := step["uses"].(string)
		display := name
		if display == "" {
			display = "run: " + firstWords(run, 6)
		}
		if display != stepName && name != stepName {
			continue
		}
		if uses != "" {
			return nil, fmt.Errorf("step %q uses %s — localci run only runs `run:` steps, not actions", display, uses)
		}
		if run == "" {
			return nil, fmt.Errorf("step %q has no `run:` script in %s", display, file)
		}
		s := &Step{Name: display, Run: run, Uses: uses, Env: map[string]string{}}
		if env, ok := asMap(step["env"]); ok {
			for k, v := range env {
				sv, _ := v.(string)
				if strings.Contains(sv, "${{") {
					s.SkippedEnv = append(s.SkippedEnv, k)
					continue
				}
				s.Env[k] = sv
			}
			sort.Strings(s.SkippedEnv)
		}
		info.Step = s
		return info, nil
	}
	return nil, fmt.Errorf("step %q not found in job %q in %s", stepName, jobName, file)
}

func firstWords(s string, n int) string {
	fields := strings.Fields(s)
	if len(fields) > n {
		fields = fields[:n]
	}
	return strings.Join(fields, " ")
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
