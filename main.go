// Command localci is a local-first CI debugger.
//
//	Find out locally why CI will fail — before you push.
//
//	localci capture > local.json     # on your laptop
//	localci capture > ci.json        # in CI
//	localci diff local.json ci.json  # shows the exact mismatch
//
// Exit codes: 0 = no differences (diff) / success (capture),
// 1 = differences found, 2 = usage or I/O error.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/Ciago-Technologies/local-ci/internal/container"
	"github.com/Ciago-Technologies/local-ci/internal/fingerprint"
	"github.com/Ciago-Technologies/local-ci/internal/workflow"
)

const version = "0.7.3"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "--help", "-h", "help":
		usage()
	case "--version", "-v", "version":
		fmt.Printf("localci %s\n", version)
	case "capture":
		os.Exit(runCapture(os.Args[2:]))
	case "diff":
		os.Exit(runDiff(os.Args[2:]))
	case "run":
		os.Exit(runRun(os.Args[2:]))
	default:
		fmt.Fprintf(os.Stderr, "localci: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Print(`localci — find out locally why CI will fail, before you push.

Usage:
  localci --help                  Show this help
  localci --version               Show version
  localci capture [--dir DIR] [-o FILE] [--include-values VAR,...]
                                  Capture an environment fingerprint as JSON.
                                  Writes to stdout unless -o is given.
                                  --include-values records values only for
                                  the named vars (e.g. NODE_ENV,TZ).
  localci diff [--json] [--workflows DIR] [--show-unused] <a.json> <b.json>
                                  Compare two fingerprints. Exits 1 when
                                  differences are found, 0 when they match.
                                  --workflows hides differences in things
                                  no workflow job uses (false positives).
  localci diff [--json] [--baseline-dir DIR] <ci.json>
                                  Pick the baseline from DIR that best
                                  matches the CI fingerprint (matrix
                                  support: one baseline per OS/runtime
                                  cell), then compare. Warns when the
                                  baseline is older than 30 days.
  localci run [--image IMG] [--dir DIR] [--fingerprint ci.json]
              [--env NAME[=VALUE] ...] [--dry-run] -- <command...>
                                  Run one command in a Docker container
                                  matching the CI environment. The repo
                                  is mounted at /work; the exit code is
                                  the command's. Docker is required.
  localci run --workflow FILE --job JOB --step NAME [--dir DIR]
              [--env ...] [--dry-run]
                                  Run one workflow step's run: script
                                  in the matching container. Only run:
                                  steps — uses: steps can't run here.

No secrets: LocalCI records env var names only, never values,
unless you explicitly opt in with --include-values.
`)
}

func runCapture(args []string) int {
	fs := flag.NewFlagSet("capture", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dir := fs.String("dir", ".", "directory to fingerprint (repo root)")
	outPath := fs.String("o", "", "write JSON to FILE instead of stdout")
	includeValues := fs.String("include-values", "", "comma-separated env var names whose values to record (e.g. NODE_ENV,TZ)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "localci capture: takes no positional arguments")
		return 2
	}

	var opts fingerprint.CaptureOptions
	if *includeValues != "" {
		opts.IncludeValues = strings.Split(*includeValues, ",")
	}
	fp, err := fingerprint.CaptureWithOptions(*dir, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "localci capture: %v\n", err)
		return 2
	}
	data, err := json.MarshalIndent(fp, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "localci capture: %v\n", err)
		return 2
	}
	data = append(data, '\n')
	if *outPath == "" {
		_, _ = os.Stdout.Write(data)
		return 0
	}
	if err := os.WriteFile(*outPath, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "localci capture: %v\n", err)
		return 2
	}
	return 0
}

func runDiff(args []string) int {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	asJSON := fs.Bool("json", false, "print differences as JSON")
	workflowsDir := fs.String("workflows", "", "workflow directory (e.g. .github/workflows) — hide differences in things no job uses")
	showUnused := fs.Bool("show-unused", false, "with --workflows, show the hidden differences too")
	baselineDir := fs.String("baseline-dir", "", "pick the baseline from DIR that best matches the CI fingerprint (matrix support); takes one positional arg, the CI fingerprint")
	ignore := fs.String("ignore", "", "comma-separated diff fields to drop, e.g. git_commit (a baseline's commit always differs from a PR head, so the Action ignores it)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	var a, b *fingerprint.Fingerprint
	var pickedBaseline string
	var err error
	if *baselineDir != "" {
		if fs.NArg() != 1 {
			fmt.Fprintln(os.Stderr, "localci diff: usage: localci diff --baseline-dir DIR <ci.json>")
			return 2
		}
		b, err = loadFingerprint(fs.Arg(0))
		if err != nil {
			fmt.Fprintf(os.Stderr, "localci diff: %v\n", err)
			return 2
		}
		pickedBaseline, a, err = fingerprint.PickBaseline(*baselineDir, b)
		if err != nil {
			fmt.Fprintf(os.Stderr, "localci diff: %v\n", err)
			return 2
		}
		if !*asJSON {
			fmt.Fprintf(os.Stderr, "Baseline: %s\n", pickedBaseline)
		}
	} else {
		if fs.NArg() != 2 {
			fmt.Fprintln(os.Stderr, "localci diff: usage: localci diff [--json] [--workflows DIR] <a.json> <b.json>")
			return 2
		}
		a, err = loadFingerprint(fs.Arg(0))
		if err != nil {
			fmt.Fprintf(os.Stderr, "localci diff: %v\n", err)
			return 2
		}
		b, err = loadFingerprint(fs.Arg(1))
		if err != nil {
			fmt.Fprintf(os.Stderr, "localci diff: %v\n", err)
			return 2
		}
	}

	// Baseline staleness is a warning, not a difference.
	if w := fingerprint.StalenessWarning(a, fingerprint.BaselineMaxAgeDays); w != "" {
		fmt.Fprintln(os.Stderr, w)
	}

	diffs := fingerprint.Diff(a, b)
	if *ignore != "" {
		diffs = fingerprint.IgnoreFields(diffs, strings.Split(*ignore, ","))
	}
	var hidden []fingerprint.Difference
	if *workflowsDir != "" {
		usage, err := workflow.ParseDir(*workflowsDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "localci diff: warning: cannot read workflows in %s (%v) — showing all differences\n", *workflowsDir, err)
		} else if usage.HasWorkflows() {
			diffs, hidden = fingerprint.ApplyWorkflow(diffs, usage)
			if *showUnused {
				diffs = append(diffs, hidden...)
				hidden = nil
			}
		}
	}
	if *asJSON {
		data, err := json.MarshalIndent(diffs, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "localci diff: %v\n", err)
			return 2
		}
		fmt.Println(string(data))
	} else {
		fmt.Print(fingerprint.FormatHumanWithHidden(diffs, hidden, "local", "CI"))
	}
	if len(diffs) == 0 {
		return 0
	}
	return 1
}

// envFlag collects repeated --env flags.
type envFlag []string

func (e *envFlag) String() string { return strings.Join(*e, ",") }
func (e *envFlag) Set(v string) error {
	*e = append(*e, v)
	return nil
}

func runRun(args []string) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	image := fs.String("image", "", "container image (overrides all image selection)")
	dir := fs.String("dir", ".", "repo directory to mount at /work")
	fpPath := fs.String("fingerprint", "", "CI fingerprint JSON, used to pick the image")
	workflowFile := fs.String("workflow", "", "workflow file to take a step from")
	jobName := fs.String("job", "", "job name (with --workflow)")
	stepName := fs.String("step", "", "step name (with --workflow)")
	dryRun := fs.Bool("dry-run", false, "print the docker command instead of running it")
	var envs envFlag
	fs.Var(&envs, "env", "env var to pass into the container: NAME (from host) or NAME=VALUE; repeatable")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	opts := container.Options{
		Image:  *image,
		Dir:    *dir,
		Env:    []string(envs),
		DryRun: *dryRun,
	}

	if *workflowFile != "" {
		if *jobName == "" || *stepName == "" {
			fmt.Fprintln(os.Stderr, "localci run: --workflow needs --job and --step")
			return 2
		}
		if fs.NArg() != 0 {
			fmt.Fprintln(os.Stderr, "localci run: with --workflow, give no command — the step's run: script is used")
			return 2
		}
		info, err := workflow.FindStep(*workflowFile, *jobName, *stepName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "localci run: %v\n", err)
			return 2
		}
		opts.RunsOn = info.RunsOn
		opts.Command = []string{info.Step.Run}
		opts.Script = true
		for k, v := range info.Step.Env {
			opts.Env = append(opts.Env, k+"="+v)
		}
		if len(info.Step.SkippedEnv) > 0 {
			fmt.Fprintf(os.Stderr, "localci run: warning: env %s reference secrets or expressions — pass them with --env NAME=VALUE\n", strings.Join(info.Step.SkippedEnv, ", "))
		}
		if len(info.Services) > 0 {
			fmt.Fprintf(os.Stderr, "localci run: warning: job uses services (%s) — localci run does not start them; start them yourself first\n", strings.Join(info.Services, ", "))
		}
	} else {
		if fs.NArg() == 0 {
			fmt.Fprintln(os.Stderr, "localci run: usage: localci run [--image IMG] [--dry-run] -- <command...>")
			return 2
		}
		opts.Command = fs.Args()
	}

	if *fpPath != "" {
		fp, err := loadFingerprint(*fpPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "localci run: %v\n", err)
			return 2
		}
		opts.Fingerprint = fp
	}
	if opts.DockerfileImage == "" {
		// Local Dockerfile, for image selection when nothing else applies.
		opts.DockerfileImage = fingerprint.DockerfileBase(*dir)
	}

	code, err := container.Run(opts, os.Stdout, os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "localci run: %v\n", err)
		return 2
	}
	return code
}

func loadFingerprint(path string) (*fingerprint.Fingerprint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var fp fingerprint.Fingerprint
	if err := json.Unmarshal(data, &fp); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &fp, nil
}
