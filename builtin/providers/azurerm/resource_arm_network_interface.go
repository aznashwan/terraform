package azurerm

import (
	"fmt"
	"net/http"
	"time"

	"github.com/Azure/azure-sdk-for-go/arm/network"
	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
)

// resourceArmNetworkInterface returns the *schema.Resource
// associated to network interface resources on ARM.
func resourceArmNetworkInterface() *schema.Resource {
	return &schema.Resource{
		Create: resourceArmNetworkInterfaceCreate,
		Read:   resourceArmNetworkInterfaceRead,
		Update: resourceArmNetworkInterfaceCreate,
		Delete: resourceArmNetworkInterfaceDelete,

		Schema: map[string]*schema.Schema{
			"name": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"resource_group_name": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"location": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"vm_id": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true, // TODET
			},

			"mac_address": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},

			"network_security_group_id": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true, // TODET
			},

			"ip_config": &schema.Schema{
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id": &schema.Schema{
							Type:     schema.TypeString,
							Computed: true,
						},
						"name": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
							ForceNew: true,
						},
						"private_ip_address": &schema.Schema{
							Type: schema.TypeString,
							// required only when 'dynamic_provate_ip' is NOT set.
							Optional:      true,
							ConflictsWith: []string{"dynamic_private_ip"},
						},
						"dynamic_private_ip": &schema.Schema{
							Type:          schema.TypeBool,
							Optional:      true,
							Default:       true,
							ConflictsWith: []string{"private_ip_address"},
						},
						"subnet_id": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},
						"public_ip_id": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
						},
					},
				},
			},

			"dns_servers": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},

			"applied_dns_servers": &schema.Schema{
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},

			"internal_name": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
			},

			"internal_fqdn": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
			},
		},
	}
}

// resourceArmNetworkInterfaceCreate goes ahead and creates the specified ARM network interface.
func resourceArmNetworkInterfaceCreate(d *schema.ResourceData, meta interface{}) error {
	ifaceClient := meta.(*ArmClient).ifaceClient

	name := d.Get("name").(string)
	location := d.Get("location").(string)
	resGrp := d.Get("resource_group_name").(string)
	vmId := d.Get("vm_id").(string)

	fields := getStringFields(d, []string{"network_security_group_id", "internal_name", "internal_fqdn"})

	// get dns servers:
	var dnses []string
	if v, ok := d.GetOk("dns_servers"); ok {
		for _, dns := range v.([]interface{}) {
			dnses = append(dnses, dns.(string))
		}
	}

	// get applied dns servers:
	var usedDnses []string
	if v, ok := d.GetOk("applied_dns_servers"); ok {
		for _, adns := range v.([]interface{}) {
			usedDnses = append(usedDnses, adns.(string))
		}
	}

	// get ip configurations:
	var ipconfigs []network.InterfaceIPConfiguration
	if configs := d.Get("ip_config").([]interface{}); len(configs) > 0 {
		for _, ipconfig := range configs {
			conf := ipconfig.(map[string]interface{})

			name := conf["name"].(string)
			sub := conf["subnet_id"].(string)

			// set the allocation method and respective address:
			var addr string
			var allocMeth network.IPAllocationMethod
			if b, ok := conf["dynamic_private_ip"]; ok && b.(bool) {
				allocMeth = network.Dynamic
				addr = ""
			} else {
				allocMeth = network.Static
				addr = conf["private_ip_address"].(string)
			}

			// get the optional public IP to bind to:
			var pubip string
			if v, ok := conf["public_ip_id"]; ok {
				pubip = v.(string)
			}

			ipconfigs = append(ipconfigs, network.InterfaceIPConfiguration{
				Name: &name,
				Properties: &network.InterfaceIPConfigurationPropertiesFormat{
					PrivateIPAddress:          &addr,
					PrivateIPAllocationMethod: allocMeth,
					Subnet:          &network.SubResource{&sub},
					PublicIPAddress: &network.SubResource{&pubip},
				},
			})
		}
	}

	resp, err := ifaceClient.CreateOrUpdate(resGrp, name, network.Interface{
		Name:     &name,
		Location: &location,
		Properties: &network.InterfacePropertiesFormat{
			VirtualMachine: &network.SubResource{&vmId},
			DNSSettings: &network.InterfaceDNSSettings{
				DNSServers:           &dnses,
				AppliedDNSServers:    &usedDnses,
				InternalDNSNameLabel: fields["internal_name"],
				InternalFqdn:         fields["internal_fqdn"],
			},
		},
	})
	if err != nil {
		return fmt.Errorf("Error encountered while issuing ARM interface %q creation: %s", name, err)
	}

	d.SetId(*resp.ID)

	// TODO: wait for state:
	stateConf := &resource.StateChangeConf{
		Pending: []string{"Accepted", "Updating"},
		Target:  "Succeded",
		Refresh: interfaceStateRefreshFunc(meta, name, resGrp),
		Timeout: 10 * time.Minute,
	}
	if _, err := stateConf.WaitForState(); err != nil {
		return fmt.Errorf("Error waiting for ARM interface %q creation: %s", name, err)
	}

	return resourceArmNetworkInterfaceRead(d, meta)
}

// resourceArmNetworkInterfaceRead goes ahead and reads the state of the corresponding ARM network interface.
func resourceArmNetworkInterfaceRead(d *schema.ResourceData, meta interface{}) error {
	ifaceClient := meta.(*ArmClient).ifaceClient

	// parse the id to get the name of the interface
	// and containing resource group:
	id, err := parseAzureResourceID(d.Id())
	if err != nil {
		return fmt.Errorf("Error parsing id of ARM interface: %s", err)
	}

	resGrp := id.ResourceGroup
	name := id.Path["networkInterfaces"]

	// fetch the interface off Azure:
	iface, err := ifaceClient.Get(resGrp, name)
	if iface.StatusCode == http.StatusNotFound {
		d.SetId("")
		return nil
	}
	if err != nil {
		return fmt.Errorf("Error reading the state of network interface %q off Azure: %s", name, err)
	}

	// now update all the variable fields:
	props := *iface.Properties

	d.Set("vm_id", *props.VirtualMachine.ID)
	d.Set("network_security_group_id", *props.NetworkSecurityGroup.ID)
	d.Set("mac_address", *props.MacAddress)

	// get the ip configs:
	var ipConfigs []map[string]interface{}
	for _, ipconf := range *props.IPConfigurations {
		v := map[string]interface{}{}

		v["id"] = *ipconf.ID
		v["name"] = *ipconf.Name

		// check for the allocation method and the address it could imply:
		if ipconf.Properties.PrivateIPAllocationMethod == network.Static {
			v["private_ip_address"] = *ipconf.Properties.PrivateIPAddress
			v["dynamic_private_ip"] = false
		} else { // guaranteed to be network.Dynamic
			v["private_ip_address"] = ""
			v["dynamic_private_ip"] = true
		}

		v["subnet_id"] = *ipconf.Properties.Subnet.ID
		v["public_ip_id"] = *ipconf.Properties.PublicIPAddress.ID

		ipConfigs = append(ipConfigs, v)
	}
	d.Set("ipConfig", ipConfigs)

	// and finally; read the DNS settings:
	d.Set("dns_servers", *iface.Properties.DNSSettings.DNSServers)
	d.Set("applied_dns_servers", *iface.Properties.DNSSettings.AppliedDNSServers)
	d.Set("internal_name", *iface.Properties.DNSSettings.InternalDNSNameLabel)
	d.Set("internal_fqdn", *iface.Properties.DNSSettings.InternalFqdn)

	return nil
}

// resourceArmNetworkInterfaceDelete deletes the specified ARM network interface.
func resourceArmNetworkInterfaceDelete(d *schema.ResourceData, meta interface{}) error {
	ifaceClient := meta.(*ArmClient).ifaceClient

	id, err := parseAzureResourceID(d.Id())
	if err != nil {
		return fmt.Errorf("Error parsing ID of ARM network interface: %s")
	}

	resGrp := id.ResourceGroup
	name := id.Path["networkInterfaces"]

	_, err = ifaceClient.Delete(resGrp, name)
	if err != nil {
		return fmt.Errorf("Error issuing deletion fo ARM network interface %q: %s", name, err)
	}

	return nil
}

// getStringFields is a helper function which, given the *schema.ResourceData and
// a list of field names, returns a map[string]string containing the provided
// keys and the associated value(if available) or an empty string.
func getStringFields(d *schema.ResourceData, fields []string) map[string]*string {
	m := map[string]*string{}
	v := ""

	for _, field := range fields {
		if r, ok := d.GetOk(field); ok {
			s := r.(string)
			m[field] = &s
		} else {
			m[field] = &v
		}
	}

	return m
}

// interfaceStateRefreshFunc returns the resource.StateRefreshFunc for the
// given interface under the given resource group.
func interfaceStateRefreshFunc(meta interface{}, name, resGrp string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		resp, err := meta.(*ArmClient).ifaceClient.Get(resGrp, name)
		if err != nil {
			return nil, "", err
		}

		return resp, *resp.Properties.ProvisioningState, nil
	}
}
