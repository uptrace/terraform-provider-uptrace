package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/uptrace/terraform/services/monitor"
	"github.com/uptrace/terraform/services/notifchan"
	"github.com/uptrace/terraform/services/org"
	"github.com/uptrace/terraform/services/project"
	"github.com/uptrace/terraform/services/team"
	"github.com/uptrace/terraform/services/user"
)

// ServiceRegistration is implemented by every service package under
// services/. Add a new service by appending to the services slice.
type ServiceRegistration interface {
	Name() string
	Resources() []func() resource.Resource
	DataSources() []func() datasource.DataSource
}

var services = []ServiceRegistration{
	monitor.Registration{},
	notifchan.Registration{},
	org.Registration{},
	project.Registration{},
	team.Registration{},
	user.Registration{},
}
