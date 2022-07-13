/*
Copyright 2020 The Kubernetes Authors.

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

package testsuites

import (
	"context"
	"fmt"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault"
	"github.com/onsi/ginkgo"
	v1 "k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	"sigs.k8s.io/blob-csi-driver/pkg/blob"
	"sigs.k8s.io/blob-csi-driver/test/e2e/driver"
	"sigs.k8s.io/blob-csi-driver/test/utils/credentials"
)

var (
	subscriptionID    string
	resourceGroupName string
	location          string
	vaultName         string
	TenantID          string
	ObjectID          string
)

// PreProvisionedKeyVaultTest will provision required PV(s), PVC(s) and Pod(s)
// Testing that the Pod(s) can be created successfully with provided Key Vault
// which is used to store storage account name and key(or sastoken)
type PreProvisionedKeyVaultTest struct {
	CSIDriver driver.PreProvisionedVolumeTestDriver
	Pods      []PodDetails
	Driver    *blob.Driver
}

func (t *PreProvisionedKeyVaultTest) Run(client clientset.Interface, namespace *v1.Namespace) {
	e2eCred, err := credentials.ParseAzureCredentialFile()
	framework.ExpectNoError(err, fmt.Sprintf("Error ParseAzureCredentialFile: %v", err))

	subscriptionID = e2eCred.SubscriptionID
	resourceGroupName = e2eCred.ResourceGroup
	location = e2eCred.Location
	TenantID = e2eCred.TenantID
	ObjectID = os.Getenv("AZURE_OBJECT_ID")
	framework.ExpectNotEqual(len(ObjectID), 0, "env AZURE_OBJECT_ID must be set")
	vaultName = "blobcsidriver-kv-test"

	for _, pod := range t.Pods {
		for n, volume := range pod.Volumes {
			accountName, accountKey, _, containerName, err := t.Driver.GetStorageAccountAndContainer(context.TODO(), volume.VolumeID, nil, nil)
			framework.ExpectNoError(err, fmt.Sprintf("Error GetStorageAccountAndContainer from volumeID(%s): %v", volume.VolumeID, err))

			azureCred, err := azidentity.NewDefaultAzureCredential(nil)
			framework.ExpectNoError(err)

			vault, err := createVault(context.TODO(), azureCred)
			framework.ExpectNoError(err)
			defer cleanVault(context.TODO(), azureCred)

			accountKeySecret, err := createSecret(context.TODO(), azureCred, accountName+"-key", accountKey)
			framework.ExpectNoError(err)

			// SAS token
			// accountSASSecret, err := createSecret(context.TODO(), azureCred, accountName+"-sas", accountSasToken)
			// framework.ExpectNoError(err)

			pod.Volumes[n].ContainerName = containerName
			pod.Volumes[n].StorageAccountname = accountName
			pod.Volumes[n].KeyVaultURL = *vault.Properties.VaultURI
			pod.Volumes[n].KeyVaultSecretName = *accountKeySecret.Name
			tpod, cleanup := pod.SetupWithPreProvisionedVolumes(client, namespace, t.CSIDriver)
			// defer must be called here for resources not get removed before using them
			for i := range cleanup {
				defer cleanup[i]()
			}

			ginkgo.By("deploying the pod")
			tpod.Create()
			defer tpod.Cleanup()
			ginkgo.By("checking that the pods command exits with no error")
			tpod.WaitForSuccess()
		}
	}
}

func createVault(ctx context.Context, cred azcore.TokenCredential) (*armkeyvault.Vault, error) {
	vaultsClient, err := armkeyvault.NewVaultsClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, err
	}

	pollerResp, err := vaultsClient.BeginCreateOrUpdate(
		ctx,
		resourceGroupName,
		vaultName,
		armkeyvault.VaultCreateOrUpdateParameters{
			Location: to.Ptr(location),
			Properties: &armkeyvault.VaultProperties{
				SKU: &armkeyvault.SKU{
					Family: to.Ptr(armkeyvault.SKUFamilyA),
					Name:   to.Ptr(armkeyvault.SKUNameStandard),
				},
				TenantID: to.Ptr(TenantID),
				AccessPolicies: []*armkeyvault.AccessPolicyEntry{
					{
						TenantID: to.Ptr(TenantID),
						ObjectID: to.Ptr(ObjectID),
						Permissions: &armkeyvault.Permissions{
							Secrets: []*armkeyvault.SecretPermissions{
								to.Ptr(armkeyvault.SecretPermissionsGet),
							},
						},
					},
				},
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

func cleanVault(ctx context.Context, cred azcore.TokenCredential) {
	err := deleteVault(ctx, cred)
	framework.ExpectNoError(err)

	err = purgeDeleted(ctx, cred)
	framework.ExpectNoError(err)
}

func deleteVault(ctx context.Context, cred azcore.TokenCredential) error {
	vaultsClient, err := armkeyvault.NewVaultsClient(subscriptionID, cred, nil)
	if err != nil {
		return err
	}

	_, err = vaultsClient.Delete(ctx, resourceGroupName, vaultName, nil)
	if err != nil {
		return err
	}
	return nil
}

func purgeDeleted(ctx context.Context, cred azcore.TokenCredential) error {
	vaultsClient, err := armkeyvault.NewVaultsClient(subscriptionID, cred, nil)
	if err != nil {
		return err
	}

	pollerResp, err := vaultsClient.BeginPurgeDeleted(ctx, vaultName, location, nil)
	if err != nil {
		return err
	}

	_, err = pollerResp.PollUntilDone(ctx, nil)
	if err != nil {
		return err
	}

	return nil
}

func createSecret(ctx context.Context, cred azcore.TokenCredential, secretName, secretValue string) (*armkeyvault.Secret, error) {
	secretsClient, err := armkeyvault.NewSecretsClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, err
	}

	secretResp, err := secretsClient.CreateOrUpdate(
		ctx,
		resourceGroupName,
		vaultName,
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
