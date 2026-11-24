package fingerprint

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCapture_DoesNotIncludeSecretValues(t *testing.T) {
	t.Setenv("LOCALCI_TEST_SECRET", "super-secret-value-12345")
	t.Setenv("STRIPE_KEY", "sk_test_should_never_appear")

	dir := t.TempDir()
	fp, err := Capture(dir)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(fp)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "super-secret-value-12345") {
		t.Error("fingerprint JSON contains an env var value")
	}
	if strings.Contains(string(data), "sk_test_should_never_appear") {
		t.Error("fingerprint JSON contains an env var value")
	}
	found := false
	for _, n := range fp.EnvNames {
		if n == "LOCALCI_TEST_SECRET" {
			found = true
		}
	}
	if !found {
		t.Error("env var name LOCALCI_TEST_SECRET was not recorded")
	}
	if fp.OS == "" || fp.Arch == "" || fp.Timezone == "" {
		t.Errorf("fingerprint missing OS/arch/timezone: %+v", fp)
	}
	if fp.Runtimes["go"] == "" {
		t.Error("go runtime version was not recorded")
	}
	if fp.CPUCount <= 0 {
		t.Errorf("cpu_count = %d, want > 0", fp.CPUCount)
	}
}

func TestCapture_LockfileHash(t *testing.T) {
	dir := t.TempDir()
	content := []byte(`{"name": "x", "lockfileVersion": 3}`)
	if err := os.WriteFile(filepath.Join(dir, "package-lock.json"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	fp, err := Capture(dir)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	want := hex.EncodeToString(sum[:])
	if fp.Lockfiles["package-lock.json"] != want {
		t.Errorf("lockfile hash = %q, want %q", fp.Lockfiles["package-lock.json"], want)
	}
	// A non-lockfile must not be hashed.
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	fp2, err := Capture(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := fp2.Lockfiles["package.json"]; ok {
		t.Error("package.json was treated as a lockfile")
	}
}

// stubRuntime writes a fake runtime binary into dir that prints out
// when invoked with args.
func stubRuntime(t *testing.T, dir, name, out string) {
	t.Helper()
	script := "#!/bin/sh\nif [ \"$1\" = \"-version\" ]; then echo '" + strings.ReplaceAll(out, "'", "'\\''") + "' >&2; else echo '" + strings.ReplaceAll(out, "'", "'\\''") + "'; fi\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestCapture_ExtraRuntimes(t *testing.T) {
	bin := t.TempDir()
	stubRuntime(t, bin, "rustc", "rustc 1.83.0 (90b35a623 2024-11-26)")
	stubRuntime(t, bin, "java", `openjdk version "17.0.9" 2024-01-16`)
	stubRuntime(t, bin, "ruby", "ruby 3.3.0p0 (2023-12-25 revision 5124f9ac75)")
	stubRuntime(t, bin, "dotnet", "8.0.100")
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))

	fp, err := Capture(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"rust": "1.83.0", "java": "17.0.9", "ruby": "3.3.0", "dotnet": "8.0.100"}
	for k, v := range want {
		if fp.Runtimes[k] != v {
			t.Errorf("runtime %s = %q, want %q (all: %v)", k, fp.Runtimes[k], v, fp.Runtimes)
		}
	}
}

func TestCapture_OSReleaseAndDockerfile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM node:20-alpine AS build\nRUN npm ci\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fp, err := Capture(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fp.ContainerImage != "node:20-alpine" {
		t.Errorf("ContainerImage = %q, want node:20-alpine", fp.ContainerImage)
	}
	// This VM is Linux; /etc/os-release must parse to something.
	if fp.OSRelease == "" {
		t.Error("OSRelease is empty on Linux")
	}
}

func TestCapture_ServicesFromWorkflow(t *testing.T) {
	dir := t.TempDir()
	wfDir := filepath.Join(dir, ".github", "workflows")
	if err := os.MkdirAll(wfDir, 0o755); err != nil {
		t.Fatal(err)
	}
	wf := "name: CI\non: [push]\njobs:\n  test:\n    runs-on: ubuntu-latest\n    services:\n      postgres:\n        image: postgres:16\n      redis:\n        image: redis:7-alpine\n    steps:\n      - run: npm test\n"
	if err := os.WriteFile(filepath.Join(wfDir, "ci.yml"), []byte(wf), 0o644); err != nil {
		t.Fatal(err)
	}
	fp, err := Capture(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fp.Services["postgres"] != "postgres:16" || fp.Services["redis"] != "redis:7-alpine" {
		t.Errorf("Services = %v, want postgres:16 and redis:7-alpine", fp.Services)
	}
}

func TestCapture_GitFacts(t *testing.T) {
	dir := t.TempDir()
	// Not a git repo: all empty/zero.
	fp, err := Capture(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fp.LineEndings != "" || fp.ExecutableFiles != 0 || fp.Symlinks != 0 {
		t.Errorf("non-repo git facts = %q/%d/%d, want empty/0/0", fp.LineEndings, fp.ExecutableFiles, fp.Symlinks)
	}
}

func TestCapture_CaseSensitivityRuns(t *testing.T) {
	dir := t.TempDir()
	fp, err := Capture(dir)
	if err != nil {
		t.Fatal(err)
	}
	// No assertion on the value itself — it depends on the filesystem.
	// The test proves capture doesn't crash and leaves no temp files behind.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".localci-case-") {
			t.Errorf("capture left temp dir behind: %s", e.Name())
		}
	}
	_ = fp
}
