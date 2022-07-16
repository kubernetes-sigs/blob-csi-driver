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
	"net/url"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/services/graphrbac/1.6/graphrbac"
	"github.com/Azure/azure-sdk-for-go/services/msi/mgmt/2018-11-30/msi"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/onsi/ginkgo"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apiserver/pkg/storage/names"
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
	cloud             string
	clientID          string
	clientSecret      string
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
	cloud = e2eCred.Cloud
	clientID = e2eCred.AADClientID
	clientSecret = e2eCred.AADClientSecret
	vaultName = names.SimpleNameGenerator.GenerateName("blob-csi-test-kv-")

	for _, pod := range t.Pods {
		for n, volume := range pod.Volumes {
			// In the method GetStorageAccountAndContainer, we can get an account key of the blob volume
			// by calling azure API, but not the sas token...
			accountName, accountKey, _, containerName, err := t.Driver.GetStorageAccountAndContainer(context.TODO(), volume.VolumeID, nil, nil)
			framework.ExpectNoError(err, fmt.Sprintf("Error GetStorageAccountAndContainer from volumeID(%s): %v", volume.VolumeID, err))

			azureCred, err := azidentity.NewDefaultAzureCredential(nil)
			framework.ExpectNoError(err)

			ginkgo.By("creating KeyVault...")
			vault, err := createVault(context.TODO(), azureCred)
			framework.ExpectNoError(err)
			defer cleanVault(context.TODO(), azureCred)

			ginkgo.By("creating secret for storage account key...")
			accountKeySecret, err := createSecret(context.TODO(), azureCred, accountName+"-key", accountKey)
			framework.ExpectNoError(err)

			pod.Volumes[n].ContainerName = containerName
			pod.Volumes[n].StorageAccountname = accountName
			pod.Volumes[n].KeyVaultURL = *vault.Properties.VaultURI
			pod.Volumes[n].KeyVaultSecretName = *accountKeySecret.Name
			// test for Account key
			ginkgo.By("test storage account key...")
			run(pod, client, namespace, t.CSIDriver)

			ginkgo.By("generate SAS token...")
			sasToken := generateSASToken(accountName, accountKey)

			ginkgo.By("creating secret for SAS token...")
			accountSASSecret, err := createSecret(context.TODO(), azureCred, accountName+"-sas", sasToken)
			framework.ExpectNoError(err)

			pod.Volumes[n].KeyVaultSecretName = *accountSASSecret.Name
			// TODO: test for SAS token
			// ginkgo.By("test SAS token...")
			// run(pod, client, namespace, t.CSIDriver)
		}
	}
}

func run(pod PodDetails, client clientset.Interface, namespace *v1.Namespace, csidriver driver.PreProvisionedVolumeTestDriver) {
	tpod, cleanup := pod.SetupWithPreProvisionedVolumes(client, namespace, csidriver)
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

func generateSASToken(accountName, accountKey string) string {
	credential, err := azblob.NewSharedKeyCredential(accountName, accountKey)
	framework.ExpectNoError(err)
	serviceClient, err := azblob.NewServiceClientWithSharedKey(fmt.Sprintf("https://%s.blob.core.windows.net/", accountName), credential, nil)
	framework.ExpectNoError(err)
	sasURL, err := serviceClient.GetSASURL(
		azblob.AccountSASResourceTypes{Object: true, Service: true, Container: true},
		azblob.AccountSASPermissions{Read: true, List: true, Write: true, Delete: true, Add: true, Create: true, Update: true},
		time.Now(), time.Now().Add(12*time.Hour))
	framework.ExpectNoError(err)
	ginkgo.By("sas URL: " + sasURL)
	u, err := url.Parse(sasURL)
	framework.ExpectNoError(err)
	queryUnescape, err := url.QueryUnescape(u.RawQuery)
	framework.ExpectNoError(err)
	sasToken := "?" + queryUnescape
	ginkgo.By("sas Token: " + sasToken)
	return sasToken
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
				TenantID:       to.Ptr(TenantID),
				AccessPolicies: getAccessPolicy(ctx),
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

func getAccessPolicy(ctx context.Context) []*armkeyvault.AccessPolicyEntry {
	accessPolicyEntry := []*armkeyvault.AccessPolicyEntry{}

	// vault secret permission for upstream e2e test, which uses application service principal
	clientObjectID, err := getServicePrincipalObjectID(ctx, clientID)
	if err == nil {
		ginkgo.By("client object ID: " + clientObjectID)
		accessPolicyEntry = append(accessPolicyEntry, &armkeyvault.AccessPolicyEntry{
			TenantID: to.Ptr(TenantID),
			ObjectID: to.Ptr(clientObjectID),
			Permissions: &armkeyvault.Permissions{
				Secrets: []*armkeyvault.SecretPermissions{
					to.Ptr(armkeyvault.SecretPermissionsGet),
				},
			},
		})
	}

	// vault secret permission for upstream e2e-vmss test, which uses msi blobfuse-csi-driver-e2e-test-id
	msiObjectID, err := getMSIObjectID(ctx, "blobfuse-csi-driver-e2e-test-id")
	if err == nil {
		ginkgo.By("MSI object ID: " + msiObjectID)
		accessPolicyEntry = append(accessPolicyEntry, &armkeyvault.AccessPolicyEntry{
			TenantID: to.Ptr(TenantID),
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

func getServicePrincipalObjectID(ctx context.Context, clientID string) (string, error) {
	spClient, err := getServicePrincipalsClient()
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

func getServicePrincipalsClient() (*graphrbac.ServicePrincipalsClient, error) {
	spClient := graphrbac.NewServicePrincipalsClient(TenantID)

	env, err := azure.EnvironmentFromName(cloud)
	if err != nil {
		return nil, err
	}

	oauthConfig, err := adal.NewOAuthConfig(env.ActiveDirectoryEndpoint, TenantID)
	if err != nil {
		return nil, err
	}

	token, err := adal.NewServicePrincipalToken(*oauthConfig, clientID, clientSecret, env.GraphEndpoint)
	if err != nil {
		return nil, err
	}

	authorizer := autorest.NewBearerAuthorizer(token)

	spClient.Authorizer = authorizer

	return &spClient, nil
}

func getMSIUserAssignedIDClient() (*msi.UserAssignedIdentitiesClient, error) {
	msiClient := msi.NewUserAssignedIdentitiesClient(subscriptionID)

	env, err := azure.EnvironmentFromName(cloud)
	if err != nil {
		return nil, err
	}

	oauthConfig, err := adal.NewOAuthConfig(env.ActiveDirectoryEndpoint, TenantID)
	if err != nil {
		return nil, err
	}

	token, err := adal.NewServicePrincipalToken(*oauthConfig, clientID, clientSecret, env.ResourceManagerEndpoint)
	if err != nil {
		return nil, err
	}

	authorizer := autorest.NewBearerAuthorizer(token)

	msiClient.Authorizer = authorizer

	return &msiClient, nil
}

func getMSIObjectID(ctx context.Context, identityName string) (string, error) {
	msiClient, err := getMSIUserAssignedIDClient()
	if err != nil {
		return "", err
	}

	id, err := msiClient.Get(ctx, resourceGroupName, identityName)

	return id.UserAssignedIdentityProperties.PrincipalID.String(), err
}
