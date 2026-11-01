package notifchan_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/uptrace/terraform/internal/client"
	"github.com/uptrace/terraform/internal/generated"
	"github.com/uptrace/terraform/internal/testutil"
)

// testAccCheckChannelDestroy verifies that every resource of the given Terraform
// type has been deleted server-side. Shared by the typed channel acceptance tests.
func testAccCheckChannelDestroy(t *testing.T, resourceType string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testutil.TestAccClient(t)
		for _, rs := range s.RootModule().Resources {
			if rs.Type != resourceType {
				continue
			}
			channelID, err := strconv.ParseUint(rs.Primary.ID, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid channel ID %q: %w", rs.Primary.ID, err)
			}
			projectID, err := strconv.ParseUint(rs.Primary.Attributes["project_id"], 10, 32)
			if err != nil {
				return fmt.Errorf("invalid project ID %q: %w", rs.Primary.Attributes["project_id"], err)
			}
			_, err = c.API.GetNotificationChannel(context.Background(), &generated.GetNotificationChannelRequestOptions{
				PathParams: &generated.GetNotificationChannelPath{
					ProjectID: uint32(projectID),
					ChannelID: channelID,
				},
			})
			if err == nil {
				return fmt.Errorf("notification channel %s still exists after destroy", rs.Primary.ID)
			}
			if !client.IsNotFound(err) {
				return fmt.Errorf("checking notification channel %s after destroy: %w", rs.Primary.ID, err)
			}
		}
		return nil
	}
}

func deleteChannelOutOfBand(t *testing.T, projectIDStr, channelIDStr string) {
	t.Helper()
	if projectIDStr == "" || channelIDStr == "" {
		t.Fatal("project ID or channel ID was not captured before out-of-band delete")
	}
	projectID, err := strconv.ParseUint(projectIDStr, 10, 32)
	if err != nil {
		t.Fatalf("invalid project ID %q: %v", projectIDStr, err)
	}
	channelID, err := strconv.ParseUint(channelIDStr, 10, 64)
	if err != nil {
		t.Fatalf("invalid channel ID %q: %v", channelIDStr, err)
	}

	c := testutil.TestAccClient(t)
	_, err = c.API.DeleteNotificationChannel(context.Background(), &generated.DeleteNotificationChannelRequestOptions{
		PathParams: &generated.DeleteNotificationChannelPath{
			ProjectID: uint32(projectID),
			ChannelID: channelID,
		},
	})
	if err != nil && !client.IsNotFound(err) {
		t.Fatalf("delete channel %d out-of-band: %v", channelID, err)
	}
}

// importStateIDFunc returns a terraform import-id builder for resources that
// follow the "<project_id>:<channel_id>" pattern.
func importStateIDFunc(resourceAddr string) func(*terraform.State) (string, error) {
	return func(s *terraform.State) (string, error) {
		rs, ok := s.RootModule().Resources[resourceAddr]
		if !ok {
			return "", fmt.Errorf("%s not found in state", resourceAddr)
		}
		return fmt.Sprintf("%s:%s", rs.Primary.Attributes["project_id"], rs.Primary.ID), nil
	}
}
