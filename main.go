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

	"github.com/Ciago-Technologies/local-ci/internal/fingerprint"
)

const version = "0.2.0-phase2"

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
  localci capture [--dir DIR] [-o FILE]
                                  Capture an environment fingerprint as JSON.
                                  Writes to stdout unless -o is given.
  localci diff [--json] <a.json> <b.json>
                                  Compare two fingerprints. Exits 1 when
                                  differences are found, 0 when they match.

No secrets: LocalCI records env var names only, never values.
`)
}

func runCapture(args []string) int {
	fs := flag.NewFlagSet("capture", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dir := fs.String("dir", ".", "directory to fingerprint (repo root)")
	outPath := fs.String("o", "", "write JSON to FILE instead of stdout")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "localci capture: takes no positional arguments")
		return 2
	}

	fp, err := fingerprint.Capture(*dir)
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
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "localci diff: usage: localci diff [--json] <a.json> <b.json>")
		return 2
	}
	a, err := loadFingerprint(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "localci diff: %v\n", err)
		return 2
	}
	b, err := loadFingerprint(fs.Arg(1))
	if err != nil {
		fmt.Fprintf(os.Stderr, "localci diff: %v\n", err)
		return 2
	}

	diffs := fingerprint.Diff(a, b)
	if *asJSON {
		data, err := json.MarshalIndent(diffs, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "localci diff: %v\n", err)
			return 2
		}
		fmt.Println(string(data))
	} else {
		fmt.Print(fingerprint.FormatHuman(diffs, "local", "CI"))
	}
	if len(diffs) == 0 {
		return 0
	}
	return 1
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
