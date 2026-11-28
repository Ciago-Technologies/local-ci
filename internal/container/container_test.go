package container

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ciago-Technologies/local-ci/internal/fingerprint"
)

func TestResolveImage_Precedence(t *testing.T) {
	fp := &fingerprint.Fingerprint{RunnerImage: "ubuntu24/20251123.1", ContainerImage: "node:20-alpine"}
	cases := []struct {
		name  string
		opts  Options
		want  string
		wantS string
	}{
		{"flag wins", Options{Image: "alpine:3.19", Fingerprint: fp, RunsOn: "ubuntu-latest"}, "alpine:3.19", "flag"},
		{"runner image", Options{Fingerprint: fp}, "ubuntu:24.04", "runner image"},
		{"dockerfile from fingerprint", Options{Fingerprint: &fingerprint.Fingerprint{ContainerImage: "node:20-alpine"}}, "node:20-alpine", "Dockerfile"},
		{"local dockerfile", Options{DockerfileImage: "golang:1.23"}, "golang:1.23", "Dockerfile"},
		{"runs-on ubuntu-22.04", Options{RunsOn: "ubuntu-22.04"}, "ubuntu:22.04", "runs-on"},
		{"runs-on ubuntu-latest", Options{RunsOn: "ubuntu-latest"}, "ubuntu:24.04", "runs-on"},
		{"default", Options{}, "ubuntu:24.04", "default"},
	}
	for _, c := range cases {
		got, src, err := ResolveImage(c.opts)
		if err != nil {
			t.Errorf("%s: ResolveImage error: %v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: image = %q, want %q", c.name, got, c.want)
		}
		if !strings.Contains(src, c.wantS) {
			t.Errorf("%s: source = %q, want it to mention %q", c.name, src, c.wantS)
		}
	}
}

func TestResolveImage_MacOSWindowsError(t *testing.T) {
	for _, runsOn := range []string{"macos-latest", "windows-latest"} {
		if _, _, err := ResolveImage(Options{RunsOn: runsOn}); err == nil {
			t.Errorf("runs-on %s: want error, got none", runsOn)
		}
	}
}

func TestBuildArgs(t *testing.T) {
	dir := t.TempDir()
	opts := Options{
		Dir:     dir,
		Env:     []string{"NODE_ENV", "STRIPE_KEY=sk_test_123"},
		Command: []string{"npm", "test", "--", "--watchAll=false"},
	}
	args, err := BuildArgs(opts, "ubuntu:24.04")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	abs, _ := filepath.Abs(dir)
	for _, want := range []string{abs + ":/work", "-w", "/work", "--rm", "ubuntu:24.04", "sh", "-c"} {
		if !strings.Contains(joined, want) {
			t.Errorf("docker args %q missing %q", joined, want)
		}
	}
	// Env values must be passed to docker (that is the point of
	// --env), but DryRunString must mask them.
	s, err := DryRunString(opts, "ubuntu:24.04")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(s, "sk_test_123") {
		t.Errorf("dry-run output leaks an env value: %s", s)
	}
	if !strings.Contains(s, "STRIPE_KEY=***") {
		t.Errorf("dry-run output should mask the value: %s", s)
	}
}

func TestBuildArgs_Script(t *testing.T) {
	// A workflow step's run: block is a whole script, passed to
	// sh -c verbatim — never quoted as one word.
	opts := Options{
		Dir:     t.TempDir(),
		Command: []string{"npm test -- --watchAll=false"},
		Script:  true,
	}
	args, err := BuildArgs(opts, "ubuntu:24.04")
	if err != nil {
		t.Fatal(err)
	}
	last := args[len(args)-1]
	if last != "npm test -- --watchAll=false" {
		t.Errorf("sh -c arg = %q, want the script verbatim", last)
	}
}

func TestRun_DryRun(t *testing.T) {
	var out bytes.Buffer
	code, err := Run(Options{Dir: t.TempDir(), DryRun: true, Command: []string{"npm", "test"}}, &out, &bytes.Buffer{})
	if err != nil || code != 0 {
		t.Fatalf("dry run code=%d err=%v", code, err)
	}
	if !strings.Contains(out.String(), "docker run") || !strings.Contains(out.String(), "ubuntu:24.04") {
		t.Errorf("dry run output = %q", out.String())
	}
}

func TestServiceContainerName(t *testing.T) {
	cases := map[string]string{
		"postgres:16":                   "localci-postgres",
		"redis:7-alpine":                "localci-redis",
		"db=postgres:16":                "localci-db",
		"docker.io/library/postgres:16": "localci-postgres",
	}
	for in, want := range cases {
		if got := ServiceContainerName(in); got != want {
			t.Errorf("ServiceContainerName(%q) = %q, want %q", in, got, want)
		}
	}
	if got := ServiceImage("db=postgres:16"); got != "postgres:16" {
		t.Errorf("ServiceImage(db=postgres:16) = %q, want postgres:16", got)
	}
}

func TestRun_DryRunWithService(t *testing.T) {
	var out bytes.Buffer
	code, err := Run(Options{Dir: t.TempDir(), DryRun: true, Services: []string{"postgres:16"}, Command: []string{"npm", "test"}}, &out, &bytes.Buffer{})
	if err != nil || code != 0 {
		t.Fatalf("dry run with service code=%d err=%v", code, err)
	}
	s := out.String()
	for _, want := range []string{"docker run -d --name localci-postgres postgres:16", "docker run", "left running", "docker rm -f localci-postgres"} {
		if !strings.Contains(s, want) {
			t.Errorf("dry run with service output missing %q:\n%s", want, s)
		}
	}
}

func TestRun_WithServiceStartsItFirst(t *testing.T) {
	// Fake docker: record args, exit 0 for -d runs, exit 3 for the main run.
	bin := t.TempDir()
	script := "#!/bin/sh\necho \"FAKE-DOCKER $@\" >> \"" + filepath.Join(bin, "calls.txt") + "\"\nif [ \"$2\" = \"-d\" ]; then exit 0; fi\nexit 3\n"
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	var out, errOut bytes.Buffer
	code, err := Run(Options{Dir: t.TempDir(), Services: []string{"postgres:16"}, Command: []string{"npm", "test"}}, &out, &errOut)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}
	calls, _ := os.ReadFile(filepath.Join(bin, "calls.txt"))
	if !strings.Contains(string(calls), "run -d --name localci-postgres postgres:16") {
		t.Errorf("docker calls missing service start: %s", calls)
	}
	if !strings.Contains(errOut.String(), "localci-postgres") {
		t.Errorf("stderr missing service reminder: %q", errOut.String())
	}
}

func TestRun_MissingDocker(t *testing.T) {
	// Empty PATH: no docker binary to find.
	t.Setenv("PATH", t.TempDir())
	_, err := Run(Options{Dir: t.TempDir(), Command: []string{"true"}}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("want ErrDockerMissing, got nil")
	}
	if !strings.Contains(err.Error(), "docker is required") {
		t.Errorf("error = %q, want docker guidance", err.Error())
	}
}

func TestRun_ExitCodePassthrough(t *testing.T) {
	// A fake `docker` that records its args and exits 3.
	bin := t.TempDir()
	script := "#!/bin/sh\necho \"FAKE-DOCKER $@\" >&2\nexit 3\n"
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	var out, errOut bytes.Buffer
	code, err := Run(Options{Dir: t.TempDir(), Command: []string{"npm", "test"}}, &out, &errOut)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if code != 3 {
		t.Errorf("exit code = %d, want 3 (container's code)", code)
	}
	if !strings.Contains(errOut.String(), "FAKE-DOCKER") {
		t.Errorf("docker was not invoked: %q", errOut.String())
	}
}
