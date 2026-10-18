package provider

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeModulesJSON(t *testing.T, root string, modules []struct {
	Key     string `json:"Key"`
	Source  string `json:"Source"`
	Version string `json:"Version"`
	Dir     string `json:"Dir"`
}) {
	t.Helper()
	dir := filepath.Join(root, ".terraform", "modules")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	raw, err := json.Marshal(modulesJSON{Modules: modules})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "modules.json"), raw, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// newTestRoot returns a symlink-resolved temp dir - on macOS, t.TempDir()
// is under /tmp, a symlink to /private/tmp, and os.Getwd() after chdir
// resolves it, which would otherwise make absModulePath (built from the
// pre-chdir path) mismatch what detectOrigin computes internally.
func newTestRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	return resolved
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
}

func TestDetectOrigin_RegistrySource_UsesModulesJSONVersion(t *testing.T) {
	root := newTestRoot(t)
	modDir := filepath.Join(root, "mod")
	if err := os.MkdirAll(modDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeModulesJSON(t, root, []struct {
		Key     string `json:"Key"`
		Source  string `json:"Source"`
		Version string `json:"Version"`
		Dir     string `json:"Dir"`
	}{
		{Key: "x", Source: "registry.terraform.io/rafalmasiarek/tagger", Version: "0.7.1", Dir: "mod"},
	})
	chdir(t, root)

	source, version := detectOrigin(context.Background(), modDir)
	if source != "registry.terraform.io/rafalmasiarek/tagger" {
		t.Errorf("source = %q", source)
	}
	if version != "0.7.1" {
		t.Errorf("version = %q, want 0.7.1", version)
	}
}

func TestDetectOrigin_GitSourceWithRef_ExtractsRef(t *testing.T) {
	root := newTestRoot(t)
	modDir := filepath.Join(root, "mod")
	if err := os.MkdirAll(modDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeModulesJSON(t, root, []struct {
		Key     string `json:"Key"`
		Source  string `json:"Source"`
		Version string `json:"Version"`
		Dir     string `json:"Dir"`
	}{
		{Key: "x", Source: "git::https://github.com/rafalmasiarek/terraform-aws-vpc.git?ref=v1.2.3", Version: "", Dir: "mod"},
	})
	chdir(t, root)

	source, version := detectOrigin(context.Background(), modDir)
	if source != "git::https://github.com/rafalmasiarek/terraform-aws-vpc.git?ref=v1.2.3" {
		t.Errorf("source = %q", source)
	}
	if version != "v1.2.3" {
		t.Errorf("version = %q, want v1.2.3", version)
	}
}

// Floating git source (no ?ref=) must resolve to "unknown", not whatever
// happens to be checked out locally - that's the non-determinism this data
// source must avoid.
func TestDetectOrigin_FloatingGitSource_VersionUnknown(t *testing.T) {
	root := newTestRoot(t)
	modDir := filepath.Join(root, "mod")
	if err := os.MkdirAll(modDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeModulesJSON(t, root, []struct {
		Key     string `json:"Key"`
		Source  string `json:"Source"`
		Version string `json:"Version"`
		Dir     string `json:"Dir"`
	}{
		{Key: "x", Source: "git::https://github.com/rafalmasiarek/terraform-aws-vpc.git", Version: "", Dir: "mod"},
	})
	chdir(t, root)

	_, version := detectOrigin(context.Background(), modDir)
	if version != "unknown" {
		t.Errorf("version = %q, want unknown", version)
	}
}

// Local path sources have neither a Version nor a ?ref= - version must be
// "unknown", not backfilled from the directory's own git state.
func TestDetectOrigin_LocalPathSource_VersionUnknown(t *testing.T) {
	root := newTestRoot(t)
	modDir := filepath.Join(root, "mod")
	if err := os.MkdirAll(modDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeModulesJSON(t, root, []struct {
		Key     string `json:"Key"`
		Source  string `json:"Source"`
		Version string `json:"Version"`
		Dir     string `json:"Dir"`
	}{
		{Key: "x", Source: "../../some-module", Version: "", Dir: "mod"},
	})
	chdir(t, root)

	source, version := detectOrigin(context.Background(), modDir)
	if source != "../../some-module" {
		t.Errorf("source = %q", source)
	}
	if version != "unknown" {
		t.Errorf("version = %q, want unknown", version)
	}
}

func TestDetectOrigin_NoModulesJSON_ReturnsUnknown(t *testing.T) {
	root := newTestRoot(t)
	modDir := filepath.Join(root, "mod")
	if err := os.MkdirAll(modDir, 0o755); err != nil {
		t.Fatal(err)
	}
	chdir(t, root)

	source, version := detectOrigin(context.Background(), modDir)
	if source != "unknown" {
		t.Errorf("source = %q, want unknown", source)
	}
	if version != "unknown" {
		t.Errorf("version = %q, want unknown", version)
	}
}

// Same declared input, two "machines" (two independent temp dirs sharing
// identical modules.json content) must produce identical output - the
// core determinism guarantee this data source exists to provide.
func TestDetectOrigin_Deterministic_AcrossIdenticalDeclaredInputs(t *testing.T) {
	entries := []struct {
		Key     string `json:"Key"`
		Source  string `json:"Source"`
		Version string `json:"Version"`
		Dir     string `json:"Dir"`
	}{
		{Key: "x", Source: "git::https://github.com/rafalmasiarek/terraform-aws-vpc.git?ref=v1.2.3", Version: "", Dir: "mod"},
	}

	var results [2][2]string
	for i := range results {
		root := newTestRoot(t)
		modDir := filepath.Join(root, "mod")
		if err := os.MkdirAll(modDir, 0o755); err != nil {
			t.Fatal(err)
		}
		writeModulesJSON(t, root, entries)
		chdir(t, root)
		source, version := detectOrigin(context.Background(), modDir)
		results[i] = [2]string{source, version}
	}

	if results[0] != results[1] {
		t.Errorf("non-deterministic: %v != %v", results[0], results[1])
	}
}
