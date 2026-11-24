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
