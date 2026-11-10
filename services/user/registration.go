package user

import (
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// Registration declares the resources and data sources exposed by the user service.
type Registration struct{}

func (Registration) Name() string {
	return "user"
}

func (Registration) Resources() []func() resource.Resource {
	return []func() resource.Resource{
		NewUserResource,
	}
}

func (Registration) DataSources() []func() datasource.DataSource {
	return nil
}
