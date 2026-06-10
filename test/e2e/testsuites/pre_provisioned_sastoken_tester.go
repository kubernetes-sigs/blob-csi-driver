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

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/sas"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/service"
	"github.com/onsi/ginkgo/v2"
	v1 "k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	"sigs.k8s.io/blob-csi-driver/pkg/blob"
	"sigs.k8s.io/blob-csi-driver/test/e2e/driver"
	"sigs.k8s.io/blob-csi-driver/test/utils/azure"
)

// PreProvisionedSASTokenTest will provision required PV(s), PVC(s) and Pod(s)
// Testing that the Pod(s) can be created successfully with provided Key Vault
// which is used to store storage SAS token
type PreProvisionedSASTokenTest struct {
	CSIDriver driver.PreProvisionedVolumeTestDriver
	Pods      []PodDetails
	Driver    *blob.Driver
}

func (t *PreProvisionedSASTokenTest) Run(ctx context.Context, client clientset.Interface, namespace *v1.Namespace) {
	keyVaultClient, err := azure.NewKeyVaultClient()
	framework.ExpectNoError(err)

	for _, pod := range t.Pods {
		for n, volume := range pod.Volumes {
			// In the method GetStorageAccountAndContainer, we can get an account key of the blob volume
			// by calling azure API, but not the sas token...
			accountName, accountKey, _, containerName, err := t.Driver.GetStorageAccountAndContainer(ctx, volume.VolumeID, nil, nil)
			framework.ExpectNoError(err, fmt.Sprintf("Error GetStorageAccountAndContainer from volumeID(%s): %v", volume.VolumeID, err))

			ginkgo.By("creating KeyVault...")
			vault, err := keyVaultClient.CreateVault(ctx)
			framework.ExpectNoError(err)
			defer func() {
				err := keyVaultClient.CleanVault(ctx)
				framework.ExpectNoError(err)
			}()

			ginkgo.By("generating SAS token...")
			sasToken := GenerateSASToken(accountName, accountKey)

			ginkgo.By("creating secret for SAS token...")
			accountSASSecret, err := keyVaultClient.CreateSecret(ctx, accountName+"-sas", sasToken)
			framework.ExpectNoError(err)

			pod.Volumes[n].Attrib["containerName"] = containerName
			pod.Volumes[n].Attrib["storageAccountName"] = accountName
			pod.Volumes[n].Attrib["keyVaultURL"] = *vault.Properties.VaultURI
			pod.Volumes[n].Attrib["keyVaultSecretName"] = *accountSASSecret.Name
			pod.Volumes[n].Attrib["azurestorageauthtype"] = "SAS"

			tpod, cleanup := pod.SetupWithPreProvisionedVolumes(ctx, client, namespace, t.CSIDriver)
			// defer must be called here for resources not get removed before using them
			for i := range cleanup {
				defer cleanup[i](ctx)
			}

			ginkgo.By("deploying the pod")
			tpod.Create(ctx)
			defer tpod.Cleanup(ctx)

			ginkgo.By("checking that the pods command exits with no error")
			tpod.WaitForSuccess(ctx)
		}
	}
}

func GenerateSASToken(accountName, accountKey string) string {
	credential, err := azblob.NewSharedKeyCredential(accountName, accountKey)
	framework.ExpectNoError(err)
	serviceClient, err := service.NewClientWithSharedKeyCredential(fmt.Sprintf("https://%s.blob.core.windows.net/", accountName), credential, nil)
	framework.ExpectNoError(err)
	sasURL, err := serviceClient.GetSASURL(
		sas.AccountResourceTypes{Object: true, Service: true, Container: true},
		sas.AccountPermissions{Read: true, List: true, Write: true, Delete: true, Add: true, Create: true, Update: true},
		time.Now().Add(10*time.Hour), nil)
	framework.ExpectNoError(err)
	u, err := url.Parse(sasURL)
	framework.ExpectNoError(err)
	sasToken := "?" + u.RawQuery
	return sasToken
}
