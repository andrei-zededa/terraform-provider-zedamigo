// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

const testAccSystemMemoryDataSourceConfig = `
data "zedamigo_system_memory" "test" {
}
`

func TestAccSystemMemoryDataSource(t *testing.T) {
	hostname, err := os.Hostname()
	if err != nil {
		t.Errorf("Can't get system hostname: %v", err)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: testAccSystemMemoryDataSourceConfig,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.zedamigo_system_memory.test",
						tfjsonpath.New("id"),
						knownvalue.StringExact(hostname),
					),
				},
			},
		},
	})
}
