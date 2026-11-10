package monitor

import (
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// Registration declares the resources and data sources exposed by the monitor service.
type Registration struct{}

func (Registration) Name() string {
	return "monitor"
}

func (Registration) Resources() []func() resource.Resource {
	out := make([]func() resource.Resource, len(kinds))
	for i, k := range kinds {
		out[i] = newResource(k)
	}
	return out
}

func (Registration) DataSources() []func() datasource.DataSource {
	return nil
}
