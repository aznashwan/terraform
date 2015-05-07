package azure

import (
	"fmt"
	"log"

	"github.com/Azure/azure-sdk-for-go/management/hostedservice"
	"github.com/hashicorp/terraform/helper/schema"
)

// resourceAzureHostedService returns the schema.Resource associated to an
// Azure hosted service.
func resourceAzureHostedService() *schema.Resource {
	return &schema.Resource{
		Create: resourceAzureHostedServiceCreate,
		Read:   resourceAzureHostedServiceRead,
		Update: resourceAzureHostedServiceUpdate,
		Delete: resourceAzureHostedServiceDelete,

		SchemaVersion: 1,

		Schema: map[string]*schema.Schema{
			"service_name": &schema.Schema{
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: parameterDescriptions["service_name"],
			},
			"location": &schema.Schema{
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: parameterDescriptions["location"],
			},
			"url": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},
			"ephemeral_contents": &schema.Schema{
				Type:     schema.TypeBool,
				Required: true,
				DefaultFunc: func() (interface{}, error) {
					return false, nil
				},
				Description: parameterDescriptions["ephemeral_contents"],
			},
			"status": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},
			"reverse_dns_fqdn": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "",
				Description: parameterDescriptions["reverse_dns_fqdn"],
			},
			"description": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "",
				Description: parameterDescriptions["description"],
			},
			"default_certificate_thumbprint": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "",
				Description: parameterDescriptions["default_certificate_thumbprint"],
			},
		},
	}
}

// resourceAzureHostedServiceCreate does all the necessary API calls
// to create a hosted service on Azure.
func resourceAzureHostedServiceCreate(d *schema.ResourceData, meta interface{}) error {
	azureClient, ok := meta.(*AzureClient)
	if !ok {
		return fmt.Errorf("Failed to convert to *AzureClient, got: %T", meta)
	}
	managementClient := azureClient.managementClient
	hostedServiceClient := hostedservice.NewClient(managementClient)

	serviceName := d.Get("service_name").(string)
	location := d.Get("location").(string)
	reverseDNS := d.Get("reverse_dns_fqdn").(string)
	description := d.Get("description").(string)

	// set the label as the resource's ID:
	label := getRandomStringLabel(50)
	d.SetId(label)

	reqID, err := hostedServiceClient.CreateHostedService(
		serviceName,
		location,
		reverseDNS,
		label,
		description,
	)
	if err != nil {
		return fmt.Errorf("Failed defining new Azure hosted service: %s", err)
	}

	log.Println("[DEBUG] Waiting for hosted service creation.")
	log.Println("[INFO] Creating new Azure hosted service.")
	err = managementClient.WaitAsyncOperation(reqID)
	if err != nil {
		return fmt.Errorf("Failed creating new Azure hosted service: %s", err)
	}

	return nil
}

// resourceAzureHostedServiceExists checks whether a hosted service with the
// given service_name already exists on Azure.
func resourceAzureHostedServiceExists(d *schema.ResourceData, meta interface{}) (bool, error) {
	azureClient, ok := meta.(*AzureClient)
	if !ok {
		return false, fmt.Errorf("Failed to convert to *AzureClient, got: %T", meta)
	}
	hostedServiceClient := hostedservice.NewClient(azureClient.managementClient)

	log.Println("[INFO] Querying for hosted service existence.")
	serviceName := d.Get("service_name").(string)
	exists, _, err := hostedServiceClient.CheckHostedServiceNameAvailability(serviceName)
	if err != nil {
		return false, fmt.Errorf("Failed to query for hosted service name availability: %s", err)
	}

	return exists, nil
}

// resourceAzureHostedServiceRead does all the necessary API calls
// to read the state of a hosted service from Azure.
func resourceAzureHostedServiceRead(d *schema.ResourceData, meta interface{}) error {
	azureClient, ok := meta.(*AzureClient)
	if !ok {
		return fmt.Errorf("Failed to convert to *AzureClient, got: %T", meta)
	}
	hostedServiceClient := hostedservice.NewClient(azureClient.managementClient)

	log.Println("[INFO] Querying for hosted service info.")
	serviceName := d.Get("service_name").(string)
	hostedService, err := hostedServiceClient.GetHostedService(serviceName)
	if err != nil {
		return fmt.Errorf("Failed to get hosted service: %s", err)
	}

	log.Println("[DEBUG] Reading hosted service query result data.")
	d.Set("service_name", hostedService.ServiceName)
	d.Set("url", hostedService.Url)
	d.Set("location", hostedService.Location)
	d.SetId(hostedService.Label)
	d.Set("description", hostedService.Description)
	d.Set("status", hostedService.Status)
	d.Set("reverse_dns_fqdn", hostedService.ReverseDnsFqdn)
	d.Set("default_certificate_thumbprint", hostedService.DefaultWinRmCertificateThumbprint)

	return nil
}

// resourceAzureHostedServiceUpdate does all the necessary API calls to
// update some settings of a hosted service on Azure.
func resourceAzureHostedServiceUpdate(d *schema.ResourceData, meta interface{}) error {
	return nil
}

// resourceAzureHostedServiceDelete does all the necessary API calls to
// delete a hosted service from Azure.
func resourceAzureHostedServiceDelete(d *schema.ResourceData, meta interface{}) error {
	azureClient, ok := meta.(*AzureClient)
	if !ok {
		return fmt.Errorf("Failed to convert to *AzureClient, got: %T", meta)
	}
	managementClient := azureClient.managementClient
	hostedServiceClient := hostedservice.NewClient(managementClient)

	log.Println("[INFO] Issuing hosted service deletion.")
	serviceName := d.Get("service_name").(string)
	ephemeral := d.Get("ephemeral_contents").(bool)
	reqID, err := hostedServiceClient.DeleteHostedService(serviceName, ephemeral)
	if err != nil {
		return fmt.Errorf("Failed issuing hosted service deletion request: %s", err)
	}

	log.Println("[DEBUG] Awaiting confirmation on hosted service deletion.")
	err = managementClient.WaitAsyncOperation(reqID)
	if err != nil {
		return fmt.Errorf("Error on hosted service deletion: %s", err)
	}

	d.SetId("")
	return nil
}
