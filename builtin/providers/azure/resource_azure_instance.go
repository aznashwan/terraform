package azure

import (
	"fmt"
	"log"

	"github.com/Azure/azure-sdk-for-go/management"
	"github.com/Azure/azure-sdk-for-go/management/hostedservice"
	"github.com/Azure/azure-sdk-for-go/management/virtualmachine"
	"github.com/Azure/azure-sdk-for-go/management/vmutils"
	"github.com/hashicorp/terraform/helper/schema"
)

// resourceAzureInstance returns the schema.Resource associated to
// an Azure instance.
func resourceAzureInstance() *schema.Resource {
	return &schema.Resource{
		Create: resourceAzureInstanceCreate,
		Read:   resourceAzureInstanceRead,
		Update: resourceAzureInstanceUpdate,
		Delete: resourceAzureInstanceDelete,

		SchemaVersion: 1,

		Schema: map[string]*schema.Schema{
			// general attributes:
			"name": &schema.Schema{
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: parameterDescriptions["name"],
			},
			"service_name": &schema.Schema{
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: parameterDescriptions["service_name"],
			},
			"description": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				Description: parameterDescriptions["description"],
			},
			"image": &schema.Schema{
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: parameterDescriptions["image"],
			},
			// TODO(aznashwan): make this used:
			// 	- seperate config for Linux and Windows
			// 	- Windows to join domain.
			// 	- public SSH/RDP/PS1
			"os_type": &schema.Schema{
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: parameterDescriptions["os_type"],
			},
			// TODO(aznashwan): use IsRoleSizeValid here.
			"size": &schema.Schema{
				Type:        schema.TypeString,
				Required:    true,
				Description: parameterDescriptions["size"],
			},
			"location": &schema.Schema{
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: parameterDescriptions["location"],
			},
			// TODO(aznashwan): improve storage disk mechanism.
			// 	- existing disk image
			//	- arbitrary remote image
			// 	- add existing data disk
			// 	- add new data disk
			"storage_account": &schema.Schema{
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: parameterDescriptions["storage_account"],
			},
			"storage_container": &schema.Schema{
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: parameterDescriptions["storage_container"],
			},
			// login attributes:
			"user_name": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				Description: parameterDescriptions["user_name"],
			},
			"user_password": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				Description: parameterDescriptions["user_password"],
			},
			// "ssh_thumbprints": &schema.Schema{
			// Type:     schema.TypeList,
			// Optional: true,
			// Description: parameterDescriptions["ssh_thumbprints"],
			// },
			//
			// computed attributes:
			"status": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},
			"power_state": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},
			"private_ip": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},
			"host_name": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},
			"agent_status": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},
			// TODO(aznashwan): make these work.
			"public_ips": &schema.Schema{
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Schema{
					Type:     schema.TypeString,
					Computed: true,
				},
			},
			// TODO(aznashwan):
			// 	- configure with external ports
			//	- use virtualnetwork package where applicable.
		},
	}
}

// resourceAzureInstanceCreate does all the necessary API calls to create the
// configuration and deploy the Azure instance.
// TODO(aznashwan): use vmutils.WaitForDeploymentPowerState.
func resourceAzureInstanceCreate(d *schema.ResourceData, meta interface{}) error {
	azureClient, ok := meta.(*AzureClient)
	if !ok {
		return fmt.Errorf("Failed to convert to *AzureClient, got: %T", meta)
	}
	managementClient := azureClient.managementClient

	// general variables:
	label := getRandomStringLabel(50)
	d.SetId(label)
	image := d.Get("image").(string)
	name := d.Get("name").(string)
	location := d.Get("location").(string)
	serviceName := d.Get("service_name").(string)

	// create the hosted service:
	hostedServiceClient := hostedservice.NewClient(managementClient)

	// check is the hosted service already exists:
	_, err := hostedServiceClient.GetHostedService(serviceName)
	if err != nil {
		if serviceName == "" || management.IsResourceNotFoundError(err) {
			log.Println("[INFO] No hosted service with the given name exists, creating new one with the instance's name.")
			err := hostedServiceClient.CreateHostedService(
				hostedservice.CreateHostedServiceParameters{
					ServiceName:    name,
					Location:       location,
					ReverseDNSFqdn: "",
					Label:          label,
				},
			)
			if err != nil {
				return fmt.Errorf("Error defining new Azure hosted service: %s", err)
			}
		} else {
			fmt.Errorf("Error querying for existing hosted service.")
		}
	}

	// create VM configuration:
	role := vmutils.NewVMConfiguration(name, d.Get("size").(string))

	// configure the VM's storage:
	// TODO(aznashwan): put things right here:
	storAccount := d.Get("storage_account").(string)
	storContainer := d.Get("storage_container").(string)
	vhdURL := fmt.Sprintf("http://%s.blob.core.windows.net/%s/%s.vhd", storAccount, storContainer, name)

	err = vmutils.ConfigureDeploymentFromPlatformImage(&role, image, vhdURL, label)
	if err != nil {
		return fmt.Errorf("Failed to configure deployment: %s", err)
	}

	// configure VM details:
	userName := d.Get("user_name").(string)
	userPass := d.Get("user_password").(string)
	vmutils.ConfigureForLinux(&role, name, userName, userPass)
	vmutils.ConfigureWithPublicSSH(&role)

	// deploy the VM:
	reqID, err := virtualmachine.NewClient(managementClient).CreateDeployment(
		role,
		serviceName,
		virtualmachine.CreateDeploymentOptions{},
	)
	if err != nil {
		return fmt.Errorf("Failed to initiate deployment creation: %s", err)
	}

	err = managementClient.WaitForOperation(reqID, nil)
	if err != nil {
		return fmt.Errorf("Deployment creation failed: %s", err)
	}

	return resourceAzureInstanceRead(d, meta)
}

// resourceAzureInstanceRead does all the necessary API calls to read the state
// of an instance deployed on Azure.
func resourceAzureInstanceRead(d *schema.ResourceData, meta interface{}) error {
	azureClient, ok := meta.(*AzureClient)
	if !ok {
		return fmt.Errorf("Failed to convert to *AzureClient, got: %T", meta)
	}
	managementClient := azureClient.managementClient

	name := d.Get("name").(string)
	serviceName := d.Get("service_name").(string)
	vmClient := virtualmachine.NewClient(managementClient)

	log.Println("[INFO] Fetching deployment information.")
	deploy, err := vmClient.GetDeployment(serviceName, name)
	if err != nil {
		return fmt.Errorf("Failed to get deployment information.")
	}

	for _, role := range deploy.RoleInstanceList {
		if role.InstanceName == name {
			d.Set("status", role.InstanceStatus)
			d.Set("power_state", role.PowerState)
			// d.Set("private_ip", role.IpAddress)
			d.Set("host_name", role.HostName)
			d.Set("agent_status", role.GuestAgentStatus)

			pubIPs := []string{}
			for _, ip := range role.PublicIPs {
				pubIPs = append(pubIPs, ip.Name)
			}
			d.Set("public_ips", pubIPs)
		}
	}

	return nil
}

// resourceAzureInstanceUpdate does all the necessary API calls to update
// the configuration of an instance deployed on Azure.
func resourceAzureInstanceUpdate(d *schema.ResourceData, meta interface{}) error {
	return nil
}

// resourceAzureInstanceDelete dos all the necessary API calls to delete
// an instance running on Azure.
func resourceAzureInstanceDelete(d *schema.ResourceData, meta interface{}) error {
	azureClient, ok := meta.(*AzureClient)
	if !ok {
		return fmt.Errorf("Failed to convert to *AzureClient, got: %T", meta)
	}
	managementClient := azureClient.managementClient

	name := d.Get("name").(string)
	serviceName := d.Get("service_name").(string)
	vmClient := virtualmachine.NewClient(managementClient)

	log.Println("[INFO] Issuing deployment deletion.")
	reqID, err := vmClient.DeleteDeployment(serviceName, name)
	if err != nil {
		return fmt.Errorf("Failed to issue deployment deletion request: %s", err)
	}

	err = managementClient.WaitForOperation(reqID, nil)
	if err != nil {
		return fmt.Errorf("Deployment deletion failed: %s", err)
	}

	d.SetId("")
	return nil
}
