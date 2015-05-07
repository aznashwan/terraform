package azure

import (
	"fmt"
	"log"

	"github.com/Azure/azure-sdk-for-go/management/virtualnetwork"
	"github.com/hashicorp/terraform/helper/schema"
)

// resourceAzureDnsServer returns the *schema.Resource associated
// to an Azure hosted service.
func resourceAzureDnsServer() *schema.Resource {
	return &schema.Resource{
		Create: resourceAzureDnsServerCreate,
		Read:   resourceAzureDnsServerRead,
		Update: resourceAzureDnsServerUpdate,
		Exists: resourceAzureDnsServerExists,
		Delete: resourceAzureDnsServerDelete,

		SchemaVersion: 1,

		Schema: map[string]*schema.Schema{
			"name": &schema.Schema{
				Type:        schema.TypeString,
				Required:    true,
				Description: parameterDescriptions["name"],
			},
			"dns_address": &schema.Schema{
				Type:        schema.TypeString,
				Required:    true,
				Description: parameterDescriptions["dns_address"],
			},
		},
	}
}

// resourceAzureDnsServerCreate does all the necessary API calls
// to create a new DNS server definition on Azure.
func resourceAzureDnsServerCreate(d *schema.ResourceData, meta interface{}) error {
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

	log.Println("[DEBUG] Adding new DNS server definition to Azure.")
	name := d.Get("name").(string)
	address := d.Get("dns_address").(string)
	netConf.Configuration.Dns.DnsServers = append(
		netConf.Configuration.Dns.DnsServers,
		virtualnetwork.DnsServer{
			Name:      name,
			IPAddress: address,
		})

	// send the configuration back to Azure:
	log.Println("[INFO] Sending updated network configuration back to Azure.")
	err = networkClient.SetVirtualNetworkConfiguration(netConf)
	azureClient.mutex.Unlock()
	if err != nil {
		return fmt.Errorf("Failed setting updated network configuration: %s", err)
	}

	d.SetId(getRandomStringLabel(50))
	return nil
}

// resourceAzureDnsServerRead does all the necessary API calls to read
// the state of the DNS server off Azure.
func resourceAzureDnsServerRead(d *schema.ResourceData, meta interface{}) error {
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

	// search for our DNS and update it if the IP has been changed:
	for _, dns := range netConf.Configuration.Dns.DnsServers {
		if dns.Name == name {
			found = true
			d.Set("dns_address", dns.IPAddress)
		}
	}

	// remove the resource from the state if it has been deleted in the meantime:
	if !found {
		d.SetId("")
	}

	return nil
}

// resourceAzureDnsServerUpdate does all the necessary API calls
// to update the DNS definition on Azure.
func resourceAzureDnsServerUpdate(d *schema.ResourceData, meta interface{}) error {
	azureClient, ok := meta.(*AzureClient)
	if !ok {
		return fmt.Errorf("Failed to convert to *AzureClient, got: %T", meta)
	}
	managementClient := azureClient.managementClient
	networkClient := virtualnetwork.NewClient(managementClient)

	var err error
	var found bool
	name := d.Get("name").(string)
	caddress := d.HasChange("dns_address")
	var netConf virtualnetwork.NetworkConfiguration

	if caddress {
		log.Println("[DEBUG] DNS server address has changes; updating it on Azure.")
		log.Println("[INFO] Fetching current network configuration from Azure.")
		azureClient.mutex.Lock()
		netConf, err = networkClient.GetVirtualNetworkConfiguration()
		if err != nil {
			return fmt.Errorf("Failed to get the current network configuration from Azure: %s", err)
		}

		// search for our DNS and update its address value:
		for i, dns := range netConf.Configuration.Dns.DnsServers {
			found = true
			if dns.Name == name {
				netConf.Configuration.Dns.DnsServers[i].IPAddress = d.Get("dns_address").(string)
			}
		}

		// if the config has changes, send the configuration back to Azure:
		if found && caddress {
			log.Println("[INFO] Sending updated network configuration back to Azure.")
			err = networkClient.SetVirtualNetworkConfiguration(netConf)
			azureClient.mutex.Unlock()
			if err != nil {
				return fmt.Errorf("Failed setting updated network configuration: %s", err)
			}
		}
	}

	// remove the resource from the state if it has been deleted in the meantime:
	if !found {
		d.SetId("")
	}

	return nil
}

// resourceAzureDnsServerExists does all the necessary API calls to
// check if the DNS server definition alredy exists on Azure.
func resourceAzureDnsServerExists(d *schema.ResourceData, meta interface{}) (bool, error) {
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

	name := d.Get("name").(string)

	// search for the DNS server's definition:
	for _, dns := range netConf.Configuration.Dns.DnsServers {
		if dns.Name == name {
			return true, nil
		}
	}

	return false, nil
}

// resourceAzureDnsServerDelete does all the necessary API calls
// to delete the DNS server definition from Azure.
func resourceAzureDnsServerDelete(d *schema.ResourceData, meta interface{}) error {
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

	// search for the DNS server's definition and remove it:
	for i, dns := range netConf.Configuration.Dns.DnsServers {
		if dns.Name == name {
			netConf.Configuration.Dns.DnsServers = append(
				netConf.Configuration.Dns.DnsServers[:i],
				netConf.Configuration.Dns.DnsServers[i+1:]...,
			)
		}
	}

	// send the configuration back to Azure:
	log.Println("[INFO] Sending updated network configuration back to Azure.")
	err = networkClient.SetVirtualNetworkConfiguration(netConf)
	azureClient.mutex.Unlock()
	if err != nil {
		return fmt.Errorf("Failed setting updated network configuration: %s", err)
	}

	d.SetId("")
	return nil
}
