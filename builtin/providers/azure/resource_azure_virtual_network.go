package azure

import (
	"fmt"
	"log"

	netsecgroup "github.com/Azure/azure-sdk-for-go/management/networksecuritygroup"
	"github.com/Azure/azure-sdk-for-go/management/virtualnetwork"
	"github.com/hashicorp/terraform/helper/schema"
)

// resourceAzureVirtualNetwork returns the schema.Resource associated to an
// Azure hosted service.
func resourceAzureVirtualNetwork() *schema.Resource {
	return &schema.Resource{
		Create: resourceAzureVirtualNetworkCreate,
		Read:   resourceAzureVirtualNetworkRead,
		Update: resourceAzureVirtualNetworkUpdate,
		Exists: resourceAzureVirtualNetworkExists,
		Delete: resourceAzureVirtualNetworkDelete,

		SchemaVersion: 1,

		Schema: map[string]*schema.Schema{
			"name": &schema.Schema{
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: parameterDescriptions["name"],
			},
			"location": &schema.Schema{
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: parameterDescriptions["location"],
			},
			"dns_servers_names": &schema.Schema{
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"subnet": &schema.Schema{
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},
						"prefix": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},
						"security_group_name": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
							Default:  "",
						},
					},
				},
			},
			"address_space_prefixes": &schema.Schema{
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Description: parameterDescriptions["address_space_prefixes"],
			},
		},
	}
}

// resourceAzureVirtualNetworkCreate does all the necessary API calls to create
// an Azure virtual network.
func resourceAzureVirtualNetworkCreate(d *schema.ResourceData, meta interface{}) error {
	azureClient, ok := meta.(*AzureClient)
	if !ok {
		return fmt.Errorf("Failed to convert to *AzureClient, got: %T", meta)
	}
	managementClient := azureClient.managementClient
	networkClient := virtualnetwork.NewClient(managementClient)

	log.Println("[INFO] Retrieving current network configuration from Azure.")
	azureClient.mutex.Lock()
	netConf, err := networkClient.GetVirtualNetworkConfiguration()
	if err != nil {
		return fmt.Errorf("Error while retrieving current network configuration: %s", err)
	}

	// create new virtual network configuration and add it to the config.
	name := d.Get("name").(string)
	location := d.Get("location").(string)

	// fetch address spaces:
	var prefixes []string
	if nprefixes := d.Get("address_space_prefixes.#").(int); nprefixes > 0 {
		prefixes = []string{}
		for i := 0; i < nprefixes; i++ {
			prefixes = append(prefixes, d.Get(fmt.Sprintf("address_space_prefixes.%d", i)).(string))
		}
	}

	// fetch DNS references:
	var dnsRefs []virtualnetwork.DnsServerRef
	if ndnses := d.Get("dns_servers_names.#").(int); ndnses > 0 {
		dnsRefs = []virtualnetwork.DnsServerRef{}
		for i := 0; i < ndnses; i++ {
			dnsRefs = append(dnsRefs, virtualnetwork.DnsServerRef{
				Name: d.Get(fmt.Sprintf("dns_servers_names.%d", i)).(string),
			})
		}
	}

	// fetch subnets:
	var subnets []virtualnetwork.Subnet
	if nsubs := d.Get("subnet.#").(int); nsubs > 0 {
		subnets = []virtualnetwork.Subnet{}
		for i := 0; i < nsubs; i++ {
			sub := d.Get(fmt.Sprintf("subnet.%d", i)).(map[string]interface{})
			subnets = append(subnets, virtualnetwork.Subnet{
				Name:          sub["name"].(string),
				AddressPrefix: sub["prefix"].(string),
			})
		}
	}

	// create the virtual network and add it to the global network config.
	vn := virtualnetwork.VirtualNetworkSite{
		Name:          name,
		Location:      location,
		Subnets:       subnets,
		DnsServersRef: dnsRefs,
		AddressSpace: virtualnetwork.AddressSpace{
			AddressPrefix: prefixes,
		},
	}
	netConf.Configuration.VirtualNetworkSites = append(netConf.Configuration.VirtualNetworkSites, vn)

	// send the updated configuration back:
	log.Println("[INFO] Sending virtual network configuration back to Azure.")
	err = networkClient.SetVirtualNetworkConfiguration(netConf)
	if err != nil {
		return fmt.Errorf("Failed updating network configuration: %s", err)
	}

	// fetch subnets:
	if nsubs := d.Get("subnet.#").(int); nsubs > 0 {
		log.Println("[INFO] Started applying network security rules to subnets:")
		netSecClient := netsecgroup.NewClient(managementClient)
		for i := 0; i < nsubs; i++ {
			log.Println(fmt.Sprintf("[DEBUG] Adding network security settings to subnet %d.", i+1))
			sub := d.Get(fmt.Sprintf("subnet.%d", i)).(map[string]interface{})
			if secGroup, ok := sub["security_group_name"].(string); ok {
				reqID, err := netSecClient.AddNetworkSecurityToSubnet(
					secGroup,
					sub["name"].(string),
					name,
				)
				if err != nil {
					return fmt.Errorf("Failed requesting addition of network security to subnet %d: %s", i+1, err)
				}
				err = managementClient.WaitAsyncOperation(reqID)
				if err != nil {
					return fmt.Errorf("Failed adding network security settings to subnet %d: %s", i+1, err)
				}
			}
		}
	}

	d.SetId(getRandomStringLabel(50))
	azureClient.mutex.Unlock()
	return nil
}

// resourceAzureVirtualNetworkRead does all the necessary API calls to read
// the state of a virtual network from Azure.
func resourceAzureVirtualNetworkRead(d *schema.ResourceData, meta interface{}) error {
	azureClient, ok := meta.(*AzureClient)
	if !ok {
		return fmt.Errorf("Failed to convert to *AzureClient, got: %T", meta)
	}
	managementClient := azureClient.managementClient
	networkClient := virtualnetwork.NewClient(managementClient)
	secGroupClient := netsecgroup.NewClient(managementClient)

	log.Println("[INFO] Retrieving current network configuration from Azure.")
	netConf, err := networkClient.GetVirtualNetworkConfiguration()
	if err != nil {
		return fmt.Errorf("Error while retrieving current network configuration: %s", err)
	}

	name := d.Get("name").(string)
	location := d.Get("location").(string)

	for _, vnet := range netConf.Configuration.VirtualNetworkSites {
		if vnet.Name == name && vnet.Location == location {
			d.Set("address_space_prefixes", vnet.AddressSpace.AddressPrefix)

			// read subnets:
			subnets := make([]map[string]interface{}, 0, 1)
			for i, sub := range vnet.Subnets {
				secGroup, err := secGroupClient.GetNetworkSecurityGroupForSubnet(sub.Name, name)
				if err != nil {
					return fmt.Errorf("Error whilst reading security groups for subnet %d: %s", i+1, err)
				}
				subnets = append(subnets, map[string]interface{}{
					"name":                sub.Name,
					"prefix":              sub.AddressPrefix,
					"security_group_name": secGroup.Name,
				})
			}
			d.Set("subnet", subnets)

			// read dns server references:
			dnsRefs := []string{}
			for _, dns := range vnet.DnsServersRef {
				dnsRefs = append(dnsRefs, dns.Name)
			}
			d.Set("dns_servers_names", dnsRefs)
		}
	}

	return nil
}

// resourceAzureVirtualNetworkUpdate does all the necessary API calls to
// update the status of our virtual network.
func resourceAzureVirtualNetworkUpdate(d *schema.ResourceData, meta interface{}) error {
	azureClient, ok := meta.(*AzureClient)
	if !ok {
		return fmt.Errorf("Failed to convert to *AzureClient, got: %T", meta)
	}
	managementClient := azureClient.managementClient
	networkClient := virtualnetwork.NewClient(managementClient)
	secGroupClient := netsecgroup.NewClient(managementClient)

	// get networking configurations from Azure:
	log.Println("[DEBUG] Changes to Azure virtual network exist; applying now.")
	log.Println("[INFO] Retrieving current network configuration from Azure.")
	azureClient.mutex.Lock()
	netConf, err := networkClient.GetVirtualNetworkConfiguration()
	if err != nil {
		return fmt.Errorf("Error while retrieving current network configuration: %s", err)
	}

	// examine for changes:
	name := d.Get("name").(string)
	location := d.Get("location").(string)
	cprefixes := d.HasChange("address_space_prefixes")
	cdnses := d.HasChange("dns_servers_names")
	csubnets := d.HasChange("subnet")

	var found bool
	vnets := netConf.Configuration.VirtualNetworkSites

	// search fo our virtual network and apply any changes:
	for i, vnet := range vnets {
		if vnet.Name == name && vnet.Location == location {
			found = true

			// apply adress space prefixe changes, if required:
			if cprefixes {
				var prefixes []string
				if nprefixes := d.Get("address_space_prefixes.#").(int); nprefixes > 0 {
					prefixes = []string{}
					for i := 0; i < nprefixes; i++ {
						prefixes = append(prefixes, d.Get(fmt.Sprintf("address_space_prefixes.%d", i)).(string))
					}
				}
				vnets[i].AddressSpace.AddressPrefix = prefixes
			}

			// apply dns server references, if required:
			if cdnses {
				var dnsRefs []virtualnetwork.DnsServerRef
				if ndnses := d.Get("dns_servers_names.#").(int); ndnses > 0 {
					dnsRefs = []virtualnetwork.DnsServerRef{}
					for i := 0; i < ndnses; i++ {
						dnsRefs = append(dnsRefs, virtualnetwork.DnsServerRef{
							Name: d.Get(fmt.Sprintf("dns_servers_names.%d", i)).(string),
						})
					}
				}
				vnets[i].DnsServersRef = dnsRefs
			}

			// apply subnet changes if required:
			if csubnets {
				var subnets []virtualnetwork.Subnet
				if nsubs := d.Get("subnet.#").(int); nsubs > 0 {
					subnets = []virtualnetwork.Subnet{}
					for i := 0; i < nsubs; i++ {
						sub := d.Get(fmt.Sprintf("subnet.%d", i)).(map[string]interface{})
						subName := sub["name"].(string)
						subnets = append(subnets, virtualnetwork.Subnet{
							Name:          subName,
							AddressPrefix: sub["prefix"].(string),
						})

						// check to see if we need to remove the old one:
						secGroup, err := secGroupClient.GetNetworkSecurityGroupForSubnet(subName, name)
						if err != nil {
							return fmt.Errorf("Error getting current network security group for subnet %d: %s", i+1, err)
						}
						// TODO(aznashwan): is this condition correct?
						if secGroup.Name != "" {
							// we must delte it first:
							reqID, err := secGroupClient.DeleteNetworkSecurityGroup(secGroup.Name)
							if err != nil {
								// TODO(aznashwan): see if this is right condition:
								return fmt.Errorf("Error issuing removal security group settings from subnet %d for update: %s", i+1, err)
							}
							err = managementClient.WaitAsyncOperation(reqID)
							if err != nil {
								return fmt.Errorf("Error removing security group settings from subnet %d for update: %s", i+1, err)
							}
						}

						// finally; add our own security group; if applicable:
						secGroupName := sub["security_group_name"].(string)
						if secGroupName != "" {
							log.Println(fmt.Sprintf("[INFO] Adding network security group settings to subnet %d.", i+1))
							reqID, err := secGroupClient.AddNetworkSecurityToSubnet(secGroupName, subName, name)
							if err != nil {
								return fmt.Errorf("Error issuing network security group settings application for subnet %d: %s", i, err)
							}
							err = managementClient.WaitAsyncOperation(reqID)
							if err != nil {
								return fmt.Errorf("Error removing network security group settings for subnet %d: %s", i+1, err)
							}
						}

					}
				}
				vnets[i].Subnets = subnets
			}
		}
	}

	// if the resource was not found; it means it was deleted from outside Terraform
	// and we must remove it from the schema:
	if !found {
		d.SetId("")
	} else if cprefixes || cdnses || csubnets {
		// if it was found and changes are due; return the new configuration to Azure:
		err = networkClient.SetVirtualNetworkConfiguration(netConf)
		if err != nil {
			return fmt.Errorf("Failed to set new Azure network configuration: %s", err)
		}

	}

	azureClient.mutex.Unlock()
	return nil
}

// resourceAzureVirtualNetworkExists does all the necessary API calls to
// check if the virtual network already exists.
func resourceAzureVirtualNetworkExists(d *schema.ResourceData, meta interface{}) (bool, error) {
	azureClient, ok := meta.(*AzureClient)
	if !ok {
		return false, fmt.Errorf("Failed to convert to *AzureClient, got: %T", meta)
	}
	managementClient := azureClient.managementClient
	networkClient := virtualnetwork.NewClient(managementClient)

	log.Println("[DEBUG] Changes to Azure virtual network exist; applying now.")
	log.Println("[INFO] Retrieving current network configuration from Azure.")
	netConf, err := networkClient.GetVirtualNetworkConfiguration()
	if err != nil {
		return false, fmt.Errorf("Error while retrieving current network configuration: %s", err)
	}

	name := d.Get("name")
	location := d.Get("location")

	// search for our virtual network:
	for _, vnet := range netConf.Configuration.VirtualNetworkSites {
		if vnet.Name == name && vnet.Location == location {
			return true, nil
		}
	}

	return false, nil
}

// resourceAzureVirtualNetworkDelete does all the necessary API calls to delete
// the virtual network off Azure.
func resourceAzureVirtualNetworkDelete(d *schema.ResourceData, meta interface{}) error {
	azureClient, ok := meta.(*AzureClient)
	if !ok {
		return fmt.Errorf("Failed to convert to *AzureClient, got: %T", meta)
	}
	managementClient := azureClient.managementClient
	networkClient := virtualnetwork.NewClient(managementClient)
	secGroupClient := netsecgroup.NewClient(managementClient)

	log.Println("[DEBUG] Changes to Azure virtual network exist; applying now.")
	log.Println("[INFO] Retrieving current network configuration from Azure.")
	azureClient.mutex.Lock()
	netConf, err := networkClient.GetVirtualNetworkConfiguration()
	if err != nil {
		return fmt.Errorf("Error while retrieving current network configuration: %s", err)
	}

	name := d.Get("name").(string)
	location := d.Get("location").(string)

	// remove all security group settings from all subnets before their deletion:
	if nsubs := d.Get("subnet.#").(int); nsubs > 0 {
		log.Println("[INFO] Started removing network security group settings from subnets.")
		for i := 0; i < nsubs; i++ {
			log.Printf("[DEBUG] Issuing network security group removal for subnet %d.\n", i+1)
			sub := d.Get(fmt.Sprintf("subnet.%d", i)).(map[string]interface{})
			subName := sub["name"].(string)
			secGroupName := sub["security_group_name"].(string)
			reqID, err := secGroupClient.RemoveNetworkSecurityGroupFromSubnet(
				secGroupName,
				subName,
				name,
			)
			if err != nil {
				return fmt.Errorf("Error issuing network security group removal from subnet %d: %s", i+1, err)
			}
			err = managementClient.WaitAsyncOperation(reqID)
			if err != nil {
				return fmt.Errorf("Error removing network security group settings from subnet %d: %s", i+1, err)
			}
		}
	}

	// look for our virtual network and remove it:
	var found bool
	for i, vnet := range netConf.Configuration.VirtualNetworkSites {
		if vnet.Name == name && vnet.Location == location {
			found = true
			netConf.Configuration.VirtualNetworkSites = append(
				netConf.Configuration.VirtualNetworkSites[:i],
				netConf.Configuration.VirtualNetworkSites[i+1:]...,
			)
		}
	}

	// if not found; it means the resource has been delted in the meantime;
	// so we stop keeping track if it:
	if !found {
		d.SetId("")
	} else {
		// else; send the updated configuration back:
		log.Println("[INFO] Sending virtual network configuration back to Azure.")
		err = networkClient.SetVirtualNetworkConfiguration(netConf)
		azureClient.mutex.Unlock()
		if err != nil {
			return fmt.Errorf("Failed updating network configuration: %s", err)
		}
	}

	d.SetId("")
	return nil
}
