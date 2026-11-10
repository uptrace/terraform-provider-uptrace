package notifchan

import (
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// Registration declares the resources and data sources exposed by the notification channel service.
type Registration struct{}

func (Registration) Name() string {
	return "notificationchannel"
}

func (Registration) Resources() []func() resource.Resource {
	return []func() resource.Resource{
		NewAlertmanagerChannelResource,
		NewEmailChannelResource,
		NewGoogleChatChannelResource,
		NewIncidentioChannelResource,
		NewMattermostChannelResource,
		NewOpsgenieChannelResource,
		NewPagerdutyChannelResource,
		NewPushoverChannelResource,
		NewServicenowChannelResource,
		NewSlackChannelResource,
		NewTeamsChannelResource,
		NewTelegramChannelResource,
		NewWebhookChannelResource,
	}
}

func (Registration) DataSources() []func() datasource.DataSource {
	return nil
}
