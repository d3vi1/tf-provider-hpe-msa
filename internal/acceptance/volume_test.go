//go:build acc

package acceptance

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/d3vi1/tf-provider-hpe-msa/internal/msa"
	"github.com/d3vi1/tf-provider-hpe-msa/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccVolume_basic(t *testing.T) {
	requireAccEnv(t)

	name := accName("vol")
	resourceName := "hpe_msa_volume.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: accProviderFactories(),
		CheckDestroy:             accCheckVolumeDestroyed,
		Steps: []resource.TestStep{
			{
				Config: accVolumeConfig(name),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttrSet(resourceName, "serial_number"),
					resource.TestCheckResourceAttrSet(resourceName, "wwid"),
				),
			},
		},
	})
}

func accProviderFactories() map[string]func() (providerserver.ProviderServer, error) {
	return map[string]func() (providerserver.ProviderServer, error){
		"hpe": providerserver.NewProtocol6WithError(provider.New("acc")()),
	}
}

func accVolumeConfig(name string) string {
	endpoint := os.Getenv("MSA_ENDPOINT")
	username := os.Getenv("MSA_USERNAME")
	password := os.Getenv("MSA_PASSWORD")
	insecure := accBoolEnv("MSA_INSECURE_TLS")
	pool := os.Getenv("MSA_POOL")
	poolBlock := ""
	if strings.TrimSpace(pool) != "" {
		poolBlock = fmt.Sprintf("  pool          = %q\n", pool)
	}

	return fmt.Sprintf(`
provider "hpe" {
  endpoint     = %q
  username     = %q
  password     = %q
  insecure_tls = %t
}

resource "hpe_msa_volume" "test" {
  name          = %q
  size          = "1GB"
%s
  allow_destroy = true
}
`, endpoint, username, password, insecure, name, poolBlock)
}

func accCheckVolumeDestroyed(state *terraform.State) error {
	client, err := accClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	response, err := client.Execute(ctx, "show", "volumes")
	if err != nil {
		return fmt.Errorf("query volumes: %w", err)
	}
	volumes := msa.VolumesFromResponse(response)

	for _, rs := range state.RootModule().Resources {
		if rs.Type != "hpe_msa_volume" {
			continue
		}
		id := rs.Primary.ID
		name := rs.Primary.Attributes["name"]
		for _, volume := range volumes {
			if id != "" && volume.SerialNumber == id {
				return fmt.Errorf("volume %s still exists on array", id)
			}
			if name != "" && strings.EqualFold(volume.Name, name) {
				return fmt.Errorf("volume %s still exists on array", name)
			}
		}
	}
	return nil
}

func requireAccEnv(t *testing.T) {
	required := []string{
		"MSA_ENDPOINT",
		"MSA_USERNAME",
		"MSA_PASSWORD",
	}

	for _, key := range required {
		if os.Getenv(key) == "" {
			t.Skip("acceptance tests skipped; missing required environment variables")
		}
	}
}

func accName(prefix string) string {
	base := os.Getenv("MSA_TEST_PREFIX")
	if base == "" {
		base = "tfacc"
	}
	base = strings.ToLower(strings.ReplaceAll(base, " ", "-"))
	return fmt.Sprintf("%s-%s-%d", base, prefix, time.Now().UnixNano())
}

func accBoolEnv(key string) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return false
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false
	}
	return parsed
}

func accClient() (*msa.Client, error) {
	return msa.NewClient(msa.Config{
		Endpoint:    os.Getenv("MSA_ENDPOINT"),
		Username:    os.Getenv("MSA_USERNAME"),
		Password:    os.Getenv("MSA_PASSWORD"),
		InsecureTLS: accBoolEnv("MSA_INSECURE_TLS"),
	})
}
