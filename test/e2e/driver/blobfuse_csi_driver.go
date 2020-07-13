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

package driver

import (
	"fmt"

	"sigs.k8s.io/blobfuse-csi-driver/pkg/blobfuse"

	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Implement DynamicPVTestDriver interface
type blobFuseCSIDriver struct {
	driverName string
}

// InitBlobFuseCSIDriver returns blobFuseCSIDriver that implemnts DynamicPVTestDriver interface
func InitBlobFuseCSIDriver() PVTestDriver {
	return &blobFuseCSIDriver{
		driverName: blobfuse.DriverName,
	}
}

func (d *blobFuseCSIDriver) GetDynamicProvisionStorageClass(parameters map[string]string, mountOptions []string, reclaimPolicy *v1.PersistentVolumeReclaimPolicy, bindingMode *storagev1.VolumeBindingMode, allowedTopologyValues []string, namespace string) *storagev1.StorageClass {
	provisioner := d.driverName
	generateName := fmt.Sprintf("%s-%s-dynamic-sc-", namespace, provisioner)
	return getStorageClass(generateName, provisioner, parameters, mountOptions, reclaimPolicy, bindingMode, nil)
}

func (d *blobFuseCSIDriver) GetPersistentVolume(volumeID string, fsType string, size string, reclaimPolicy *v1.PersistentVolumeReclaimPolicy, namespace string, attrib map[string]string, nodeStageSecretRef string) *v1.PersistentVolume {
	provisioner := d.driverName
	generateName := fmt.Sprintf("%s-%s-preprovsioned-pv-", namespace, provisioner)
	// Default to Retain ReclaimPolicy for pre-provisioned volumes
	pvReclaimPolicy := v1.PersistentVolumeReclaimRetain
	if reclaimPolicy != nil {
		pvReclaimPolicy = *reclaimPolicy
	}
	stageSecretRef := &v1.SecretReference{}
	if nodeStageSecretRef != "" {
		stageSecretRef.Name = nodeStageSecretRef
		stageSecretRef.Namespace = namespace
	} else {
		stageSecretRef = nil
	}
	return &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: generateName,
			Namespace:    namespace,
			// TODO remove if https://github.com/kubernetes-csi/external-provisioner/issues/202 is fixed
			Annotations: map[string]string{
				"pv.kubernetes.io/provisioned-by": provisioner,
			},
		},
		Spec: v1.PersistentVolumeSpec{
			AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
			Capacity: v1.ResourceList{
				v1.ResourceName(v1.ResourceStorage): resource.MustParse(size),
			},
			PersistentVolumeReclaimPolicy: pvReclaimPolicy,
			PersistentVolumeSource: v1.PersistentVolumeSource{
				CSI: &v1.CSIPersistentVolumeSource{
					Driver:             provisioner,
					VolumeHandle:       volumeID,
					FSType:             fsType,
					VolumeAttributes:   attrib,
					NodeStageSecretRef: stageSecretRef,
				},
			},
		},
	}
}

func (d *blobFuseCSIDriver) GetPreProvisionStorageClass(parameters map[string]string, mountOptions []string, reclaimPolicy *v1.PersistentVolumeReclaimPolicy, bindingMode *storagev1.VolumeBindingMode, allowedTopologyValues []string, namespace string) *storagev1.StorageClass {
	provisioner := d.driverName
	generateName := fmt.Sprintf("%s-%s-pre-provisioned-sc-", namespace, provisioner)
	return getStorageClass(generateName, provisioner, parameters, mountOptions, reclaimPolicy, bindingMode, nil)
}
