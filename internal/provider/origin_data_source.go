package provider

import (
	"context"
	"encoding/json"
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
// own path.module explicitly. From there, this reads Terraform's own
// internal (undocumented, but stable in practice since 0.11+)
// .terraform/modules/modules.json to recover that module's source/version,
// the same way the earlier bash-script approach did, but with real error
// handling instead of a brittle shell pipeline.
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
		Description: "Best-effort detection of a calling module's own source/version, read back from Terraform's internal .terraform/modules/modules.json. Never fails the plan - a detection miss yields \"unknown\" values, since this is optional metadata, not a hard dependency.",
		Attributes: map[string]schema.Attribute{
			"module_path": schema.StringAttribute{
				Required:    true,
				Description: "Pass path.module from the calling module - there is no supported way for this provider to discover that on its own.",
			},
			"source": schema.StringAttribute{
				Computed:    true,
				Description: "The calling module's source, e.g. \"github.com/rafalmasiarek/terraform-aws-vpc\". \"unknown\" if it could not be determined.",
			},
			"version": schema.StringAttribute{
				Computed:    true,
				Description: "The calling module's resolved version/ref. \"unknown\" if it could not be determined (always the case for local path sources, which carry no version).",
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
// "unknown" for both values, since this is best-effort metadata that must
// never break a real plan/apply.
//
// Tries git first: for a git-sourced module, path.module IS the actual
// checked-out repo directory (Terraform's module installer leaves .git
// intact), so `git remote get-url origin` + `git describe` there is more
// precise than modules.json - it reports the exact commit actually
// checked out, not just whatever ref string the caller wrote, and it also
// works for the root module itself (which modules.json never describes
// meaningfully). modules.json is only consulted as a fallback, for cases
// git can't help with (local path sources outside any repo, registry
// sources served as plain tarballs with no .git at all, or git missing
// from PATH - which would be surprising, since Terraform itself already
// shells out to git to fetch git-sourced modules in the first place).
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

	if src, ver, ok := detectOriginViaGit(ctx, absModulePath); ok {
		return src, ver
	}

	modulesJSONPath, ok := findModulesJSON(root)
	if !ok {
		return unknown, unknown
	}

	raw, err := os.ReadFile(modulesJSONPath)
	if err != nil {
		return unknown, unknown
	}

	var parsed modulesJSON
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return unknown, unknown
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
		if absDir == absModulePath {
			src, ver := m.Source, m.Version
			if src == "" {
				src = unknown
			}
			if ver == "" {
				ver = unknown
			}
			return src, ver
		}
	}

	return unknown, unknown
}

// detectOriginViaGit reports ok=false (never an error) if dir isn't inside
// a git repo, git isn't on PATH, or either command fails/times out - any
// of which simply means "try the modules.json fallback instead".
func detectOriginViaGit(ctx context.Context, dir string) (source string, version string, ok bool) {
	remote, err := runGit(ctx, dir, "remote", "get-url", "origin")
	if err != nil || remote == "" {
		return "", "", false
	}

	// --always falls back to a short commit SHA when there's no reachable
	// tag at all, so this still returns something precise rather than
	// giving up. --dirty flags a checkout with local modifications -
	// should never happen for a module Terraform itself fetched, but
	// cheap to surface if it somehow does.
	describe, err := runGit(ctx, dir, "describe", "--tags", "--always", "--dirty")
	if err != nil || describe == "" {
		describe = "unknown"
	}

	return remote, describe, true
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
