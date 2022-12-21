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

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault"
	"github.com/Azure/azure-sdk-for-go/services/graphrbac/1.6/graphrbac"
	"github.com/Azure/azure-sdk-for-go/services/msi/mgmt/2018-11-30/msi"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/onsi/ginkgo/v2"
	"k8s.io/apiserver/pkg/storage/names"
	"sigs.k8s.io/blob-csi-driver/test/utils/credentials"
)

type KeyVaultClient struct {
	Cred      *credentials.Credentials
	vaultName string
}

func NewKeyVaultClient() (*KeyVaultClient, error) {
	e2eCred, err := credentials.ParseAzureCredentialFile()
	if err != nil {
		return nil, err
	}

	return &KeyVaultClient{
		Cred: &credentials.Credentials{
			SubscriptionID:  e2eCred.SubscriptionID,
			ResourceGroup:   e2eCred.ResourceGroup,
			Location:        e2eCred.Location,
			TenantID:        e2eCred.TenantID,
			Cloud:           e2eCred.Cloud,
			AADClientID:     e2eCred.AADClientID,
			AADClientSecret: e2eCred.AADClientSecret,
		},
		vaultName: names.SimpleNameGenerator.GenerateName("blob-csi-test-kv-"),
	}, nil
}

func (kvc *KeyVaultClient) CreateVault(ctx context.Context) (*armkeyvault.Vault, error) {
	azureCred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, err
	}

	vaultsClient, err := armkeyvault.NewVaultsClient(kvc.Cred.SubscriptionID, azureCred, nil)
	if err != nil {
		return nil, err
	}

	pollerResp, err := vaultsClient.BeginCreateOrUpdate(
		ctx,
		kvc.Cred.ResourceGroup,
		kvc.vaultName,
		armkeyvault.VaultCreateOrUpdateParameters{
			Location: to.Ptr(kvc.Cred.Location),
			Properties: &armkeyvault.VaultProperties{
				SKU: &armkeyvault.SKU{
					Family: to.Ptr(armkeyvault.SKUFamilyA),
					Name:   to.Ptr(armkeyvault.SKUNameStandard),
				},
				TenantID:       to.Ptr(kvc.Cred.TenantID),
				AccessPolicies: kvc.getAccessPolicy(ctx),
			},
		},
		nil,
	)
	if err != nil {
		return nil, err
	}

	resp, err := pollerResp.PollUntilDone(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &resp.Vault, nil
}

func (kvc *KeyVaultClient) CleanVault(ctx context.Context) error {
	azureCred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return err
	}

	err = kvc.deleteVault(ctx, azureCred)
	if err != nil {
		return err
	}

	err = kvc.purgeDeleted(ctx, azureCred)
	if err != nil {
		return err
	}

	return nil
}

func (kvc *KeyVaultClient) CreateSecret(ctx context.Context, secretName, secretValue string) (*armkeyvault.Secret, error) {
	azureCred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, err
	}

	secretsClient, err := armkeyvault.NewSecretsClient(kvc.Cred.SubscriptionID, azureCred, nil)
	if err != nil {
		return nil, err
	}

	secretResp, err := secretsClient.CreateOrUpdate(
		ctx,
		kvc.Cred.ResourceGroup,
		kvc.vaultName,
		secretName,
		armkeyvault.SecretCreateOrUpdateParameters{
			Properties: &armkeyvault.SecretProperties{
				Attributes: &armkeyvault.SecretAttributes{
					Enabled: to.Ptr(true),
				},
				Value: to.Ptr(secretValue),
			},
		},
		nil,
	)
	if err != nil {
		return nil, err
	}

	return &secretResp.Secret, nil
}

func (kvc *KeyVaultClient) getAccessPolicy(ctx context.Context) []*armkeyvault.AccessPolicyEntry {
	accessPolicyEntry := []*armkeyvault.AccessPolicyEntry{}

	// vault secret permission for upstream e2e test, which uses application service principal
	clientObjectID, err := kvc.GetServicePrincipalObjectID(ctx, kvc.Cred.AADClientID)
	if err == nil {
		ginkgo.By("client object ID: " + clientObjectID)
		accessPolicyEntry = append(accessPolicyEntry, &armkeyvault.AccessPolicyEntry{
			TenantID: to.Ptr(kvc.Cred.TenantID),
			ObjectID: to.Ptr(clientObjectID),
			Permissions: &armkeyvault.Permissions{
				Secrets: []*armkeyvault.SecretPermissions{
					to.Ptr(armkeyvault.SecretPermissionsGet),
				},
			},
		})
	}

	// vault secret permission for upstream e2e-vmss test, which uses msi blobfuse-csi-driver-e2e-test-id
	msiObjectID, err := kvc.GetMSIObjectID(ctx, "blobfuse-csi-driver-e2e-test-id")
	if err == nil {
		ginkgo.By("MSI object ID: " + msiObjectID)
		accessPolicyEntry = append(accessPolicyEntry, &armkeyvault.AccessPolicyEntry{
			TenantID: to.Ptr(kvc.Cred.TenantID),
			ObjectID: to.Ptr(msiObjectID),
			Permissions: &armkeyvault.Permissions{
				Secrets: []*armkeyvault.SecretPermissions{
					to.Ptr(armkeyvault.SecretPermissionsGet),
				},
			},
		})
	}

	return accessPolicyEntry
}

func (kvc *KeyVaultClient) deleteVault(ctx context.Context, cred azcore.TokenCredential) error {
	vaultsClient, err := armkeyvault.NewVaultsClient(kvc.Cred.SubscriptionID, cred, nil)
	if err != nil {
		return err
	}

	_, err = vaultsClient.Delete(ctx, kvc.Cred.ResourceGroup, kvc.vaultName, nil)
	if err != nil {
		return err
	}
	return nil
}

func (kvc *KeyVaultClient) purgeDeleted(ctx context.Context, cred azcore.TokenCredential) error {
	vaultsClient, err := armkeyvault.NewVaultsClient(kvc.Cred.SubscriptionID, cred, nil)
	if err != nil {
		return err
	}

	pollerResp, err := vaultsClient.BeginPurgeDeleted(ctx, kvc.vaultName, kvc.Cred.Location, nil)
	if err != nil {
		return err
	}

	_, err = pollerResp.PollUntilDone(ctx, nil)
	if err != nil {
		return err
	}

	return nil
}

func (kvc *KeyVaultClient) GetServicePrincipalObjectID(ctx context.Context, clientID string) (string, error) {
	spClient, err := kvc.getServicePrincipalsClient()
	if err != nil {
		return "", err
	}

	page, err := spClient.List(ctx, fmt.Sprintf("servicePrincipalNames/any(c:c eq '%s')", clientID))
	if err != nil {
		return "", err
	}
	servicePrincipals := page.Values()
	if len(servicePrincipals) == 0 {
		return "", fmt.Errorf("didn't find any service principals for client ID %s", clientID)
	}
	return *servicePrincipals[0].ObjectID, nil
}

func (kvc *KeyVaultClient) getServicePrincipalsClient() (*graphrbac.ServicePrincipalsClient, error) {
	spClient := graphrbac.NewServicePrincipalsClient(kvc.Cred.TenantID)

	env, err := azure.EnvironmentFromName(kvc.Cred.Cloud)
	if err != nil {
		return nil, err
	}

	oauthConfig, err := adal.NewOAuthConfig(env.ActiveDirectoryEndpoint, kvc.Cred.TenantID)
	if err != nil {
		return nil, err
	}

	token, err := adal.NewServicePrincipalToken(*oauthConfig, kvc.Cred.AADClientID, kvc.Cred.AADClientSecret, env.GraphEndpoint)
	if err != nil {
		return nil, err
	}

	authorizer := autorest.NewBearerAuthorizer(token)

	spClient.Authorizer = authorizer

	return &spClient, nil
}

func (kvc *KeyVaultClient) getMSIUserAssignedIDClient() (*msi.UserAssignedIdentitiesClient, error) {
	msiClient := msi.NewUserAssignedIdentitiesClient(kvc.Cred.SubscriptionID)

	env, err := azure.EnvironmentFromName(kvc.Cred.Cloud)
	if err != nil {
		return nil, err
	}

	oauthConfig, err := adal.NewOAuthConfig(env.ActiveDirectoryEndpoint, kvc.Cred.TenantID)
	if err != nil {
		return nil, err
	}

	token, err := adal.NewServicePrincipalToken(*oauthConfig, kvc.Cred.AADClientID, kvc.Cred.AADClientSecret, env.ResourceManagerEndpoint)
	if err != nil {
		return nil, err
	}

	authorizer := autorest.NewBearerAuthorizer(token)

	msiClient.Authorizer = authorizer

	return &msiClient, nil
}

func (kvc *KeyVaultClient) GetMSIObjectID(ctx context.Context, identityName string) (string, error) {
	msiClient, err := kvc.getMSIUserAssignedIDClient()
	if err != nil {
		return "", err
	}

	id, err := msiClient.Get(ctx, kvc.Cred.ResourceGroup, identityName)
	if err != nil {
		return "", err
	}

	return id.UserAssignedIdentityProperties.PrincipalID.String(), err
}

func (kvc *KeyVaultClient) GetMSIResourceID(ctx context.Context, identityName string) (string, error) {
	msiClient, err := kvc.getMSIUserAssignedIDClient()
	if err != nil {
		return "", err
	}

	id, err := msiClient.Get(ctx, kvc.Cred.ResourceGroup, identityName)
	if err != nil {
		return "", err
	}

	return *id.ID, err
}
