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

func TestFindStep(t *testing.T) {
	info, err := FindStep("testdata/run.yml", "test", "Test")
	if err != nil {
		t.Fatalf("FindStep: %v", err)
	}
	if info.RunsOn != "ubuntu-latest" {
		t.Errorf("RunsOn = %q, want ubuntu-latest", info.RunsOn)
	}
	if len(info.Services) != 1 || info.Services[0] != "postgres:16" {
		t.Errorf("Services = %v, want [postgres:16]", info.Services)
	}
	if info.Step.Run != "npm test -- --watchAll=false" {
		t.Errorf("Step.Run = %q", info.Step.Run)
	}
	if info.Step.Env["NODE_ENV"] != "test" {
		t.Errorf("Step.Env = %v, want NODE_ENV=test", info.Step.Env)
	}
	// ${{ secrets... }} values are skipped, never resolved.
	if _, ok := info.Step.Env["STRIPE_KEY"]; ok {
		t.Error("Step.Env must not contain STRIPE_KEY (it is a secret expression)")
	}
	if len(info.Step.SkippedEnv) != 1 || info.Step.SkippedEnv[0] != "STRIPE_KEY" {
		t.Errorf("SkippedEnv = %v, want [STRIPE_KEY]", info.Step.SkippedEnv)
	}
}

func TestFindStep_Errors(t *testing.T) {
	// uses: step cannot run locally.
	if _, err := FindStep("testdata/run.yml", "test", "Deploy"); err == nil {
		t.Error("uses: step: want error, got none")
	}
	// Missing job / step.
	if _, err := FindStep("testdata/run.yml", "nope", "Test"); err == nil {
		t.Error("missing job: want error, got none")
	}
	if _, err := FindStep("testdata/run.yml", "test", "Nope"); err == nil {
		t.Error("missing step: want error, got none")
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
