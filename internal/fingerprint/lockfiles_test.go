package fingerprint

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParsePackageLockJSON(t *testing.T) {
	data := []byte(`{"name":"x","lockfileVersion":3,"packages":{
		"":{"name":"x","version":"1.0.0"},
		"node_modules/react":{"version":"18.2.0"},
		"node_modules/@scope/pkg":{"version":"2.0.1"},
		"node_modules/a/node_modules/b":{"version":"0.1.0"}}}`)
	pkgs := parseLockfile("package-lock.json", data)
	want := map[string]string{"react": "18.2.0", "@scope/pkg": "2.0.1", "b": "0.1.0"}
	for k, v := range want {
		if pkgs[k] != v {
			t.Errorf("package-lock %s = %q, want %q (got %v)", k, pkgs[k], v, pkgs)
		}
	}
}

func TestParsePackageLockJSONLegacy(t *testing.T) {
	data := []byte(`{"dependencies":{
		"react":{"version":"18.2.0","dependencies":{"loose-envify":{"version":"1.4.0"}}},
		"lodash":{"version":"4.17.21"}}}`)
	pkgs := parseLockfile("package-lock.json", data)
	if pkgs["react"] != "18.2.0" || pkgs["loose-envify"] != "1.4.0" || pkgs["lodash"] != "4.17.21" {
		t.Errorf("legacy package-lock = %v", pkgs)
	}
}

func TestParseYarnLock(t *testing.T) {
	data := []byte("\"react@^18.0.0\":\n  version \"18.2.0\"\n  resolved \"https://example.com\"\n\"lodash@^4.0.0\", \"lodash@~4.17.0\":\n  version \"4.17.21\"\n")
	pkgs := parseLockfile("yarn.lock", data)
	if pkgs["react"] != "18.2.0" || pkgs["lodash"] != "4.17.21" {
		t.Errorf("yarn.lock = %v", pkgs)
	}
}

func TestParsePnpmLock(t *testing.T) {
	data := []byte("lockfileVersion: '9.0'\npackages:\n  /react@18.2.0:\n    resolution: {}\n  /@scope/pkg@2.0.1:\n    resolution: {}\n")
	pkgs := parseLockfile("pnpm-lock.yaml", data)
	if pkgs["react"] != "18.2.0" || pkgs["@scope/pkg"] != "2.0.1" {
		t.Errorf("pnpm-lock = %v", pkgs)
	}
}

func TestParseTomlPackages(t *testing.T) {
	data := []byte("[[package]]\nname = \"serde\"\nversion = \"1.0.210\"\n\n[[package]]\nname = \"tokio\"\nversion = \"1.40.0\"\n")
	pkgs := parseLockfile("Cargo.lock", data)
	if pkgs["serde"] != "1.0.210" || pkgs["tokio"] != "1.40.0" {
		t.Errorf("Cargo.lock = %v", pkgs)
	}
	if pkgs2 := parseLockfile("poetry.lock", data); len(pkgs2) != 2 {
		t.Errorf("poetry.lock = %v", pkgs2)
	}
}

func TestParseGemfileLock(t *testing.T) {
	data := []byte("GEM\n  specs:\n    rails (7.1.0)\n    puma (6.4.0)\n\nPLATFORMS\n  ruby\n")
	pkgs := parseLockfile("Gemfile.lock", data)
	if pkgs["rails"] != "7.1.0" || pkgs["puma"] != "6.4.0" {
		t.Errorf("Gemfile.lock = %v", pkgs)
	}
}

func TestParseComposerLock(t *testing.T) {
	data := []byte(`{"packages":[{"name":"laravel/framework","version":"v11.0.0"}],"packages-dev":[{"name":"phpunit/phpunit","version":"10.5.0"}]}`)
	pkgs := parseLockfile("composer.lock", data)
	if pkgs["laravel/framework"] != "11.0.0" || pkgs["phpunit/phpunit"] != "10.5.0" {
		t.Errorf("composer.lock = %v", pkgs)
	}
}

func TestParseGoSum(t *testing.T) {
	data := []byte("github.com/spf13/cobra v1.8.1 h1:abc=\ngithub.com/spf13/cobra v1.8.1/go.mod h1:def=\ngithub.com/spf13/pflag v1.0.5 h1:ghi=\n")
	pkgs := parseLockfile("go.sum", data)
	if pkgs["github.com/spf13/cobra"] != "v1.8.1" || pkgs["github.com/spf13/pflag"] != "v1.0.5" {
		t.Errorf("go.sum = %v", pkgs)
	}
}

func TestParsePipfileLock(t *testing.T) {
	data := []byte(`{"default":{"requests":{"version":"==2.32.0"}},"develop":{"pytest":{"version":"==8.0.0"}}}`)
	pkgs := parseLockfile("Pipfile.lock", data)
	if pkgs["requests"] != "2.32.0" || pkgs["pytest"] != "8.0.0" {
		t.Errorf("Pipfile.lock = %v", pkgs)
	}
}

func TestLockfilePackages_Walk(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte(`{"packages":{"node_modules/react":{"version":"18.2.0"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Cargo.lock"), []byte("[[package]]\nname = \"serde\"\nversion = \"1.0.210\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pkgs := lockfilePackages(dir)
	if pkgs["package-lock.json"]["react"] != "18.2.0" {
		t.Errorf("walk package-lock = %v", pkgs)
	}
	if pkgs["Cargo.lock"]["serde"] != "1.0.210" {
		t.Errorf("walk Cargo.lock = %v", pkgs)
	}
}
