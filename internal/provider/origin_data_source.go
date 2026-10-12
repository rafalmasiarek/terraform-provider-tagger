package provider

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// A Terraform provider has no implicit way to know which specific calling
// module instantiated one of its data sources - the caller must pass its
// own path.module explicitly.
type originDataSource struct{}

func NewOriginDataSource() datasource.DataSource {
	return &originDataSource{}
}

type originDataSourceModel struct {
	ModulePath types.String `tfsdk:"module_path"`
	Source     types.String `tfsdk:"source"`
	Version    types.String `tfsdk:"version"`
}

type modulesJSON struct {
	Modules []struct {
		Key     string `json:"Key"`
		Source  string `json:"Source"`
		Version string `json:"Version"`
		Dir     string `json:"Dir"`
	} `json:"Modules"`
}

func (d *originDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_origin"
}

func (d *originDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Best-effort detection of a calling module's own source/release version. Never fails the plan - a detection miss yields \"unknown\" values, since this is optional metadata, not a hard dependency.",
		Attributes: map[string]schema.Attribute{
			"module_path": schema.StringAttribute{
				Required:    true,
				Description: "Pass path.module from the calling module - there is no supported way for this provider to discover that on its own.",
			},
			"source": schema.StringAttribute{
				Computed:    true,
				Description: "The calling module's source, e.g. \"https://github.com/rafalmasiarek/terraform-aws-vpc.git\". \"unknown\" if it could not be determined.",
			},
			"version": schema.StringAttribute{
				Computed:    true,
				Description: "The calling module's release version - an actual git tag if one is reachable, otherwise the ?ref= the caller wrote when sourcing the module, otherwise a bare commit SHA as a last resort. \"unknown\" if none of these were available (e.g. a local path source).",
			},
		},
	}
}

func (d *originDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data originDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	source, version := detectOrigin(ctx, data.ModulePath.ValueString())

	data.Source = types.StringValue(source)
	data.Version = types.StringValue(version)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// detectOrigin never returns an error - any failure falls back to
// "unknown", since this is best-effort metadata that must never break a
// real plan/apply.
//
// We resolve dependencies by release version, not by whichever commit
// happens to be checked out - so version precedence is:
//  1. An actual reachable git tag (git describe --tags, no --always - a
//     real release version, not a synthesized one).
//  2. The ?ref= the caller wrote in modules.json's Source string when
//     sourcing the module - also a real release version when a tag ref
//     was used, just not confirmed reachable from HEAD.
//  3. A bare commit SHA, only as an absolute last resort.
//
// source is resolved independently: git's own remote origin URL if
// available (for a git-sourced module, path.module IS the actual
// checked-out repo directory - Terraform's module installer leaves .git
// intact), falling back to modules.json's Source string otherwise.
func detectOrigin(ctx context.Context, modulePath string) (source string, version string) {
	const unknown = "unknown"

	root, err := os.Getwd()
	if err != nil {
		return unknown, unknown
	}

	absModulePath := modulePath
	if !filepath.IsAbs(absModulePath) {
		absModulePath = filepath.Join(root, absModulePath)
	}
	absModulePath, err = filepath.Abs(absModulePath)
	if err != nil {
		return unknown, unknown
	}

	jsonSource, jsonRef, jsonOK := refFromModulesJSON(root, absModulePath)

	source = unknown
	if remote, ok := gitRemoteOrigin(ctx, absModulePath); ok {
		source = remote
	} else if jsonOK && jsonSource != "" {
		source = jsonSource
	}

	version = unknown
	if tag, ok := gitTagVersion(ctx, absModulePath); ok {
		version = tag
	} else if jsonOK && jsonRef != "" {
		version = jsonRef
	} else if sha, ok := gitShortSHA(ctx, absModulePath); ok {
		version = sha
	}

	return source, version
}

func gitRemoteOrigin(ctx context.Context, dir string) (string, bool) {
	out, err := runGit(ctx, dir, "remote", "get-url", "origin")
	if err != nil || out == "" {
		return "", false
	}
	return out, true
}

// gitTagVersion deliberately omits --always: it must fail (not fall back
// to a hash) when no tag is reachable, so the caller can move on to the
// modules.json ?ref= fallback instead of settling for a hash early.
func gitTagVersion(ctx context.Context, dir string) (string, bool) {
	out, err := runGit(ctx, dir, "describe", "--tags", "--dirty")
	if err != nil || out == "" {
		return "", false
	}
	return out, true
}

func gitShortSHA(ctx context.Context, dir string) (string, bool) {
	out, err := runGit(ctx, dir, "rev-parse", "--short", "HEAD")
	if err != nil || out == "" {
		return "", false
	}
	return out, true
}

func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// refFromModulesJSON finds the entry in .terraform/modules/modules.json
// matching absModulePath and returns its raw Source string plus a release
// version: Version when modules.json populated one (registry sources),
// otherwise whatever ?ref=... was given in Source (git sources).
func refFromModulesJSON(root, absModulePath string) (source string, ref string, ok bool) {
	modulesJSONPath, found := findModulesJSON(root)
	if !found {
		return "", "", false
	}

	raw, err := os.ReadFile(modulesJSONPath)
	if err != nil {
		return "", "", false
	}

	var parsed modulesJSON
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", "", false
	}

	for _, m := range parsed.Modules {
		dir := m.Dir
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(root, dir)
		}
		absDir, err := filepath.Abs(dir)
		if err != nil {
			continue
		}
		if absDir != absModulePath {
			continue
		}

		ref = m.Version
		if ref == "" {
			ref = refFromSourceString(m.Source)
		}
		return m.Source, ref, true
	}

	return "", "", false
}

// refFromSourceString pulls ?ref=... out of a git-style source string,
// e.g. "git::https://github.com/x/y.git?ref=v0.3.1" -> "v0.3.1". Returns
// "" if there's no such query parameter (or the string doesn't parse as a
// URL at all, e.g. a bare local path).
func refFromSourceString(src string) string {
	u, err := url.Parse(strings.TrimPrefix(src, "git::"))
	if err != nil {
		return ""
	}
	return u.Query().Get("ref")
}

// findModulesJSON walks upward from root looking for
// .terraform/modules/modules.json - bounded, so a malformed/unusual
// directory layout fails fast instead of walking to filesystem root.
// Walking at all (rather than only checking root itself) is a safety net
// for -chdir-style invocations; in the common case it matches on the
// first iteration.
func findModulesJSON(root string) (string, bool) {
	dir := root
	for i := 0; i < 8; i++ {
		candidate := filepath.Join(dir, ".terraform", "modules", "modules.json")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
}
