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
	"os"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	resources "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/accountclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/identityclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/resourcegroupclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/roleassignmentclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/roledefinitionclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/vaultclient"
)

type Client struct {
	subscriptionID       string
	groupsClient         resourcegroupclient.Interface
	accountsClient       accountclient.Interface
	roledefinitionClient roledefinitionclient.Interface
	roleassignmentClient roleassignmentclient.Interface
	vaultClient          vaultclient.Interface
	identityClient       identityclient.Interface
}

func GetClient(cloud, subscriptionID, clientID, tenantID, clientSecret string, aadFederatedTokenFile string) (*Client, error) {
	armConfig := &azclient.ARMClientConfig{
		Cloud:    cloud,
		TenantID: tenantID,
	}
	useFederatedWorkloadIdentityExtension := false
	if aadFederatedTokenFile != "" {
		useFederatedWorkloadIdentityExtension = true
	}
	credProvider, err := azclient.NewAuthProvider(armConfig, &azclient.AzureAuthConfig{
		AADClientID:                           clientID,
		AADClientSecret:                       clientSecret,
		AADFederatedTokenFile:                 aadFederatedTokenFile,
		UseFederatedWorkloadIdentityExtension: useFederatedWorkloadIdentityExtension,
	})
	if err != nil {
		return nil, err
	}
	cred := credProvider.GetAzIdentity()
	factory, err := azclient.NewClientFactory(&azclient.ClientFactoryConfig{
		SubscriptionID: subscriptionID,
	}, armConfig, cred)
	if err != nil {
		return nil, err
	}
	roleclient, err := roledefinitionclient.New(cred, nil)
	if err != nil {
		return nil, err
	}
	return &Client{
		subscriptionID:       subscriptionID,
		groupsClient:         factory.GetResourceGroupClient(),
		accountsClient:       factory.GetAccountClient(),
		roleassignmentClient: factory.GetRoleAssignmentClient(),
		vaultClient:          factory.GetVaultClient(),
		roledefinitionClient: roleclient,
		identityClient:       factory.GetIdentityClient(),
	}, nil
}

func (az *Client) EnsureResourceGroup(ctx context.Context, name, location string, managedBy *string) (resourceGroup *resources.ResourceGroup, err error) {
	var tags map[string]*string
	group, err := az.groupsClient.Get(ctx, name)
	if err != nil {
		group = &resources.ResourceGroup{}
	}
	if err == nil && group.Tags != nil {
		tags = group.Tags
	} else {
		tags = make(map[string]*string)
	}
	if managedBy == nil {
		managedBy = group.ManagedBy
	}
	// Tags for correlating resource groups with prow jobs on testgrid
	tags["buildID"] = to.Ptr(os.Getenv("BUILD_ID"))
	tags["jobName"] = to.Ptr(os.Getenv("JOB_NAME"))
	tags["creationTimestamp"] = to.Ptr(time.Now().UTC().Format(time.RFC3339))

	response, err := az.groupsClient.CreateOrUpdate(ctx, name, resources.ResourceGroup{
		Name:      &name,
		Location:  &location,
		ManagedBy: managedBy,
		Tags:      tags,
	})
	if err != nil {
		return response, err
	}

	return response, nil
}

func (az *Client) DeleteResourceGroup(ctx context.Context, groupName string) error {
	_, err := az.groupsClient.Get(ctx, groupName)
	if err == nil {
		err := az.groupsClient.Delete(ctx, groupName)
		if err != nil {
			return fmt.Errorf("cannot delete resource group %v: %w", groupName, err)
		}
	}
	return nil
}

func (az *Client) GetAccountNumByResourceGroup(ctx context.Context, groupName string) (count int, err error) {
	result, err := az.accountsClient.List(ctx, groupName)
	if err != nil {
		return -1, err
	}
	return len(result), nil
}
