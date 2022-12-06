/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package azure

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2018-05-01/resources"
	"github.com/Azure/azure-sdk-for-go/services/storage/mgmt/2021-09-01/storage"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/jongio/azidext/go/azidext"
)

type Client struct {
	environment    azure.Environment
	subscriptionID string
	groupsClient   resources.GroupsClient
	accountsClient storage.AccountsClient
}

func GetClient(cloud, subscriptionID, clientID, tenantID, clientSecret string) (*Client, error) {
	env, err := azure.EnvironmentFromName(cloud)
	if err != nil {
		return nil, err
	}

	options := azidentity.ClientSecretCredentialOptions{
		ClientOptions: azcore.ClientOptions{
			Cloud: getCloudConfig(env),
		},
	}
	cred, err := azidentity.NewClientSecretCredential(tenantID, clientID, clientSecret, &options)
	if err != nil {
		return nil, err
	}

	return getClient(env, subscriptionID, tenantID, cred, env.TokenAudience), nil
}

func (az *Client) EnsureResourceGroup(ctx context.Context, name, location string, managedBy *string) (resourceGroup *resources.Group, err error) {
	var tags map[string]*string
	group, err := az.groupsClient.Get(ctx, name)
	if err == nil && group.Tags != nil {
		tags = group.Tags
	} else {
		tags = make(map[string]*string)
	}
	if managedBy == nil {
		managedBy = group.ManagedBy
	}
	// Tags for correlating resource groups with prow jobs on testgrid
	tags["buildID"] = stringPointer(os.Getenv("BUILD_ID"))
	tags["jobName"] = stringPointer(os.Getenv("JOB_NAME"))
	tags["creationTimestamp"] = stringPointer(time.Now().UTC().Format(time.RFC3339))

	response, err := az.groupsClient.CreateOrUpdate(ctx, name, resources.Group{
		Name:      &name,
		Location:  &location,
		ManagedBy: managedBy,
		Tags:      tags,
	})
	if err != nil {
		return &response, err
	}

	return &response, nil
}

func (az *Client) DeleteResourceGroup(ctx context.Context, groupName string) error {
	_, err := az.groupsClient.Get(ctx, groupName)
	if err == nil {
		future, err := az.groupsClient.Delete(ctx, groupName)
		if err != nil {
			return fmt.Errorf("cannot delete resource group %v: %w", groupName, err)
		}
		err = future.WaitForCompletionRef(ctx, az.groupsClient.Client)
		if err != nil {
			// Skip the teardown errors because of https://github.com/Azure/go-autorest/issues/357
			// TODO(feiskyer): fix the issue by upgrading go-autorest version >= v11.3.2.
			log.Printf("Warning: failed to delete resource group %q with error %v", groupName, err)
		}
	}
	return nil
}

func (az *Client) GetAccountNumByResourceGroup(ctx context.Context, groupName string) (count int, err error) {
	result, err := az.accountsClient.ListByResourceGroup(ctx, groupName)
	if err != nil {
		return -1, err
	}
	return len(result.Values()), nil
}

func getCloudConfig(env azure.Environment) cloud.Configuration {
	switch env.Name {
	case azure.USGovernmentCloud.Name:
		return cloud.AzureGovernment
	case azure.ChinaCloud.Name:
		return cloud.AzureChina
	case azure.PublicCloud.Name:
		return cloud.AzurePublic
	default:
		return cloud.Configuration{
			ActiveDirectoryAuthorityHost: env.ActiveDirectoryEndpoint,
			Services: map[cloud.ServiceName]cloud.ServiceConfiguration{
				cloud.ResourceManager: {
					Audience: env.TokenAudience,
					Endpoint: env.ResourceManagerEndpoint,
				},
			},
		}
	}
}

func getClient(env azure.Environment, subscriptionID, tenantID string, cred *azidentity.ClientSecretCredential, scope string) *Client {
	c := &Client{
		environment:    env,
		subscriptionID: subscriptionID,
		groupsClient:   resources.NewGroupsClientWithBaseURI(env.ResourceManagerEndpoint, subscriptionID),
		accountsClient: storage.NewAccountsClient(subscriptionID),
	}

	if !strings.HasSuffix(scope, "/.default") {
		scope += "/.default"
	}
	// Use an adapter so azidentity in the Azure SDK can be used as Authorizer
	// when calling the Azure Management Packages, which we currently use. Once
	// the Azure SDK clients (found in /sdk) move to stable, we can update our
	// clients and they will be able to use the creds directly without the
	// authorizer.
	authorizer := azidext.NewTokenCredentialAdapter(cred, []string{scope})
	c.groupsClient.Authorizer = authorizer
	c.accountsClient.Authorizer = authorizer

	return c
}

func stringPointer(s string) *string {
	return &s
}
