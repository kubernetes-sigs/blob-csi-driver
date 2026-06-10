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

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault"
	"github.com/onsi/ginkgo/v2"
	"k8s.io/apiserver/pkg/storage/names"
	"sigs.k8s.io/blob-csi-driver/test/utils/credentials"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/identityclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/vaultclient"
)

type KeyVaultClient struct {
	vaultClient    vaultclient.Interface
	identityClient identityclient.Interface
	Cred           *credentials.Credentials
	vaultName      string
}

func NewKeyVaultClient() (*KeyVaultClient, error) {
	e2eCred, err := credentials.ParseAzureCredentialFile()
	if err != nil {
		return nil, err
	}
	client, err := GetClient(e2eCred.Cloud, e2eCred.SubscriptionID, e2eCred.AADClientID, e2eCred.TenantID, e2eCred.AADClientSecret, e2eCred.AADFederatedTokenFile)
	if err != nil {
		return nil, err
	}
	return &KeyVaultClient{
		vaultName:      names.SimpleNameGenerator.GenerateName("blob-csi-test-kv-"),
		vaultClient:    client.vaultClient,
		identityClient: client.identityClient,
		Cred:           e2eCred,
	}, nil
}

func (kvc *KeyVaultClient) CreateVault(ctx context.Context) (*armkeyvault.Vault, error) {

	resp, err := kvc.vaultClient.CreateOrUpdate(
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
	)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (kvc *KeyVaultClient) CleanVault(ctx context.Context) error {
	err := kvc.deleteVault(ctx)
	if err != nil {
		return err
	}

	err = kvc.purgeDeleted(ctx)
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

func (kvc *KeyVaultClient) deleteVault(ctx context.Context) error {
	err := kvc.vaultClient.Delete(ctx, kvc.Cred.ResourceGroup, kvc.vaultName)
	if err != nil {
		return err
	}
	return nil
}

func (kvc *KeyVaultClient) purgeDeleted(ctx context.Context) error {
	err := kvc.vaultClient.PurgeDeleted(ctx, kvc.vaultName, kvc.Cred.Location)
	if err != nil {
		return err
	}
	return nil
}

func (kvc *KeyVaultClient) GetMSIObjectID(ctx context.Context, identityName string) (string, error) {
	id, err := kvc.identityClient.Get(ctx, kvc.Cred.ResourceGroup, identityName)
	if err != nil {
		return "", err
	}
	return *id.Properties.PrincipalID, err
}

func (kvc *KeyVaultClient) GetMSIResourceID(ctx context.Context, identityName string) (string, error) {
	id, err := kvc.identityClient.Get(ctx, kvc.Cred.ResourceGroup, identityName)
	if err != nil {
		return "", err
	}

	return *id.ID, err
}
