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

	"github.com/onsi/ginkgo/v2"
	v1 "k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	"sigs.k8s.io/blob-csi-driver/pkg/blob"
	"sigs.k8s.io/blob-csi-driver/test/e2e/driver"
	"sigs.k8s.io/blob-csi-driver/test/utils/azure"
)

// PreProvisionedKeyVaultTest will provision required PV(s), PVC(s) and Pod(s)
// Testing that the Pod(s) can be created successfully with provided Key Vault
// which is used to store storage account name and key
type PreProvisionedKeyVaultTest struct {
	CSIDriver driver.PreProvisionedVolumeTestDriver
	Pods      []PodDetails
	Driver    *blob.Driver
}

func (t *PreProvisionedKeyVaultTest) Run(client clientset.Interface, namespace *v1.Namespace) {
	keyVaultClient, err := azure.NewKeyVaultClient()
	framework.ExpectNoError(err)

	for _, pod := range t.Pods {
		for n, volume := range pod.Volumes {
			// In the method GetStorageAccountAndContainer, we can get an account key of the blob volume
			// by calling azure API, but not the sas token...
			accountName, accountKey, _, containerName, err := t.Driver.GetStorageAccountAndContainer(context.TODO(), volume.VolumeID, nil, nil)
			framework.ExpectNoError(err, fmt.Sprintf("Error GetStorageAccountAndContainer from volumeID(%s): %v", volume.VolumeID, err))

			ginkgo.By("creating KeyVault...")
			vault, err := keyVaultClient.CreateVault(context.TODO())
			framework.ExpectNoError(err)
			defer func() {
				err := keyVaultClient.CleanVault(context.TODO())
				framework.ExpectNoError(err)
			}()

			ginkgo.By("creating secret for storage account key...")
			accountKeySecret, err := keyVaultClient.CreateSecret(context.TODO(), accountName+"-key", accountKey)
			framework.ExpectNoError(err)

			pod.Volumes[n].Attrib["containerName"] = containerName
			pod.Volumes[n].Attrib["storageAccountName"] = accountName
			pod.Volumes[n].Attrib["keyVaultURL"] = *vault.Properties.VaultURI
			pod.Volumes[n].Attrib["keyVaultSecretName"] = *accountKeySecret.Name

			ginkgo.By("test storage account key...")
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
