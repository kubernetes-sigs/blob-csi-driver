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
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	armauthorization "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	armcompute "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v7"
	resources "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/google/uuid"
	"sigs.k8s.io/blob-csi-driver/pkg/blob"
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
	vmssClient           *armcompute.VirtualMachineScaleSetsClient
	vmClient             *armcompute.VirtualMachinesClient
	roleClient           *armauthorization.RoleAssignmentsClient
}

func GetClient(cloud, subscriptionID, clientID, tenantID, clientSecret string, aadFederatedTokenFile string) (*Client, error) {
	armConfig := &azclient.ARMClientConfig{
		Cloud:     cloud,
		TenantID:  tenantID,
		UserAgent: blob.GetUserAgent(blob.DefaultDriverName, "", "e2e-test"),
	}
	clientOps, _, err := azclient.GetAzureCloudConfigAndEnvConfig(armConfig)
	if err != nil {
		return nil, err
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
	}, armConfig, clientOps, cred)
	if err != nil {
		return nil, err
	}
	roleclient, err := roledefinitionclient.New(cred, nil)
	if err != nil {
		return nil, err
	}

	armClientOpts, err := azclient.GetDefaultResourceClientOption(armConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to get default ARM client options: %v", err)
	}
	armClientOpts.Cloud = clientOps

	vmssClient, err := armcompute.NewVirtualMachineScaleSetsClient(subscriptionID, cred, armClientOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create VMSS client: %v", err)
	}
	vmClient, err := armcompute.NewVirtualMachinesClient(subscriptionID, cred, armClientOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create VM client: %v", err)
	}
	roleAssignClient, err := armauthorization.NewRoleAssignmentsClient(subscriptionID, cred, armClientOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create role assignments client: %v", err)
	}

	return &Client{
		subscriptionID:       subscriptionID,
		groupsClient:         factory.GetResourceGroupClient(),
		accountsClient:       factory.GetAccountClient(),
		roleassignmentClient: factory.GetRoleAssignmentClient(),
		vaultClient:          factory.GetVaultClient(),
		roledefinitionClient: roleclient,
		identityClient:       factory.GetIdentityClient(),
		vmssClient:           vmssClient,
		vmClient:             vmClient,
		roleClient:           roleAssignClient,
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

// NodeIdentityInfo holds the principal ID and client ID of a managed identity.
type NodeIdentityInfo struct {
	PrincipalID string
	ClientID    string
}

// GetNodeIdentityInfo retrieves the principal IDs and client IDs of managed identities from VMSS and VMs in the resource group.
// Returns the first user-assigned identity's client ID for use in e2e tests.
func (az *Client) GetNodeIdentityInfo(ctx context.Context, resourceGroup string) ([]NodeIdentityInfo, error) {
	var identities []NodeIdentityInfo

	// Check VMSS
	log.Printf("Listing VMSS in resource group %s ...", resourceGroup)
	vmssPager := az.vmssClient.NewListPager(resourceGroup, nil)
	for vmssPager.More() {
		page, err := vmssPager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list VMSS in resource group %s: %v", resourceGroup, err)
		}
		for _, vmss := range page.Value {
			name := ""
			if vmss.Name != nil {
				name = *vmss.Name
			}
			if vmss.Identity == nil {
				log.Printf("VMSS %s has no managed identity, skipping", name)
				continue
			}
			// System-assigned identity
			if vmss.Identity.PrincipalID != nil && *vmss.Identity.PrincipalID != "" {
				log.Printf("VMSS %s: found system-assigned identity, principalID=%s", name, *vmss.Identity.PrincipalID)
				identities = append(identities, NodeIdentityInfo{PrincipalID: *vmss.Identity.PrincipalID})
			}
			// User-assigned identities
			for uaID, uaIdentity := range vmss.Identity.UserAssignedIdentities {
				if uaIdentity != nil && uaIdentity.PrincipalID != nil && *uaIdentity.PrincipalID != "" {
					clientID := ""
					if uaIdentity.ClientID != nil {
						clientID = *uaIdentity.ClientID
					}
					log.Printf("VMSS %s: found user-assigned identity %s, principalID=%s, clientID=%s", name, uaID, *uaIdentity.PrincipalID, clientID)
					identities = append(identities, NodeIdentityInfo{PrincipalID: *uaIdentity.PrincipalID, ClientID: clientID})
				}
			}
		}
	}

	// Check individual VMs (availability set scenario)
	log.Printf("Listing VMs in resource group %s ...", resourceGroup)
	vmPager := az.vmClient.NewListPager(resourceGroup, nil)
	for vmPager.More() {
		page, err := vmPager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list VMs in resource group %s: %v", resourceGroup, err)
		}
		for _, vm := range page.Value {
			name := ""
			if vm.Name != nil {
				name = *vm.Name
			}
			if vm.Identity == nil {
				log.Printf("VM %s has no managed identity, skipping", name)
				continue
			}
			// System-assigned identity
			if vm.Identity.PrincipalID != nil && *vm.Identity.PrincipalID != "" {
				log.Printf("VM %s: found system-assigned identity, principalID=%s", name, *vm.Identity.PrincipalID)
				identities = append(identities, NodeIdentityInfo{PrincipalID: *vm.Identity.PrincipalID})
			}
			// User-assigned identities
			for uaID, uaIdentity := range vm.Identity.UserAssignedIdentities {
				if uaIdentity != nil && uaIdentity.PrincipalID != nil && *uaIdentity.PrincipalID != "" {
					clientID := ""
					if uaIdentity.ClientID != nil {
						clientID = *uaIdentity.ClientID
					}
					log.Printf("VM %s: found user-assigned identity %s, principalID=%s, clientID=%s", name, uaID, *uaIdentity.PrincipalID, clientID)
					identities = append(identities, NodeIdentityInfo{PrincipalID: *uaIdentity.PrincipalID, ClientID: clientID})
				}
			}
		}
	}

	if len(identities) == 0 {
		return nil, fmt.Errorf("no VMSS or VM with managed identity found in resource group %s", resourceGroup)
	}

	// Deduplicate by principalID
	seen := make(map[string]bool)
	unique := identities[:0]
	for _, id := range identities {
		if !seen[id.PrincipalID] {
			seen[id.PrincipalID] = true
			unique = append(unique, id)
		}
	}
	log.Printf("Found %d unique identities in resource group %s", len(unique), resourceGroup)
	return unique, nil
}

// AssignRoleToIdentity assigns an Azure role to a principal on a resource group scope.
// roleDefinitionID should be just the GUID of the role definition.
func (az *Client) AssignRoleToIdentity(ctx context.Context, resourceGroup, principalID, roleDefinitionID string) error {
	scope := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s", az.subscriptionID, resourceGroup)
	fullRoleDefID := fmt.Sprintf("/subscriptions/%s/providers/Microsoft.Authorization/roleDefinitions/%s", az.subscriptionID, roleDefinitionID)

	// Deterministic assignment ID based on scope+principal+role to avoid duplicate assignments on retry
	assignmentID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(scope+"|"+principalID+"|"+roleDefinitionID)).String()

	_, err := az.roleClient.Create(ctx, scope, assignmentID, armauthorization.RoleAssignmentCreateParameters{
		Properties: &armauthorization.RoleAssignmentProperties{
			PrincipalID:      &principalID,
			PrincipalType:    to.Ptr(armauthorization.PrincipalTypeServicePrincipal),
			RoleDefinitionID: &fullRoleDefID,
		},
	}, nil)
	if err != nil {
		// Ignore conflict only when role assignment already exists
		var respErr *azcore.ResponseError
		if ok := errors.As(err, &respErr); ok && respErr.StatusCode == http.StatusConflict && respErr.ErrorCode == "RoleAssignmentExists" {
			return nil
		}
		return fmt.Errorf("failed to create role assignment for principal %s with role %s on scope %s: %v", principalID, roleDefinitionID, scope, err)
	}
	return nil
}

// EnsureNodeStorageBlobDataRole gets the node identities from VMSS and VMs in the given
// resource group and assigns them the "Storage Blob Data Contributor" role.
// Returns the first user-assigned identity client ID for use in StorageClass parameters.
func (az *Client) EnsureNodeStorageBlobDataRole(ctx context.Context, resourceGroup string) (string, error) {
	log.Printf("Begin to assign Storage Blob Data Contributor role to node identities in resource group %s", resourceGroup)
	identities, err := az.GetNodeIdentityInfo(ctx, resourceGroup)
	if err != nil {
		log.Printf("Failed to get node identity info: %v", err)
		return "", err
	}

	// Storage Blob Data Contributor
	const storageBlobDataContributorRoleID = "ba92f5b4-2d11-453d-a403-e96b0029c9fe"
	var firstClientID string
	for _, identity := range identities {
		log.Printf("Assigning Storage Blob Data Contributor role to principal %s ...", identity.PrincipalID)
		if err := az.AssignRoleToIdentity(ctx, resourceGroup, identity.PrincipalID, storageBlobDataContributorRoleID); err != nil {
			log.Printf("Failed to assign role to principal %s: %v", identity.PrincipalID, err)
			return "", err
		}
		log.Printf("Successfully assigned Storage Blob Data Contributor role to principal %s", identity.PrincipalID)
		if firstClientID == "" && identity.ClientID != "" {
			firstClientID = identity.ClientID
		}
	}
	log.Printf("Successfully assigned Storage Blob Data Contributor role to all %d node identities", len(identities))
	return firstClientID, nil
}
