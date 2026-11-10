package testutil

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/uptrace/terraform/client"
	"github.com/uptrace/terraform/provider"
)

// ProtoV6ProviderFactories returns the provider factories for acceptance tests.
// Used as ProtoV6ProviderFactories field in resource.TestCase.
var ProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"uptrace": providerserver.NewProtocol6WithError(provider.New("test")()),
}

// PreCheck validates that required environment variables are set.
// Call from every acceptance test's PreCheck function.
func PreCheck(t *testing.T) {
	t.Helper()
	LoadEnv()
	if os.Getenv("UPTRACE_ENDPOINT") == "" {
		t.Fatal("UPTRACE_ENDPOINT must be set for acceptance tests")
	}
	if os.Getenv("UPTRACE_TOKEN") == "" {
		t.Fatal("UPTRACE_TOKEN must be set for acceptance tests")
	}
}

// AcceptanceTestUserEmail returns a clearly-marked email address for tests that
// need to create an Uptrace user. The pid suffix avoids collisions between
// concurrent acceptance-test processes.
func AcceptanceTestUserEmail(tag string) string {
	return fmt.Sprintf("terraform-acceptance-tests+%s-%d@example.com", tag, os.Getpid())
}

// CaptureAttr returns a TestCheckFunc that copies an attribute value from a
// resource's primary state into dest. Use it to thread an attribute between
// acceptance-test steps (e.g. to delete the resource out-of-band by ID).
func CaptureAttr(resourceAddr, attr string, dest *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceAddr]
		if !ok {
			return fmt.Errorf("resource %s not found", resourceAddr)
		}
		v, ok := rs.Primary.Attributes[attr]
		if !ok {
			return fmt.Errorf("attribute %s not found on %s", attr, resourceAddr)
		}
		*dest = v
		return nil
	}
}

// TestAccClient returns an API client configured from environment variables.
// It fails the test immediately if the client cannot be created.
func TestAccClient(t *testing.T) *client.Client {
	t.Helper()
	c, err := client.New(
		os.Getenv("UPTRACE_ENDPOINT"),
		os.Getenv("UPTRACE_TOKEN"),
		"test",
	)
	if err != nil {
		t.Fatalf("creating API client: %v", err)
	}
	return c
}
