package team

import (
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// Registration declares the resources and data sources exposed by the team service.
type Registration struct{}

func (Registration) Name() string {
	return "team"
}

func (Registration) Resources() []func() resource.Resource {
	return []func() resource.Resource{
		NewTeamResource,
		NewTeamProjectResource,
		NewTeamUserResource,
	}
}

func (Registration) DataSources() []func() datasource.DataSource {
	return nil
}
