package project

import (
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// Registration declares the resources and data sources exposed by the project service.
type Registration struct{}

func (Registration) Name() string {
	return "project"
}

func (Registration) Resources() []func() resource.Resource {
	return []func() resource.Resource{
		NewProjectResource,
		NewProjectTokenResource,
	}
}

func (Registration) DataSources() []func() datasource.DataSource {
	return nil
}
