package workflow

import (
	"testing"
)

func TestParseDir(t *testing.T) {
	u, err := ParseDir("testdata")
	if err != nil {
		t.Fatalf("ParseDir: %v", err)
	}
	if !u.Found {
		t.Fatal("expected Found=true")
	}
	if !u.UsedRuntime("node") {
		t.Error("expected node to be used (setup-node + matrix)")
	}
	if u.UsedRuntime("go") {
		t.Error("expected go NOT to be used")
	}
	if !u.UsedEnv("NODE_ENV") {
		t.Error("expected NODE_ENV to be used (job env)")
	}
	if !u.UsedEnv("STRIPE_KEY") {
		t.Error("expected STRIPE_KEY to be used (step env)")
	}
	if u.UsedEnv("UNUSED_VAR") {
		t.Error("expected UNUSED_VAR NOT to be used")
	}
	if !u.RunsOn["ubuntu-latest"] {
		t.Error("expected runs-on ubuntu-latest")
	}
	if len(u.Services) != 1 || u.Services[0] != "postgres:16" {
		t.Errorf("expected services [postgres:16], got %v", u.Services)
	}
	if u.ServiceImages["postgres"] != "postgres:16" {
		t.Errorf("expected ServiceImages[postgres]=postgres:16, got %v", u.ServiceImages)
	}
	// Matrix + setup-node both pin 20.
	if !u.Runtimes["node"]["20"] || !u.Runtimes["node"]["22"] {
		t.Errorf("expected node versions {20, 22}, got %v", u.Runtimes["node"])
	}
}

func TestParseDirMissing(t *testing.T) {
	u, err := ParseDir("testdata/does-not-exist")
	if err == nil {
		t.Fatal("expected error for missing dir")
	}
	// Fail-open: with no workflows, everything counts as used.
	if !u.UsedRuntime("node") || !u.UsedEnv("ANYTHING") {
		t.Error("fail-open: missing workflows should mark everything used")
	}
}
