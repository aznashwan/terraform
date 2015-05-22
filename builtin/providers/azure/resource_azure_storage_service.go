package azure

import (
	"fmt"
	"log"

	"github.com/Azure/azure-sdk-for-go/management"
	"github.com/Azure/azure-sdk-for-go/management/storageservice"
	"github.com/hashicorp/terraform/helper/schema"
)

// resourceAzureStorageService returns the *schema.Resource associated
// to an Azure hosted service.
func resourceAzureStorageService() *schema.Resource {
	return &schema.Resource{
		Create: resourceAzureStorageServiceCreate,
		Read:   resourceAzureStorageServiceRead,
		// Update: resourceAzureStorageServiceUpdate,
		Exists: resourceAzureStorageServiceExists,
		Delete: resourceAzureStorageServiceDelete,

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
			"account_type": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				// ForceNew: true,
				Description: parameterDescriptions["account_type"],
			},
			"url": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},
			"description": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "",
				Description: parameterDescriptions["description"],
			},
			"affinity_group": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "",
				Description: parameterDescriptions["affinity_group"],
			},
			"properties": &schema.Schema{
				Type:     schema.TypeMap,
				Optional: true,
				Elem:     schema.TypeString,
			},
		},
	}
}

// resourceAzureStorageServiceCreate does all the necessary API calls to
// create a new Azure storage service.
func resourceAzureStorageServiceCreate(d *schema.ResourceData, meta interface{}) error {
	azureClient, ok := meta.(*AzureClient)
	if !ok {
		return fmt.Errorf("Failed to convert to *AzureClient, got: %T", meta)
	}
	managementClient := azureClient.managementClient
	storageServiceClient := storageservice.NewClient(managementClient)

	// get all the values:
	log.Println("[INFO] Creating Azure storage service creation parameters.")
	name := d.Get("name").(string)
	location := d.Get("location").(string)
	accountType := storageservice.AccountType(d.Get("account_type").(string))
	affinityGroup := d.Get("affinity_group").(string)
	description := d.Get("description").(string)
	label := getRandomStringLabel(20)
	var props []storageservice.ExtendedProperty
	if given := d.Get("properties").(map[string]interface{}); len(given) > 0 {
		props = []storageservice.ExtendedProperty{}
		for k, v := range given {
			props = append(props, storageservice.ExtendedProperty{
				Name:  k,
				Value: v.(string),
			})
		}
	}

	// create parameters and send request:
	log.Println("[INFO] Sending storage service creation request to Azure.")
	reqID, err := storageServiceClient.CreateStorageService(
		storageservice.StorageAccountCreateParameters{
			ServiceName:   name,
			Location:      location,
			Description:   description,
			Label:         label,
			AffinityGroup: affinityGroup,
			AccountType:   accountType,
			ExtendedProperties: storageservice.ExtendedPropertyList{
				ExtendedProperty: props,
			},
		})
	if err != nil {
		return fmt.Errorf("Failed to create Azure hosted service: %s", err)
	}
	err = managementClient.WaitForOperation(reqID, nil)
	if err != nil {
		return fmt.Errorf("Failed updating the network configuration: %s", err)
	}

	// TODO(aznashwan): find work around here:
	// get computed values:
	// d.Set("url", svc.Url)

	d.SetId(label)
	return resourceAzureStorageServiceRead(d, meta)
}

// resourceAzureStorageServiceRead does all the necessary API calls to
// read the state of the storage service off Azure.
func resourceAzureStorageServiceRead(d *schema.ResourceData, meta interface{}) error {
	azureClient, ok := meta.(*AzureClient)
	if !ok {
		return fmt.Errorf("Failed to convert to *AzureClient, got: %T", meta)
	}
	managementClient := azureClient.managementClient
	storageServiceClient := storageservice.NewClient(managementClient)

	// get our storage service:
	log.Println("[INFO] Sending query about storage service to Azure.")
	name := d.Get("name").(string)
	storsvc, err := storageServiceClient.GetStorageService(name)
	if err != nil {
		if !management.IsResourceNotFoundError(err) {
			return fmt.Errorf("Failed to query about Azure about storage service: %s", err)
		} else {
			// it means that the resource has been deleted from Azure
			// in the meantime and we must remove its associated Resource.
			d.SetId("")
			return nil

		}
	}

	// read values:
	d.Set("url", storsvc.URL)

	return nil
}

// TODO(aznashwan): is this necessary?
// resourceAzureStorageServiceUpdate does all the necessary API calls to
// update the parameters of the storage service on Azure.
// func resourceAzureStorageServiceUpdate(d *schema.ResourceData, meta interface{}) error {
//	return nil
// }

// resourceAzureStorageServiceExists does all the necessary API calls to
// check if the storage service exists on Azure.
func resourceAzureStorageServiceExists(d *schema.ResourceData, meta interface{}) (bool, error) {
	azureClient, ok := meta.(*AzureClient)
	if !ok {
		return false, fmt.Errorf("Failed to convert to *AzureClient, got: %T", meta)
	}
	managementClient := azureClient.managementClient
	storageServiceClient := storageservice.NewClient(managementClient)

	// get our storage service:
	log.Println("[INFO] Sending query about storage service to Azure.")
	name := d.Get("name").(string)
	_, err := storageServiceClient.GetStorageService(name)
	if err != nil {
		if !management.IsResourceNotFoundError(err) {
			return false, fmt.Errorf("Failed to query about Azure about storage service: %s", err)
		} else {
			// it means that the resource has been deleted from Azure
			// in the meantime and we must remove its associated Resource.
			d.SetId("")
			return false, nil

		}
	}

	return true, nil
}

// resourceAzureStorageServiceDelete does all the necessary API calls to
// delete the storage service off Azure.
func resourceAzureStorageServiceDelete(d *schema.ResourceData, meta interface{}) error {
	azureClient, ok := meta.(*AzureClient)
	if !ok {
		return fmt.Errorf("Failed to convert to *AzureClient, got: %T", meta)
	}
	managementClient := azureClient.managementClient
	storageClient := storageservice.NewClient(managementClient)

	// issue the deletion:
	name := d.Get("name").(string)
	log.Println("[INFO] Issuing delete of storage service off Azure.")
	reqID, err := storageClient.DeleteStorageService(name)
	if err != nil {
		return fmt.Errorf("Error whilst issuing deletion of storage service off Azure: %s", err)
	}
	err = managementClient.WaitForOperation(reqID, nil)
	if err != nil {
		return fmt.Errorf("Error whilst deleting storage service off Azure: %s", err)
	}

	d.SetId("")
	return nil
}
