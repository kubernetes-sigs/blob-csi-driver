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
	"context"
	"fmt"
	"math/rand"
	"strconv"

	"sigs.k8s.io/blob-csi-driver/test/e2e/driver"
	"sigs.k8s.io/blob-csi-driver/test/e2e/testsuites"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/onsi/ginkgo/v2"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	admissionapi "k8s.io/pod-security-admission/api"
)

const (
	defaultVolumeSize = 10
)

var (
	defaultVolumeSizeBytes int64 = defaultVolumeSize * 1024 * 1024 * 1024
)

var _ = ginkgo.Describe("[blob-csi-e2e] Pre-Provisioned", func() {
	f := framework.NewDefaultFramework("blob")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

	var (
		cs         clientset.Interface
		ns         *v1.Namespace
		testDriver driver.PreProvisionedVolumeTestDriver
		volumeID   string
	)

	ginkgo.BeforeEach(func() {
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

		volName := fmt.Sprintf("pre-provisioned-%d-%v", ginkgo.GinkgoParallelProcess(), strconv.Itoa(rand.Intn(10000)))
		resp, err := blobDriver.CreateVolume(context.Background(), makeCreateVolumeReq(volName, ns.Name))
		framework.ExpectNoError(err, "create volume error")
		volumeID = resp.Volume.VolumeId

		ginkgo.DeferCleanup(func() {
			_, err := blobDriver.DeleteVolume(
				context.Background(),
				&csi.DeleteVolumeRequest{
					VolumeId: volumeID,
				})
			framework.ExpectNoError(err, "delete volume %s error", volumeID)
		})
	})

	ginkgo.It("[env] should use a pre-provisioned volume and mount it as readOnly in a pod", func() {
		volumeSize := fmt.Sprintf("%dGi", defaultVolumeSize)
		pods := []testsuites.PodDetails{
			{
				Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
				Volumes: []testsuites.VolumeDetails{
					{
						VolumeID:  volumeID,
						FSType:    "ext4",
						ClaimSize: volumeSize,
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
							ReadOnly:          true,
						},
					},
				},
			},
		}
		test := testsuites.PreProvisionedReadOnlyVolumeTest{
			CSIDriver: testDriver,
			Pods:      pods,
		}
		test.Run(cs, ns)
	})

	ginkgo.It(fmt.Sprintf("[env] should use a pre-provisioned volume and retain PV with reclaimPolicy %q", v1.PersistentVolumeReclaimRetain), func() {
		volumeSize := fmt.Sprintf("%dGi", defaultVolumeSize)
		reclaimPolicy := v1.PersistentVolumeReclaimRetain
		volumes := []testsuites.VolumeDetails{
			{
				VolumeID:      volumeID,
				FSType:        "ext4",
				ClaimSize:     volumeSize,
				ReclaimPolicy: &reclaimPolicy,
			},
		}
		test := testsuites.PreProvisionedReclaimPolicyTest{
			CSIDriver: testDriver,
			Volumes:   volumes,
		}
		test.Run(cs, ns)
	})

	ginkgo.It("should use a pre-provisioned volume and mount it by multiple pods", func() {
		volumeSize := fmt.Sprintf("%dGi", defaultVolumeSize)
		pods := []testsuites.PodDetails{}
		for i := 1; i <= 6; i++ {
			pod := testsuites.PodDetails{
				Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
				Volumes: []testsuites.VolumeDetails{
					{
						// make VolumeID unique in test
						// to workaround the issue: https://github.com/kubernetes/kubernetes/pull/107065
						// which was fixed in k8s 1.24
						VolumeID:  fmt.Sprintf("%s%d", volumeID, i),
						ClaimSize: volumeSize,
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
			}
			pods = append(pods, pod)
		}

		test := testsuites.PreProvisionedMultiplePods{
			CSIDriver: testDriver,
			Pods:      pods,
		}
		test.Run(cs, ns)
	})

	ginkgo.It("should use existing credentials in k8s cluster", func() {
		volumeSize := fmt.Sprintf("%dGi", defaultVolumeSize)
		reclaimPolicy := v1.PersistentVolumeReclaimRetain
		volumeBindingMode := storagev1.VolumeBindingImmediate

		pods := []testsuites.PodDetails{
			{
				Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
				Volumes: []testsuites.VolumeDetails{
					{
						VolumeID:          volumeID,
						FSType:            "ext4",
						ClaimSize:         volumeSize,
						ReclaimPolicy:     &reclaimPolicy,
						VolumeBindingMode: &volumeBindingMode,
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
			},
		}
		test := testsuites.PreProvisionedExistingCredentialsTest{
			CSIDriver: testDriver,
			Pods:      pods,
		}
		test.Run(cs, ns)
	})

	ginkgo.It("should use provided credentials", ginkgo.Label("flaky"), func() {
		volumeSize := fmt.Sprintf("%dGi", defaultVolumeSize)
		reclaimPolicy := v1.PersistentVolumeReclaimRetain
		volumeBindingMode := storagev1.VolumeBindingImmediate

		pods := []testsuites.PodDetails{
			{
				Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
				Volumes: []testsuites.VolumeDetails{
					{
						VolumeID:          volumeID,
						FSType:            "ext4",
						ClaimSize:         volumeSize,
						ReclaimPolicy:     &reclaimPolicy,
						VolumeBindingMode: &volumeBindingMode,
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
						NodeStageSecretRef: "azure-secret",
						Attrib:             make(map[string]string),
					},
				},
			},
		}
		test := testsuites.PreProvisionedProvidedCredentiasTest{
			CSIDriver: testDriver,
			Pods:      pods,
			Driver:    blobDriver,
		}
		test.Run(cs, ns)
	})

	ginkgo.It("should use Key Vault", func() {
		volumeSize := fmt.Sprintf("%dGi", defaultVolumeSize)
		reclaimPolicy := v1.PersistentVolumeReclaimRetain
		volumeBindingMode := storagev1.VolumeBindingImmediate

		pods := []testsuites.PodDetails{
			{
				Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
				Volumes: []testsuites.VolumeDetails{
					{
						VolumeID:          volumeID,
						FSType:            "ext4",
						ClaimSize:         volumeSize,
						ReclaimPolicy:     &reclaimPolicy,
						VolumeBindingMode: &volumeBindingMode,
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
						Attrib: make(map[string]string),
					},
				},
			},
		}

		test := testsuites.PreProvisionedKeyVaultTest{
			CSIDriver: testDriver,
			Pods:      pods,
			Driver:    blobDriver,
		}
		test.Run(cs, ns)
	})

	ginkgo.It("should use SAS token", func() {
		pods := []testsuites.PodDetails{
			{
				Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
				Volumes: []testsuites.VolumeDetails{
					{
						VolumeID:          volumeID,
						FSType:            "ext4",
						ClaimSize:         fmt.Sprintf("%dGi", defaultVolumeSize),
						ReclaimPolicy:     to.Ptr(v1.PersistentVolumeReclaimRetain),
						VolumeBindingMode: to.Ptr(storagev1.VolumeBindingImmediate),
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
						Attrib: make(map[string]string),
					},
				},
			},
		}

		test := testsuites.PreProvisionedSASTokenTest{
			CSIDriver: testDriver,
			Pods:      pods,
			Driver:    blobDriver,
		}
		test.Run(cs, ns)
	})
})

func makeCreateVolumeReq(volumeName, secretNamespace string) *csi.CreateVolumeRequest {
	req := &csi.CreateVolumeRequest{
		Name: volumeName,
		VolumeCapabilities: []*csi.VolumeCapability{
			{
				AccessType: &csi.VolumeCapability_Mount{
					Mount: &csi.VolumeCapability_MountVolume{},
				},
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				},
			},
		},
		CapacityRange: &csi.CapacityRange{
			RequiredBytes: defaultVolumeSizeBytes,
			LimitBytes:    defaultVolumeSizeBytes,
		},
		Parameters: map[string]string{
			"skuname":         "Standard_LRS",
			"containerName":   volumeName,
			"secretNamespace": secretNamespace,
		},
	}

	return req
}
