// Command localci is a local-first CI debugger.
//
//	Find out locally why CI will fail — before you push.
//
// Phase 0: CLI skeleton only. capture and diff are stubs that
// exit with a clear message until Phase 1 implements fingerprinting.
package main

import (
	"fmt"
	"os"
)

const version = "0.0.1-phase0"

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
		fmt.Fprintln(os.Stderr, "localci capture: coming in Phase 1 — fingerprinting is not implemented yet.")
		os.Exit(1)
	case "diff":
		fmt.Fprintln(os.Stderr, "localci diff: coming in Phase 1 — fingerprint comparison is not implemented yet.")
		os.Exit(1)
	default:
		fmt.Fprintf(os.Stderr, "localci: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Print(`localci — find out locally why CI will fail, before you push.

Usage:
  localci --help              Show this help
  localci --version           Show version
  localci capture             Capture an environment fingerprint (Phase 1)
  localci diff <a.json> <b.json>
                              Compare two fingerprints (Phase 1)

No secrets: LocalCI records env var names only, never values.
`)
}
