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

package e2e

import (
	"fmt"
	"time"

	"sigs.k8s.io/blob-csi-driver/test/e2e/driver"
	"sigs.k8s.io/blob-csi-driver/test/e2e/testsuites"

	"github.com/onsi/ginkgo/v2"
	v1 "k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	admissionapi "k8s.io/pod-security-admission/api"
)

var _ = ginkgo.Describe("[blob-csi-e2e] Dynamic Provisioning", func() {
	f := framework.NewDefaultFramework("blob")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

	var (
		cs         clientset.Interface
		ns         *v1.Namespace
		testDriver driver.PVTestDriver
	)

	ginkgo.BeforeEach(func(ctx ginkgo.SpecContext) {
		checkPodsRestart := testCmd{
			command:  "sh",
			args:     []string{"test/utils/check_driver_pods_restart.sh"},
			startLog: "Check driver pods if restarts ...",
			endLog:   "Check successfully",
		}
		execTestCmd([]testCmd{checkPodsRestart})

		cs = f.ClientSet
		ns = f.Namespace
		testDriver = driver.InitBlobCSIDriver()
	})

	ginkgo.It("should create a volume on demand without saving storage account key", func(ctx ginkgo.SpecContext) {
		pods := []testsuites.PodDetails{
			{
				Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
				Volumes: []testsuites.VolumeDetails{
					{
						ClaimSize: "10Gi",
						MountOptions: []string{
							"-o allow_other",
							"--file-cache-timeout-in-seconds=120",
							"--cancel-list-on-mount-seconds=0",
							"-o uid=0",
							"-o gid=0",
						},
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedCmdVolumeTest{
			CSIDriver: testDriver,
			Pods:      pods,
			StorageClassParameters: map[string]string{
				"skuName":         "Standard_GRS",
				"secretNamespace": "default",
				// make sure this is the first test case due to storeAccountKey is set as false
				"storeAccountKey":        "false",
				"getLatestAccountKey":    "true",
				"requireInfraEncryption": "true",
				"accessTier":             "Hot",
			},
		}
		test.Run(ctx, cs, ns)
	})

	ginkgo.It("should create a volume on demand with mount options", func(ctx ginkgo.SpecContext) {
		pods := []testsuites.PodDetails{
			{
				Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
				Volumes: []testsuites.VolumeDetails{
					{
						ClaimSize: "10Gi",
						MountOptions: []string{
							"-o allow_other",
							"--file-cache-timeout-in-seconds=120",
							"--cancel-list-on-mount-seconds=0",
							"-o uid=0",
							"-o gid=0",
						},
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedCmdVolumeTest{
			CSIDriver: testDriver,
			Pods:      pods,
			StorageClassParameters: map[string]string{
				"skuName":             "Standard_LRS",
				"secretNamespace":     "default",
				"containerNamePrefix": "nameprefix",
				"accessTier":          "Cool",
			},
		}
		test.Run(ctx, cs, ns)
	})

	ginkgo.It("should create a volume on demand with specified secretName", func(ctx ginkgo.SpecContext) {
		pods := []testsuites.PodDetails{
			{
				Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
				Volumes: []testsuites.VolumeDetails{
					{
						ClaimSize: "10Gi",
						MountOptions: []string{
							"-o allow_other",
							"--file-cache-timeout-in-seconds=120",
							"--cancel-list-on-mount-seconds=0",
						},
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
			},
		}
		scParameters := map[string]string{
			"skuName":         "Standard_LRS",
			"secretNamespace": "kube-system",
		}
		scParameters["secretName"] = fmt.Sprintf("secret-%d", time.Now().Unix())
		test := testsuites.DynamicallyProvisionedCmdVolumeTest{
			CSIDriver:              testDriver,
			Pods:                   pods,
			StorageClassParameters: scParameters,
		}
		test.Run(ctx, cs, ns)
	})

	ginkgo.It("should create a deployment object, write and read to it, delete the pod and write and read to it again", func(ctx ginkgo.SpecContext) {
		pod := testsuites.PodDetails{
			Cmd: "echo 'hello world' >> /mnt/test-1/data && while true; do sleep 1; done",
			Volumes: []testsuites.VolumeDetails{
				{
					FSType:    "ext3",
					ClaimSize: "10Gi",
					MountOptions: []string{
						"-o allow_other",
						"--file-cache-timeout-in-seconds=120",
						"--cancel-list-on-mount-seconds=60",
					},
					VolumeMount: testsuites.VolumeMountDetails{
						NameGenerate:      "test-volume-",
						MountPathGenerate: "/mnt/test-",
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedDeletePodTest{
			CSIDriver: testDriver,
			Pod:       pod,
			PodCheck: &testsuites.PodExecCheck{
				Cmd:            []string{"cat", "/mnt/test-1/data"},
				ExpectedString: "hello world\nhello world\n", // pod will be restarted so expect to see 2 instances of string
			},
			StorageClassParameters: map[string]string{
				"skuName":               "Premium_LRS",
				"isHnsEnabled":          "true",
				"allowBlobPublicAccess": "false",
				"accessTier":            "Premium",
				"useDataPlaneAPI":       "true",
				"containerName":         "container-${pvc.metadata.name}",
			},
		}
		test.Run(ctx, cs, ns)
	})

	// Track issue https://github.com/kubernetes/kubernetes/issues/70505
	ginkgo.It("should create a volume on demand and mount it as readOnly in a pod", func(ctx ginkgo.SpecContext) {
		pods := []testsuites.PodDetails{
			{
				Cmd: "touch /mnt/test-1/data",
				Volumes: []testsuites.VolumeDetails{
					{
						ClaimSize: "10Gi",
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
							ReadOnly:          true,
						},
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedReadOnlyVolumeTest{
			CSIDriver:              testDriver,
			Pods:                   pods,
			StorageClassParameters: map[string]string{"skuName": "Standard_GRS"},
		}
		if isAzureStackCloud {
			test.StorageClassParameters = map[string]string{"skuName": "Standard_LRS"}
		}
		test.Run(ctx, cs, ns)
	})

	ginkgo.It("should create a blobfuse volume on demand and mount it as readOnly when volume access mode is readonly", func(ctx ginkgo.SpecContext) {
		pods := []testsuites.PodDetails{
			{
				Cmd: "touch /mnt/test-1/data",
				Volumes: []testsuites.VolumeDetails{
					{
						ClaimSize: "10Gi",
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
						AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadOnlyMany},
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedReadOnlyVolumeTest{
			CSIDriver:              testDriver,
			Pods:                   pods,
			StorageClassParameters: map[string]string{"skuName": "Standard_GRS"},
		}
		if isAzureStackCloud {
			test.StorageClassParameters = map[string]string{"skuName": "Standard_LRS"}
		}
		test.Run(ctx, cs, ns)
	})

	ginkgo.It("should create a nfs volume on demand and mount it as readOnly when volume access mode is readonly", func(ctx ginkgo.SpecContext) {
		if isAzureStackCloud {
			ginkgo.Skip("test case is not available for Azure Stack")
		}
		pods := []testsuites.PodDetails{
			{
				Cmd: "touch /mnt/test-1/data",
				Volumes: []testsuites.VolumeDetails{
					{
						ClaimSize: "10Gi",
						MountOptions: []string{
							"nconnect=8",
						},
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
						AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadOnlyMany},
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedReadOnlyVolumeTest{
			CSIDriver: testDriver,
			Pods:      pods,
			StorageClassParameters: map[string]string{
				"skuName":  "Premium_LRS",
				"protocol": "nfs",
			},
		}
		test.Run(ctx, cs, ns)
	})

	ginkgo.It("should create multiple PV objects, bind to PVCs and attach all to different pods on the same node", func(ctx ginkgo.SpecContext) {
		pods := []testsuites.PodDetails{
			{
				Cmd: "while true; do echo $(date -u) >> /mnt/test-1/data; sleep 1; done",
				Volumes: []testsuites.VolumeDetails{
					{
						FSType:    "ext3",
						ClaimSize: "10Gi",
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
			},
			{
				Cmd: "while true; do echo $(date -u) >> /mnt/test-1/data; sleep 1; done",
				Volumes: []testsuites.VolumeDetails{
					{
						FSType:    "ext4",
						ClaimSize: "10Gi",
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedCollocatedPodTest{
			CSIDriver:    testDriver,
			Pods:         pods,
			ColocatePods: true,
			StorageClassParameters: map[string]string{
				"skuName":               "Standard_RAGRS",
				"allowBlobPublicAccess": "false",
				"containerName":         "container-${pvc.metadata.namespace}",
			},
		}
		if isAzureStackCloud {
			test.StorageClassParameters = map[string]string{"skuName": "Standard_LRS"}
		}
		test.Run(ctx, cs, ns)
	})

	ginkgo.It(fmt.Sprintf("should delete PV with reclaimPolicy %q", v1.PersistentVolumeReclaimDelete), func(ctx ginkgo.SpecContext) {
		reclaimPolicy := v1.PersistentVolumeReclaimDelete
		volumes := []testsuites.VolumeDetails{
			{
				FSType:        "ext4",
				ClaimSize:     "10Gi",
				ReclaimPolicy: &reclaimPolicy,
			},
		}
		test := testsuites.DynamicallyProvisionedReclaimPolicyTest{
			CSIDriver:              testDriver,
			Volumes:                volumes,
			StorageClassParameters: map[string]string{"skuName": "Standard_LRS"},
		}
		test.Run(ctx, cs, ns)
	})

	ginkgo.It(fmt.Sprintf("[env] should retain PV with reclaimPolicy %q", v1.PersistentVolumeReclaimRetain), func(ctx ginkgo.SpecContext) {
		reclaimPolicy := v1.PersistentVolumeReclaimRetain
		volumes := []testsuites.VolumeDetails{
			{
				FSType:        "ext4",
				ClaimSize:     "10Gi",
				ReclaimPolicy: &reclaimPolicy,
			},
		}
		test := testsuites.DynamicallyProvisionedReclaimPolicyTest{
			CSIDriver: testDriver,
			Volumes:   volumes,
			Driver:    blobDriver,
			StorageClassParameters: map[string]string{
				"skuName":               "Standard_GRS",
				"allowBlobPublicAccess": "false",
			},
		}
		if isAzureStackCloud {
			test.StorageClassParameters = map[string]string{"skuName": "Standard_LRS"}
		}
		test.Run(ctx, cs, ns)
	})

	ginkgo.It("should create a pod with multiple volumes", func(ctx ginkgo.SpecContext) {
		volumes := []testsuites.VolumeDetails{}
		for i := 1; i <= 6; i++ {
			volume := testsuites.VolumeDetails{
				ClaimSize: "10Gi",
				VolumeMount: testsuites.VolumeMountDetails{
					NameGenerate:      "test-volume-",
					MountPathGenerate: "/mnt/test-",
				},
			}
			volumes = append(volumes, volume)
		}

		pods := []testsuites.PodDetails{
			{
				Cmd:     "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
				Volumes: volumes,
			},
		}
		test := testsuites.DynamicallyProvisionedPodWithMultiplePVsTest{
			CSIDriver: testDriver,
			Pods:      pods,
		}
		test.Run(ctx, cs, ns)
	})

	ginkgo.It("should receive FailedMount event with invalid mount options", func(ctx ginkgo.SpecContext) {
		pods := []testsuites.PodDetails{
			{
				Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
				Volumes: []testsuites.VolumeDetails{
					{
						ClaimSize: "10Gi",
						MountOptions: []string{
							"invalid",
							"mount",
							"options",
						},
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedInvalidMountOptions{
			CSIDriver: testDriver,
			Pods:      pods,
			StorageClassParameters: map[string]string{
				"skuName":               "Standard_LRS",
				"allowBlobPublicAccess": "true",
			},
		}
		test.Run(ctx, cs, ns)
	})

	ginkgo.It("should create a volume on demand (Bring Your Own Key)", func(ctx ginkgo.SpecContext) {
		// create a volume
		volName := fmt.Sprintf("byok-%d", ginkgo.GinkgoParallelProcess())
		resp, err := blobDriver.CreateVolume(ctx, makeCreateVolumeReq(volName, ns.Name))
		framework.ExpectNoError(err, "create volume error")
		volumeID := resp.Volume.VolumeId
		// get accountname and key
		accountName, accountKey, _, _, err := blobDriver.GetStorageAccountAndContainer(ctx, volumeID, nil, nil)
		framework.ExpectNoError(err, fmt.Sprintf("Error GetStorageAccountAndContainer from volumeID(%s): %v", volumeID, err))
		// create secret
		secretName := "byok-secret"
		secretData := map[string]string{
			"azurestorageaccountname": accountName,
			"azurestorageaccountkey":  accountKey,
		}
		tsecret := testsuites.NewTestSecret(cs, ns, secretName, secretData)
		tsecret.Create(ctx)
		defer tsecret.Cleanup(ctx)

		var bringKeyStorageClassParameters = map[string]string{
			"csi.storage.k8s.io/provisioner-secret-name":      secretName,
			"csi.storage.k8s.io/node-stage-secret-name":       secretName,
			"csi.storage.k8s.io/provisioner-secret-namespace": ns.Name,
			"csi.storage.k8s.io/node-stage-secret-namespace":  ns.Name,
		}

		pods := []testsuites.PodDetails{
			{
				Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
				Volumes: []testsuites.VolumeDetails{
					{
						ClaimSize: "10Gi",
						MountOptions: []string{
							"-o allow_other",
							"--file-cache-timeout-in-seconds=120",
						},
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedCmdVolumeTest{
			CSIDriver:              testDriver,
			Pods:                   pods,
			StorageClassParameters: bringKeyStorageClassParameters,
		}
		test.Run(ctx, cs, ns)
	})

	ginkgo.It("should create a volume on demand and resize it [blob.csi.azure.com]", func(ctx ginkgo.SpecContext) {
		pods := []testsuites.PodDetails{
			{
				Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
				Volumes: []testsuites.VolumeDetails{
					{
						ClaimSize: "10Gi",
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedResizeVolumeTest{
			CSIDriver: testDriver,
			Pods:      pods,
			StorageClassParameters: map[string]string{
				"skuName":   "Standard_LRS",
				"matchTags": "true",
			},
		}
		test.Run(ctx, cs, ns)
	})

	ginkgo.It("should create an CSI inline volume [blob.csi.azure.com]", func(ctx ginkgo.SpecContext) {
		// create a volume
		containerName := "csi-inline-blobfuse-volume"
		resp, err := blobDriver.CreateVolume(ctx, makeCreateVolumeReq(containerName, ns.Name))
		framework.ExpectNoError(err, "create volume error")
		volumeID := resp.Volume.VolumeId
		// get accountname and key
		accountName, accountKey, _, _, err := blobDriver.GetStorageAccountAndContainer(ctx, volumeID, nil, nil)
		framework.ExpectNoError(err, fmt.Sprintf("Error GetStorageAccountAndContainer from volumeID(%s): %v", volumeID, err))
		// create secret
		secretName := "csi-inline-blobfuse-volume-secret"
		secretData := map[string]string{
			"azurestorageaccountname": accountName,
			"azurestorageaccountkey":  accountKey,
		}
		tsecret := testsuites.NewTestSecret(cs, ns, secretName, secretData)
		tsecret.Create(ctx)
		defer tsecret.Cleanup(ctx)

		pods := []testsuites.PodDetails{
			{
				Volumes: []testsuites.VolumeDetails{
					{
						ClaimSize: "100Gi",
						MountOptions: []string{
							"-o allow_other",
							"--file-cache-timeout-in-seconds=120",
						},
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
			},
		}

		test := testsuites.DynamicallyProvisionedInlineVolumeTest{
			CSIDriver:     testDriver,
			Pods:          pods,
			SecretName:    secretName,
			ContainerName: containerName,
			ReadOnly:      false,
		}
		test.Run(ctx, cs, ns)
	})

	ginkgo.It("should create a NFSv3 volume on demand with mount options [nfs]", func(ctx ginkgo.SpecContext) {
		if isAzureStackCloud {
			ginkgo.Skip("test case is not available for Azure Stack")
		}
		pods := []testsuites.PodDetails{
			{
				Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
				Volumes: []testsuites.VolumeDetails{
					{
						ClaimSize: "10Gi",
						MountOptions: []string{
							"nconnect=8",
						},
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedCmdVolumeTest{
			CSIDriver: testDriver,
			Pods:      pods,
			StorageClassParameters: map[string]string{
				"skuName":             "Premium_LRS",
				"protocol":            "nfs",
				"mountPermissions":    "0755",
				"fsGroupChangePolicy": "Always",
			},
		}
		test.Run(ctx, cs, ns)
	})

	ginkgo.It("enforce with nfs mount [nfs]", func(ctx ginkgo.SpecContext) {
		if isAzureStackCloud {
			ginkgo.Skip("test case is not available for Azure Stack")
		}
		pods := []testsuites.PodDetails{
			{
				Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
				Volumes: []testsuites.VolumeDetails{
					{
						ClaimSize: "10Gi",
						MountOptions: []string{
							"nconnect=8",
						},
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedCmdVolumeTest{
			CSIDriver: testDriver,
			Pods:      pods,
			StorageClassParameters: map[string]string{
				"protocol": "nfsv3",
			},
		}
		test.Run(ctx, cs, ns)
	})

	ginkgo.It("should create a NFSv3 volume on demand with zero mountPermissions [nfs]", func(ctx ginkgo.SpecContext) {
		if isAzureStackCloud {
			ginkgo.Skip("test case is not available for Azure Stack")
		}
		pods := []testsuites.PodDetails{
			{
				Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
				Volumes: []testsuites.VolumeDetails{
					{
						ClaimSize: "10Gi",
						MountOptions: []string{
							"nconnect=8",
						},
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedCmdVolumeTest{
			CSIDriver: testDriver,
			Pods:      pods,
			StorageClassParameters: map[string]string{
				"skuName":              "Premium_LRS",
				"protocol":             "nfs",
				"mountPermissions":     "0",
				"allowSharedKeyAccess": "false",
			},
		}
		test.Run(ctx, cs, ns)
	})

	ginkgo.It("should create a blobfuse2 volume on demand with mount options [fuse2]", func(ctx ginkgo.SpecContext) {
		if isAzureStackCloud {
			ginkgo.Skip("test case is not available for Azure Stack")
		}
		pods := []testsuites.PodDetails{
			{
				Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
				Volumes: []testsuites.VolumeDetails{
					{
						ClaimSize: "10Gi",
						MountOptions: []string{
							"-o allow_other",
							"--virtual-directory=true", // blobfuse2 mount options
						},
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedCmdVolumeTest{
			CSIDriver: testDriver,
			Pods:      pods,
			StorageClassParameters: map[string]string{
				"skuName":  "Standard_LRS",
				"protocol": "fuse2",
			},
		}
		test.Run(ctx, cs, ns)
	})

	ginkgo.It("should create a private endpoint volume on demand", ginkgo.Serial, func(ctx ginkgo.SpecContext) {
		if isAzureStackCloud {
			ginkgo.Skip("test case is not available for Azure Stack")
		}
		pods := []testsuites.PodDetails{
			{
				Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
				Volumes: []testsuites.VolumeDetails{
					{
						ClaimSize: "10Gi",
						MountOptions: []string{
							"-o allow_other",
							"--file-cache-timeout-in-seconds=120",
							"--cancel-list-on-mount-seconds=0",
						},
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedCmdVolumeTest{
			CSIDriver: testDriver,
			Pods:      pods,
			StorageClassParameters: map[string]string{
				"skuName":             "Standard_LRS",
				"networkEndpointType": "privateEndpoint",
			},
		}
		test.Run(ctx, cs, ns)
	})

	ginkgo.It("should create a private endpoint volume on demand with protocol [fuse2]", ginkgo.Serial, func(ctx ginkgo.SpecContext) {
		if isAzureStackCloud {
			ginkgo.Skip("test case is not available for Azure Stack")
		}
		pods := []testsuites.PodDetails{
			{
				Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
				Volumes: []testsuites.VolumeDetails{
					{
						ClaimSize: "10Gi",
						MountOptions: []string{
							"-o allow_other",
							"--virtual-directory=true", // blobfuse2 mount options
						},
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedCmdVolumeTest{
			CSIDriver: testDriver,
			Pods:      pods,
			StorageClassParameters: map[string]string{
				"skuName":             "Standard_LRS",
				"protocol":            "fuse2",
				"networkEndpointType": "privateEndpoint",
			},
		}
		test.Run(ctx, cs, ns)
	})

	ginkgo.It("should create a private endpoint volume on demand with protocol [nfs]", ginkgo.Serial, func(ctx ginkgo.SpecContext) {
		if isAzureStackCloud {
			ginkgo.Skip("test case is not available for Azure Stack")
		}
		pods := []testsuites.PodDetails{
			{
				Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
				Volumes: []testsuites.VolumeDetails{
					{
						ClaimSize: "10Gi",
						MountOptions: []string{
							"nconnect=8",
						},
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedCmdVolumeTest{
			CSIDriver: testDriver,
			Pods:      pods,
			StorageClassParameters: map[string]string{
				"skuName":             "Premium_LRS",
				"protocol":            "nfs",
				"mountPermissions":    "0755",
				"networkEndpointType": "privateEndpoint",
			},
		}
		test.Run(ctx, cs, ns)
	})

	ginkgo.It("should be able to unmount blobfuse volume if volume is already deleted [blob.csi.azure.com]", func(ctx ginkgo.SpecContext) {
		pod := testsuites.PodDetails{
			Cmd: "echo 'hello world' >> /mnt/test-1/data && while true; do sleep 1; done",
			Volumes: []testsuites.VolumeDetails{
				{
					ClaimSize: "10Gi",
					MountOptions: []string{
						"-o allow_other",
						"--file-cache-timeout-in-seconds=120",
						"--cancel-list-on-mount-seconds=0",
					},
					VolumeMount: testsuites.VolumeMountDetails{
						NameGenerate:      "test-volume-",
						MountPathGenerate: "/mnt/test-",
					},
				},
			},
		}

		test := testsuites.DynamicallyProvisionedVolumeUnmountTest{
			CSIDriver: testDriver,
			Driver:    blobDriver,
			Pod:       pod,
			PodCheck: &testsuites.PodExecCheck{
				Cmd:            []string{"cat", "/mnt/test-1/data"},
				ExpectedString: "hello world\n",
			},
			StorageClassParameters: map[string]string{
				"skuName":  "Standard_LRS",
				"protocol": "fuse",
			},
		}
		test.Run(ctx, cs, ns)
	})

	ginkgo.It("should be able to unmount blobfuse2 volume if volume is already deleted [blob.csi.azure.com]", func(ctx ginkgo.SpecContext) {
		pod := testsuites.PodDetails{
			Cmd: "echo 'hello world' >> /mnt/test-1/data && while true; do sleep 1; done",
			Volumes: []testsuites.VolumeDetails{
				{
					ClaimSize: "10Gi",
					MountOptions: []string{
						"-o allow_other",
						"--virtual-directory=true", // blobfuse2 mount options
					},
					VolumeMount: testsuites.VolumeMountDetails{
						NameGenerate:      "test-volume-",
						MountPathGenerate: "/mnt/test-",
					},
				},
			},
		}

		test := testsuites.DynamicallyProvisionedVolumeUnmountTest{
			CSIDriver: testDriver,
			Driver:    blobDriver,
			Pod:       pod,
			PodCheck: &testsuites.PodExecCheck{
				Cmd:            []string{"cat", "/mnt/test-1/data"},
				ExpectedString: "hello world\n",
			},
			StorageClassParameters: map[string]string{
				"skuName":  "Standard_LRS",
				"protocol": "fuse2",
			},
		}
		test.Run(ctx, cs, ns)
	})

	ginkgo.It("should be able to unmount NFS volume if volume is already deleted [blob.csi.azure.com]", func(ctx ginkgo.SpecContext) {
		pod := testsuites.PodDetails{
			Cmd: "echo 'hello world' >> /mnt/test-1/data && while true; do sleep 1; done",
			Volumes: []testsuites.VolumeDetails{
				{
					ClaimSize: "10Gi",
					MountOptions: []string{
						"nconnect=8",
					},
					VolumeMount: testsuites.VolumeMountDetails{
						NameGenerate:      "test-volume-",
						MountPathGenerate: "/mnt/test-",
					},
				},
			},
		}

		test := testsuites.DynamicallyProvisionedVolumeUnmountTest{
			CSIDriver: testDriver,
			Driver:    blobDriver,
			Pod:       pod,
			PodCheck: &testsuites.PodExecCheck{
				Cmd:            []string{"cat", "/mnt/test-1/data"},
				ExpectedString: "hello world\n",
			},
			StorageClassParameters: map[string]string{
				"skuName":          "Premium_LRS",
				"protocol":         "nfs",
				"mountPermissions": "0755",
			},
		}
		test.Run(ctx, cs, ns)
	})

	ginkgo.It("[blob.csi.azure.com] verify examples", ginkgo.Label("flaky"), func(ctx ginkgo.SpecContext) {
		createExampleDeployment := testCmd{
			command:  "bash",
			args:     []string{"hack/verify-examples.sh"},
			startLog: "create example deployments",
			endLog:   "example deployments created",
		}
		execTestCmd([]testCmd{createExampleDeployment})
	})

	ginkgo.It("volume mount is still valid after driver restart [blob.csi.azure.com]", ginkgo.Serial, func(ctx ginkgo.SpecContext) {
		pod := testsuites.PodDetails{
			Cmd: "echo 'hello world' >> /mnt/test-1/data && while true; do sleep 3600; done",
			Volumes: []testsuites.VolumeDetails{
				{
					ClaimSize: "10Gi",
					VolumeMount: testsuites.VolumeMountDetails{
						NameGenerate:      "test-volume-",
						MountPathGenerate: "/mnt/test-",
					},
				},
			},
		}

		podCheckCmd := []string{"cat", "/mnt/test-1/data"}
		expectedString := "hello world\n"
		test := testsuites.DynamicallyProvisionedRestartDriverTest{
			CSIDriver: testDriver,
			Pod:       pod,
			PodCheck: &testsuites.PodExecCheck{
				Cmd:            podCheckCmd,
				ExpectedString: expectedString,
			},
			StorageClassParameters: make(map[string]string),
			RestartDriverFunc: func() {
				restartDriver := testCmd{
					command:  "bash",
					args:     []string{"test/utils/restart_driver_daemonset.sh"},
					startLog: "Restart driver node daemonset ...",
					endLog:   "Restart driver node daemonset done successfully",
				}
				execTestCmd([]testCmd{restartDriver})
			},
		}
		test.Run(ctx, cs, ns)
	})

	ginkgo.It("should clone a volume from an existing NFSv3 volume [nfs]", func(ctx ginkgo.SpecContext) {
		pod := testsuites.PodDetails{
			Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
			Volumes: []testsuites.VolumeDetails{
				{
					ClaimSize: "10Gi",
					MountOptions: []string{
						"nconnect=8",
					},
					VolumeMount: testsuites.VolumeMountDetails{
						NameGenerate:      "test-volume-",
						MountPathGenerate: "/mnt/test-",
					},
				},
			},
		}
		podWithClonedVolume := testsuites.PodDetails{
			Cmd: "grep 'hello world' /mnt/test-1/data",
		}
		test := testsuites.DynamicallyProvisionedVolumeCloningTest{
			CSIDriver:           testDriver,
			Pod:                 pod,
			PodWithClonedVolume: podWithClonedVolume,
			StorageClassParameters: map[string]string{
				"skuName":          "Premium_LRS",
				"protocol":         "nfs",
				"mountPermissions": "0755",
				"secretNamespace":  "kube-system",
				"secretName":       fmt.Sprintf("secret-%d", time.Now().Unix()),
			},
		}
		test.Run(ctx, cs, ns)
	})

	ginkgo.It("should clone a large size volume from an existing NFSv3 volume [nfs]", func(ctx ginkgo.SpecContext) {
		pod := testsuites.PodDetails{
			Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data && dd if=/dev/zero of=/mnt/test-1/test bs=99G count=5",
			Volumes: []testsuites.VolumeDetails{
				{
					ClaimSize: "100Gi",
					MountOptions: []string{
						"nconnect=8",
					},
					VolumeMount: testsuites.VolumeMountDetails{
						NameGenerate:      "test-volume-",
						MountPathGenerate: "/mnt/test-",
					},
				},
			},
		}
		podWithClonedVolume := testsuites.PodDetails{
			Cmd: "grep 'hello world' /mnt/test-1/data",
		}
		test := testsuites.DynamicallyProvisionedVolumeCloningTest{
			CSIDriver:           testDriver,
			Pod:                 pod,
			PodWithClonedVolume: podWithClonedVolume,
			StorageClassParameters: map[string]string{
				"skuName":          "Premium_LRS",
				"protocol":         "nfs",
				"mountPermissions": "0755",
				"secretNamespace":  "kube-system",
				"secretName":       fmt.Sprintf("secret-%d", time.Now().Unix()),
			},
		}
		test.Run(ctx, cs, ns)
	})

	ginkgo.It("should clone a volume from an existing blobfuse2 volume [fuse2]", func(ctx ginkgo.SpecContext) {
		pod := testsuites.PodDetails{
			Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
			Volumes: []testsuites.VolumeDetails{
				{
					ClaimSize: "10Gi",
					MountOptions: []string{
						"-o allow_other",
						"--virtual-directory=true", // blobfuse2 mount options
					},
					VolumeMount: testsuites.VolumeMountDetails{
						NameGenerate:      "test-volume-",
						MountPathGenerate: "/mnt/test-",
					},
				},
			},
		}
		podWithClonedVolume := testsuites.PodDetails{
			Cmd: "grep 'hello world' /mnt/test-1/data",
		}
		test := testsuites.DynamicallyProvisionedVolumeCloningTest{
			CSIDriver:           testDriver,
			Pod:                 pod,
			PodWithClonedVolume: podWithClonedVolume,
			StorageClassParameters: map[string]string{
				"skuName":         "Standard_LRS",
				"protocol":        "fuse2",
				"secretNamespace": "kube-system",
				"secretName":      fmt.Sprintf("secret-%d", time.Now().Unix()),
			},
		}
		test.Run(ctx, cs, ns)
	})

	ginkgo.It("should clone a large size volume from an existing blobfuse2 volume [fuse2]", func(ctx ginkgo.SpecContext) {
		pod := testsuites.PodDetails{
			Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data && dd if=/dev/zero of=/mnt/test-1/test bs=99G count=5",
			Volumes: []testsuites.VolumeDetails{
				{
					ClaimSize: "100Gi",
					MountOptions: []string{
						"-o allow_other",
						"--virtual-directory=true", // blobfuse2 mount options
					},
					VolumeMount: testsuites.VolumeMountDetails{
						NameGenerate:      "test-volume-",
						MountPathGenerate: "/mnt/test-",
					},
				},
			},
		}
		podWithClonedVolume := testsuites.PodDetails{
			Cmd: "grep 'hello world' /mnt/test-1/data",
		}
		test := testsuites.DynamicallyProvisionedVolumeCloningTest{
			CSIDriver:           testDriver,
			Pod:                 pod,
			PodWithClonedVolume: podWithClonedVolume,
			StorageClassParameters: map[string]string{
				"skuName":         "Standard_LRS",
				"protocol":        "fuse2",
				"secretNamespace": "kube-system",
				"secretName":      fmt.Sprintf("secret-%d", time.Now().Unix()),
			},
		}
		test.Run(ctx, cs, ns)
	})

	ginkgo.It("should clone a volume from an existing NFSv3 volume to another storage class [nfs]", func(ctx ginkgo.SpecContext) {
		pod := testsuites.PodDetails{
			Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
			Volumes: []testsuites.VolumeDetails{
				{
					ClaimSize: "10Gi",
					MountOptions: []string{
						"nconnect=8",
					},
					VolumeMount: testsuites.VolumeMountDetails{
						NameGenerate:      "test-volume-",
						MountPathGenerate: "/mnt/test-",
					},
				},
			},
		}
		podWithClonedVolume := testsuites.PodDetails{
			Cmd: "grep 'hello world' /mnt/test-1/data",
		}
		test := testsuites.DynamicallyProvisionedVolumeCloningTest{
			CSIDriver:           testDriver,
			Pod:                 pod,
			PodWithClonedVolume: podWithClonedVolume,
			StorageClassParameters: map[string]string{
				"skuName":              "Premium_LRS",
				"protocol":             "nfs",
				"mountPermissions":     "0755",
				"allowsharedkeyaccess": "true",
				"secretNamespace":      "kube-system",
				"secretName":           fmt.Sprintf("secret-%d", time.Now().Unix()),
			},
			ClonedStorageClassParameters: map[string]string{
				"skuName":              "Standard_LRS",
				"protocol":             "nfs",
				"mountPermissions":     "0755",
				"allowsharedkeyaccess": "true",
			},
		}
		test.Run(ctx, cs, ns)
	})

	ginkgo.It("should clone a volume from an existing blobfuse2 volume to another storage class [fuse2]", func(ctx ginkgo.SpecContext) {
		pod := testsuites.PodDetails{
			Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
			Volumes: []testsuites.VolumeDetails{
				{
					ClaimSize: "10Gi",
					MountOptions: []string{
						"-o allow_other",
						"--virtual-directory=true", // blobfuse2 mount options
					},
					VolumeMount: testsuites.VolumeMountDetails{
						NameGenerate:      "test-volume-",
						MountPathGenerate: "/mnt/test-",
					},
				},
			},
		}
		podWithClonedVolume := testsuites.PodDetails{
			Cmd: "grep 'hello world' /mnt/test-1/data",
		}
		test := testsuites.DynamicallyProvisionedVolumeCloningTest{
			CSIDriver:           testDriver,
			Pod:                 pod,
			PodWithClonedVolume: podWithClonedVolume,
			StorageClassParameters: map[string]string{
				"skuName":         "Standard_LRS",
				"protocol":        "fuse2",
				"secretNamespace": "kube-system",
				"secretName":      fmt.Sprintf("secret-%d", time.Now().Unix()),
			},
			ClonedStorageClassParameters: map[string]string{
				"skuName":  "Premium_LRS",
				"protocol": "fuse2",
			},
		}
		test.Run(ctx, cs, ns)
	})
})
