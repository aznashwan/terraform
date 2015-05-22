package azure

import (
	"fmt"
	"log"

	"github.com/Azure/azure-sdk-for-go/management/virtualnetwork"
	"github.com/hashicorp/terraform/helper/schema"
)

// resourceAzureLocalNetworkConnetion returns the schema.Resource associated to an
// Azure hosted service.
func resourceAzureLocalNetworkConnection() *schema.Resource {
	return &schema.Resource{
		Create: resourceAzureLocalNetworkConnectionCreate,
		Read:   resourceAzureLocalNetworkConnectionRead,
		Update: resourceAzureLocalNetworkConnectionUpdate,
		Exists: resourceAzureLocalNetworkConnectionExists,
		Delete: resourceAzureLocalNetworkConnectionDelete,

		SchemaVersion: 1,

		Schema: map[string]*schema.Schema{
			"name": &schema.Schema{
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: parameterDescriptions["name"],
			},
			"vpn_gateway_address": &schema.Schema{
				Type:        schema.TypeString,
				Required:    true,
				Description: parameterDescriptions["vpn_gateway_address"],
			},
			"address_space_prefixes": &schema.Schema{
				Type:     schema.TypeList,
				Required: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Description: parameterDescriptions["address_space_prefixes"],
			},
		},
	}
}

// sourceAzureLocalNetworkConnectionCreate issues all the necessary API calls
// to create a virtual network on Azure.
func resourceAzureLocalNetworkConnectionCreate(d *schema.ResourceData, meta interface{}) error {
	azureClient, ok := meta.(*AzureClient)
	if !ok {
		return fmt.Errorf("Failed to convert to *AzureClient, got: %T", meta)
	}
	managementClient := azureClient.managementClient
	networkClient := virtualnetwork.NewClient(managementClient)

	log.Println("[INFO] Fetching current network configuration from Azure.")
	azureClient.mutex.Lock()
	netConf, err := networkClient.GetVirtualNetworkConfiguration()
	if err != nil {
		return fmt.Errorf("Failed to get the current network configuration from Azure: %s", err)
	}

	// get provided configuration:
	name := d.Get("name").(string)
	vpnGateway := d.Get("vpn_gateway_address").(string)
	var prefixes []string
	if nprefixes := d.Get("address_space_prefixes.#").(int); nprefixes > 0 {
		prefixes = []string{}
		for i := 0; i < nprefixes; i++ {
			prefixes = append(prefixes, d.Get(fmt.Sprintf("address_space_prefixes.%d", i)).(string))
		}
	}

	// add configuration to network config:
	netConf.Configuration.LocalNetworkSites = append(netConf.Configuration.LocalNetworkSites,
		virtualnetwork.LocalNetworkSite{
			Name:              name,
			VPNGatewayAddress: vpnGateway,
			AddressSpace: virtualnetwork.AddressSpace{
				AddressPrefix: prefixes,
			},
		})

	// send the configuration back to Azure:
	log.Println("[INFO] Sending updated network configuration back to Azure.")
	reqID, err := networkClient.SetVirtualNetworkConfiguration(netConf)
	if err != nil {
		return fmt.Errorf("Failed setting updated network configuration: %s", err)
	}
	err = managementClient.WaitForOperation(reqID, nil)
	if err != nil {
		return fmt.Errorf("Failed updating the network configuration: %s", err)
	}

	azureClient.mutex.Unlock()
	d.SetId(getRandomStringLabel(50))
	return nil
}

// resourceAzureLocalNetworkConnectionRead does all the necessary API calls to
// read the state of our local natwork from Azure.
func resourceAzureLocalNetworkConnectionRead(d *schema.ResourceData, meta interface{}) error {
	azureClient, ok := meta.(*AzureClient)
	if !ok {
		return fmt.Errorf("Failed to convert to *AzureClient, got: %T", meta)
	}
	managementClient := azureClient.managementClient
	networkClient := virtualnetwork.NewClient(managementClient)

	log.Println("[INFO] Fetching current network configuration from Azure.")
	netConf, err := networkClient.GetVirtualNetworkConfiguration()
	if err != nil {
		return fmt.Errorf("Failed to get the current network configuration from Azure: %s", err)
	}

	var found bool
	name := d.Get("name").(string)

	// browsing for our network config:
	for _, lnet := range netConf.Configuration.LocalNetworkSites {
		if lnet.Name == name {
			found = true
			d.Set("vpn_gateway_address", lnet.VPNGatewayAddress)
			d.Set("address_space_prefixes", lnet.AddressSpace.AddressPrefix)
		}
	}

	// remove the resource from the state of it has been deleted in the meantime:
	if !found {
		log.Println(fmt.Printf("[INFO] Azure local network '%s' has been deleted remotely. Removimg from Terraform.", name))
		d.SetId("")
	}

	return nil
}

// resourceAzureLocalNetworkConnectionUpdate does all the necessary API calls
// update the settings of our Local Network on Azure.
func resourceAzureLocalNetworkConnectionUpdate(d *schema.ResourceData, meta interface{}) error {
	azureClient, ok := meta.(*AzureClient)
	if !ok {
		return fmt.Errorf("Failed to convert to *AzureClient, got: %T", meta)
	}
	managementClient := azureClient.managementClient
	networkClient := virtualnetwork.NewClient(managementClient)

	log.Println("[INFO] Fetching current network configuration from Azure.")
	azureClient.mutex.Lock()
	netConf, err := networkClient.GetVirtualNetworkConfiguration()
	if err != nil {
		return fmt.Errorf("Failed to get the current network configuration from Azure: %s", err)
	}

	name := d.Get("name").(string)
	cvpn := d.HasChange("vpn_gateway_address")
	cprefixes := d.HasChange("address_space_prefixes")

	var found bool
	for i, lnet := range netConf.Configuration.LocalNetworkSites {
		if lnet.Name == name {
			found = true
			if cvpn {
				netConf.Configuration.LocalNetworkSites[i].VPNGatewayAddress = d.Get("vpn_gateway_address").(string)
			}
			if cprefixes {
				var prefixes []string
				if nprefixes := d.Get("address_space_prefixes.#").(int); nprefixes > 0 {
					prefixes = []string{}
					for i := 0; i < nprefixes; i++ {
						prefixes = append(prefixes, d.Get(fmt.Sprintf("address_space_prefixes.%d", i)).(string))
					}
				}
				netConf.Configuration.LocalNetworkSites[i].AddressSpace.AddressPrefix = prefixes
			}
		}
	}

	// remove the resource from the state of it has been deleted in the meantime:
	if !found {
		log.Println(fmt.Printf("[INFO] Azure local network '%s' has been deleted remotely. Removimg from Terraform.", name))
		d.SetId("")
	} else if cvpn || cprefixes {
		// else, send the configuration back to Azure:
		log.Println("[INFO] Sending updated network configuration back to Azure.")
		reqID, err := networkClient.SetVirtualNetworkConfiguration(netConf)
		if err != nil {
			return fmt.Errorf("Failed setting updated network configuration: %s", err)
		}
		err = managementClient.WaitForOperation(reqID, nil)
		if err != nil {
			return fmt.Errorf("Failed updating the network configuration: %s", err)
		}
	}

	azureClient.mutex.Unlock()
	return nil
}

// resourceAzureLocalNetworkConnectionExists does all the necessary API calls
// to check if the local network already exists on Azure.
func resourceAzureLocalNetworkConnectionExists(d *schema.ResourceData, meta interface{}) (bool, error) {
	azureClient, ok := meta.(*AzureClient)
	if !ok {
		return false, fmt.Errorf("Failed to convert to *AzureClient, got: %T", meta)
	}
	managementClient := azureClient.managementClient
	networkClient := virtualnetwork.NewClient(managementClient)

	log.Println("[INFO] Fetching current network configuration from Azure.")
	netConf, err := networkClient.GetVirtualNetworkConfiguration()
	if err != nil {
		return false, fmt.Errorf("Failed to get the current network configuration from Azure: %s", err)
	}

	name := d.Get("name")

	for _, lnet := range netConf.Configuration.LocalNetworkSites {
		if lnet.Name == name {
			return true, nil
		}
	}

	return false, nil
}

// resourceAzureLocalNetworkConnectionDelete does all the necessary API calls
// to delete a local network off Azure.
func resourceAzureLocalNetworkConnectionDelete(d *schema.ResourceData, meta interface{}) error {
	azureClient, ok := meta.(*AzureClient)
	if !ok {
		return fmt.Errorf("Failed to convert to *AzureClient, got: %T", meta)
	}
	managementClient := azureClient.managementClient
	networkClient := virtualnetwork.NewClient(managementClient)

	log.Println("[INFO] Fetching current network configuration from Azure.")
	azureClient.mutex.Lock()
	netConf, err := networkClient.GetVirtualNetworkConfiguration()
	if err != nil {
		return fmt.Errorf("Failed to get the current network configuration from Azure: %s", err)
	}

	name := d.Get("name").(string)

	// search for our local network and remove it if found:
	for i, lnet := range netConf.Configuration.LocalNetworkSites {
		if lnet.Name == name {
			netConf.Configuration.LocalNetworkSites = append(
				netConf.Configuration.LocalNetworkSites[:i],
				netConf.Configuration.LocalNetworkSites[i+1:]...,
			)
		}
	}

	// send the configuration back to Azure:
	log.Println("[INFO] Sending updated network configuration back to Azure.")
	reqID, err := networkClient.SetVirtualNetworkConfiguration(netConf)
	if err != nil {
		return fmt.Errorf("Failed setting updated network configuration: %s", err)
	}
	err = managementClient.WaitForOperation(reqID, nil)
	if err != nil {
		return fmt.Errorf("Failed updating the network configuration: %s", err)
	}

	azureClient.mutex.Unlock()
	d.SetId("")
	return nil
}
