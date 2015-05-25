package azure

import (
	"fmt"

	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/terraform"
)

// Provider returns a teraform.ResourceProvider.
func Provider() terraform.ResourceProvider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{
			"publish_settings_file": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				Description: parameterDescriptions["publish_settings_file"],
			},
			"subscription_id": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "",
				Description: parameterDescriptions["subscription_id"],
			},
			"management_certificate": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "",
				Description: parameterDescriptions["management_certificate"],
			},
			"management_url": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "",
				Description: parameterDescriptions["management_url"],
			},
		},

		ResourcesMap: map[string]*schema.Resource{
			"azure_instance":                 resourceAzureInstance(),
			"azure_hosted_service":           resourceAzureHostedService(),
			"azure_storage_service":          resourceAzureStorageService(),
			"azure_storage_container":        resourceAzureStorageContainer(),
			"azure_storage_blob":             resourceAzureStorageBlob(),
			"azure_virtual_network":          resourceAzureVirtualNetwork(),
			"azure_dns_server":               resourceAzureDnsServer(),
			"azure_local_network_connection": resourceAzureLocalNetworkConnection(),
			"azure_security_group":           resourceAzureSecurityGroup(),
			"azure_security_group_rule":      resourceAzureSecurityGroupRule(),
		},

		ConfigureFunc: providerConfigure,
	}
}

// providerConfigure configures the provider's AzureClient struct.
func providerConfigure(d *schema.ResourceData) (interface{}, error) {
	if !areValidAzureSettings(d) {
		return nil, fmt.Errorf("Insufficient configuration data. Please specify either a 'publish_settings_file'\n" +
			"or both a 'subscription_id' or 'management_certificate' with optional 'management_url'.")
	}
	config := &Config{
		PublishSettingsFilePath: d.Get("publish_settings_file").(string),
		SubscriptionID:          d.Get("subscription_id").(string),
		ManagementCert:          []byte(d.Get("management_certificate").(string)),
		ManagementUrl:           d.Get("management_url").(string),
	}

	return config.Client()
}

// areValidAzureSettings checks whether the provided dataset contains all the
// necessary fields for accessing the Azure management API.
func areValidAzureSettings(d *schema.ResourceData) bool {
	if _, ok := d.GetOk("publish_settings_file"); ok {
		return true
	}

	_, subIdOk := d.GetOk("subscription_id")
	_, certOk := d.GetOk("management_certificate")

	return subIdOk && certOk
}
