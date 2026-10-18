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
// Deterministic by design: every value comes from .terraform/modules/modules.json,
// which Terraform itself generates from the *declared* module source/ref at
// init time - never from live git state on the actual checked-out
// directory (git describe, rev-parse, working-tree status). Two machines
// with the same declared module source/version always produce the same
// origin, regardless of fetch depth, available tags, or a dirty tree.
//
// Version precedence:
//  1. modules.json's Version field - populated for registry sources with
//     the resolved release version.
//  2. The ?ref= in modules.json's Source string - populated for git
//     sources pinned to a tag/branch/SHA.
//  3. "unknown" - a floating git source (no ?ref=) or a local path source
//     has no declared version to report. This is intentionally never
//     backfilled from the actual checkout (that's exactly the
//     non-determinism this data source must avoid).
//
// source: modules.json's Source string, falling back to git's remote
// origin URL only if no modules.json entry exists at all. The remote URL
// is itself declared configuration (not checkout state), so it doesn't
// reintroduce non-determinism.
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
	if jsonOK && jsonSource != "" {
		source = jsonSource
	} else if remote, ok := gitRemoteOrigin(ctx, absModulePath); ok {
		source = remote
	}

	version = unknown
	if jsonOK && jsonRef != "" {
		version = jsonRef
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
//
// modules.json is only ever looked up directly under root (the current
// working directory Terraform was invoked in) - never discovered by
// walking upward, which previously risked picking up an unrelated
// ancestor project's .terraform directory (e.g. in a monorepo).
func refFromModulesJSON(root, absModulePath string) (source string, ref string, ok bool) {
	modulesJSONPath := filepath.Join(root, ".terraform", "modules", "modules.json")
	if info, err := os.Stat(modulesJSONPath); err != nil || info.IsDir() {
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
