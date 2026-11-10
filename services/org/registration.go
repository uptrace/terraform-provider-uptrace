package org

import (
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// Registration declares the resources exposed by the org service.
type Registration struct{}

func (Registration) Name() string {
	return "org"
}

func (Registration) Resources() []func() resource.Resource {
	return []func() resource.Resource{
		NewOrgResource,
		NewOrgUserResource,
	}
}

func (Registration) DataSources() []func() datasource.DataSource {
	return nil
}
