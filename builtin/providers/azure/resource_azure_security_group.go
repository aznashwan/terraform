package azure

import (
	"fmt"
	"log"

	"github.com/Azure/azure-sdk-for-go/management"
	netsecgroup "github.com/Azure/azure-sdk-for-go/management/networksecuritygroup"
	"github.com/hashicorp/terraform/helper/schema"
)

// resourceAzureSecurityGroup returns the *schema.Resource associated to
// a network security group.
func resourceAzureSecurityGroup() *schema.Resource {
	return &schema.Resource{
		Create: resourceAzureSecurityGroupCreate,
		Read:   resourceAzureSecurityGroupRead,
		//	Update: resourceAzureSecurityGroupUpdate,
		Exists: resourceAzureSecurityGroupExists,
		Delete: resourceAzureSecurityGroupDelete,

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
		},
	}
}

// resourceAzureSecurityGroupCreate does all the necessary API calls to
// create the network security group on Azure.
func resourceAzureSecurityGroupCreate(d *schema.ResourceData, meta interface{}) error {
	azureClient, ok := meta.(*AzureClient)
	if !ok {
		return fmt.Errorf("Failed to convert to *AzureClient, got: %T", meta)
	}
	managementClient := azureClient.managementClient
	netSecClient := netsecgroup.NewClient(managementClient)

	name := d.Get("name").(string)
	location := d.Get("location").(string)
	label := getRandomStringLabel(50)

	// create the network security group:
	log.Println("[INFO] Sending network security group creating request to Azure.")
	reqID, err := netSecClient.CreateNetworkSecurityGroup(
		name,
		label,
		location,
	)
	if err != nil {
		return fmt.Errorf("Error whilst sending network security group create request to Azure: %s", err)
	}

	err = managementClient.WaitAsyncOperation(reqID)
	if err != nil {
		return fmt.Errorf("Error creating network security group on Azure: %s", err)
	}
	d.SetId(label)
	return nil
}

// resourceAzureSecurityGroupRead does all the necessary API calls to
// read the state of the network security group off Azure.
func resourceAzureSecurityGroupRead(d *schema.ResourceData, meta interface{}) error {
	_, err := resourceAzureSecurityGroupExists(d, meta)
	return err
}

// resourceAzureSecurityGroupUpdate does all the necessary API calls to
// update the state of the network security group on Azure.
// func resourceAzureSecurityGroupUpdate(d *schema.ResourceData, meta interface{}) error {
// redundant as all the parameters force new creation on change.
// }

// resourceAzureSecurityGroupExists does all the necessary API calls to
// check if the network security group already exists on Azure.
func resourceAzureSecurityGroupExists(d *schema.ResourceData, meta interface{}) (bool, error) {
	azureClient, ok := meta.(*AzureClient)
	if !ok {
		return false, fmt.Errorf("Failed to convert to *AzureClient, got: %T", meta)
	}
	netSecClient := netsecgroup.NewClient(azureClient.managementClient)

	name := d.Get("name").(string)
	log.Println("[INFO] Sending network security group query to Azure.")
	_, err := netSecClient.GetNetworkSecurityGroup(name)
	if err != nil {
		if !management.IsResourceNotFoundError(err) {
			return false, fmt.Errorf("Error querying Azure for network security group: %s", err)
		} else {
			// it means that the resource has been deleted in the meantime,
			// in which case we remove it from the schema.
			d.SetId("")
			return false, nil
		}
	}

	return true, nil
}

// resourceAzureSecurityGroupDelete does all the necessary API calls to
// delete a network security group off Azure.
func resourceAzureSecurityGroupDelete(d *schema.ResourceData, meta interface{}) error {
	azureClient, ok := meta.(*AzureClient)
	if !ok {
		return fmt.Errorf("Failed to convert to *AzureClient, got: %T", meta)
	}
	managementClient := azureClient.managementClient
	netSecClient := netsecgroup.NewClient(managementClient)

	name := d.Get("name").(string)
	log.Println("[INFO] Issuing network security delete to Azure.")
	reqID, err := netSecClient.DeleteNetworkSecurityGroup(name)
	if err != nil {
		return fmt.Errorf("Error whilst issuing Azure network security group deletion: %s", err)
	}
	err = managementClient.WaitAsyncOperation(reqID)
	if err != nil {
		return fmt.Errorf("Error in Azure network security group deletion: %s", err)
	}

	d.SetId("")
	return nil
}
