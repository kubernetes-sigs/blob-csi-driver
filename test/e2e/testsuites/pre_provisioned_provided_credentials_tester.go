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

	"github.com/onsi/ginkgo"

	"sigs.k8s.io/blob-csi-driver/pkg/blob"
	"sigs.k8s.io/blob-csi-driver/test/e2e/driver"
	"sigs.k8s.io/blob-csi-driver/test/utils/azure"

	v1 "k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
)

// PreProvisionedProvidedCredentiasTest will provision required PV(s), PVC(s) and Pod(s)
// Testing that the Pod(s) can be created successfully with provided storage account name and key(or sastoken)
type PreProvisionedProvidedCredentiasTest struct {
	CSIDriver driver.PreProvisionedVolumeTestDriver
	Pods      []PodDetails
	Driver    *blob.Driver
}

func (t *PreProvisionedProvidedCredentiasTest) Run(client clientset.Interface, namespace *v1.Namespace) {
	kvClient, err := azure.NewKeyVaultClient()
	framework.ExpectNoError(err)

	authClient, err := azure.NewAuthorizationClient()
	framework.ExpectNoError(err)

	for _, pod := range t.Pods {
		for n, volume := range pod.Volumes {
			accountName, accountKey, _, _, err := t.Driver.GetStorageAccountAndContainer(context.Background(), volume.VolumeID, nil, nil)
			framework.ExpectNoError(err, fmt.Sprintf("Error GetStorageAccountAndContainer from volumeID(%s): %v", volume.VolumeID, err))
			var secretData map[string]string

			var run = func() {
				tsecret := NewTestSecret(client, namespace, volume.NodeStageSecretRef, secretData)
				tsecret.Create()
				defer tsecret.Cleanup()

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

			// test for storage account key
			ginkgo.By("Run for storage account key")
			secretData = map[string]string{
				"azurestorageaccountname": accountName,
				"azurestorageaccountkey":  accountKey,
			}
			run()

			// test for storage account SAS token
			ginkgo.By("Run for storage account SAS token")
			pod.Volumes[n].Attrib = map[string]string{
				"azurestorageauthtype": "SAS",
			}
			sasToken := GenerateSASToken(accountName, accountKey)
			secretData = map[string]string{
				"azurestorageaccountname":     accountName,
				"azurestorageaccountsastoken": sasToken,
			}
			run()

			// test for service principal
			ginkgo.By("Run for service principal")
			pod.Volumes[n].Attrib = map[string]string{
				"azurestorageauthtype":    "SPN",
				"azurestoragespnclientid": kvClient.Cred.AADClientID,
				"azurestoragespntenantid": kvClient.Cred.TenantID,
			}
			secretData = map[string]string{
				"azurestorageaccountname":     accountName,
				"azurestoragespnclientsecret": kvClient.Cred.AADClientSecret,
			}

			objectID, err := kvClient.GetServicePrincipalObjectID(context.TODO(), kvClient.Cred.AADClientID)
			framework.ExpectNoError(err, fmt.Sprintf("Error GetServicePrincipalObjectID from clientID(%s): %v", kvClient.Cred.AADClientID, err))

			resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Storage/storageAccounts/%s", kvClient.Cred.SubscriptionID, kvClient.Cred.ResourceGroup, accountName)

			ginkgo.By(fmt.Sprintf("assign Storage Blob Data Contributor role to the service principal, objectID:%s", objectID))
			roleDef, err := authClient.GetRoleDefinition(context.TODO(), resourceID, "Storage Blob Data Contributor")
			framework.ExpectNoError(err, fmt.Sprintf("Error GetRoleDefinition from resourceID(%s): %v", resourceID, err))

			roleDefID := *roleDef.ID
			_, err = authClient.AssignRole(context.TODO(), resourceID, objectID, roleDefID)
			framework.ExpectNoError(err, fmt.Sprintf("Error AssignRole (roleDefID(%s)) to objectID(%s) to access resource (resourceID(%s)), error: %v", roleDefID, objectID, resourceID, err))

			run()

			// test for managed identity
			// e2e-vmss test job uses msi blobfuse-csi-driver-e2e-test-id, other jobs use service principal
			objectID, err = kvClient.GetMSIObjectID(context.TODO(), "blobfuse-csi-driver-e2e-test-id")
			if err != nil {
				return
			}

			ginkgo.By("Run for managed identity")
			pod.Volumes[n].Attrib = map[string]string{
				"azurestorageauthtype":         "MSI",
				"azurestorageidentityobjectid": objectID,
			}

			secretData = map[string]string{
				"azurestorageaccountname": accountName,
			}
			ginkgo.By(fmt.Sprintf("assign Storage Blob Data Contributor role to the managed identity, objectID:%s", objectID))
			_, err = authClient.AssignRole(context.TODO(), resourceID, objectID, roleDefID)
			framework.ExpectNoError(err, fmt.Sprintf("Error AssignRole (roleDefID(%s)) to objectID(%s) to access resource (resourceID(%s)), error: %v", roleDefID, objectID, resourceID, err))

			run()
		}
	}
}
