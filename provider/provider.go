package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	tfprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptrace/terraform/client"
)

// UptraceProvider implements the Terraform provider for Uptrace.
type UptraceProvider struct {
	version string
}

type uptraceProviderModel struct {
	Endpoint types.String `tfsdk:"endpoint"`
	Token    types.String `tfsdk:"token"`
}

// New returns a provider factory for use with providerserver.Serve.
func New(version string) func() tfprovider.Provider {
	return func() tfprovider.Provider {
		return &UptraceProvider{version: version}
	}
}

// Metadata names the provider. Each resource type name is this name, an
// underscore and the resource's own name.
// It holds terraform:F2: the resource type names, their attributes and their
// import IDs are the ones the configurations and the state of operators hold.
func (p *UptraceProvider) Metadata(_ context.Context, _ tfprovider.MetadataRequest, resp *tfprovider.MetadataResponse) {
	resp.TypeName = "uptrace"
	resp.Version = p.version
}

func (p *UptraceProvider) Schema(_ context.Context, _ tfprovider.SchemaRequest, resp *tfprovider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage Uptrace organizations, projects, monitors, notification channels, teams, users, and project tokens.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				Optional:    true,
				Description: "Uptrace API endpoint. Can also be set with the UPTRACE_ENDPOINT environment variable.",
			},
			"token": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Uptrace API token. Can also be set with the UPTRACE_TOKEN environment variable.",
			},
		},
	}
}

// Configure resolves provider settings from HCL config or environment
// variables (UPTRACE_ENDPOINT, UPTRACE_TOKEN) and builds the shared API client.
func (p *UptraceProvider) Configure(ctx context.Context, req tfprovider.ConfigureRequest, resp *tfprovider.ConfigureResponse) {
	var conf uptraceProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &conf)...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpoint := envOrConfig("UPTRACE_ENDPOINT", conf.Endpoint)
	if endpoint == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("endpoint"), "Missing endpoint",
			"Set endpoint in config or UPTRACE_ENDPOINT env var.")
		return
	}

	token := envOrConfig("UPTRACE_TOKEN", conf.Token)
	if token == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("token"), "Missing token",
			"Set token in config or UPTRACE_TOKEN env var.")
		return
	}

	c, err := client.New(endpoint, token, p.version)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create API client", err.Error())
		return
	}

	resp.DataSourceData = c
	resp.ResourceData = c
}

func (p *UptraceProvider) Resources(_ context.Context) []func() resource.Resource {
	var all []func() resource.Resource
	for _, svc := range services {
		all = append(all, svc.Resources()...)
	}
	return all
}

func (p *UptraceProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	var all []func() datasource.DataSource
	for _, svc := range services {
		all = append(all, svc.DataSources()...)
	}
	return all
}

func envOrConfig(envKey string, configVal types.String) string {
	if !configVal.IsNull() && !configVal.IsUnknown() {
		return configVal.ValueString()
	}
	return os.Getenv(envKey)
}
