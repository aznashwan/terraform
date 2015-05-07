package azure

import (
	"fmt"
	"log"
	"sync"

	"github.com/Azure/azure-sdk-for-go/management"
)

// Config is a struct which holds all the required information to access
// Azure services.
type Config struct {
	PublishSettingsFilePath string
	SubscriptionID          string
	ManagementCert          []byte
	ManagementUrl           string
}

// AzureClient contains all the handles required for managing Azure services.
type AzureClient struct {
	// the client which holds the authentification information
	managementClient management.Client

	// unfortunately; because of how Azure's network API works; doing networking operations
	// concurrently is very hazardous, and we need a mutex.
	mutex *sync.Mutex
}

// Client configures and returns a fully initialized Azure client.
func (c *Config) Client() (interface{}, error) {
	var err error
	var managementClient management.Client
	var azureClient AzureClient

	log.Println("[DEBUG] Building Azure management client.")
	if c.PublishSettingsFilePath != "" {
		managementClient, err = management.ClientFromPublishSettingsFile(
			c.PublishSettingsFilePath,
			c.SubscriptionID,
		)
	} else if c.ManagementUrl != "" {
		managementClient, err = management.NewClientFromConfig(
			c.SubscriptionID,
			c.ManagementCert,
			management.ClientConfig{c.ManagementUrl},
		)
	} else {
		managementClient, err = management.NewClient(
			c.SubscriptionID,
			c.ManagementCert,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("Failed to create Azure management client: %s", err)
	}
	azureClient.managementClient = managementClient

	azureClient.mutex = &sync.Mutex{}

	log.Println("[DEBUG] Built Azure management client.")
	return &azureClient, nil
}
