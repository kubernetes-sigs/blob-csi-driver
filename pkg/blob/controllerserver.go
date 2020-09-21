/*
Copyright 2017 The Kubernetes Authors.

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

package blob

import (
	"context"
	"fmt"
	"strings"

	"sigs.k8s.io/blob-csi-driver/pkg/util"

	"github.com/Azure/azure-sdk-for-go/services/storage/mgmt/2019-06-01/storage"
	azstorage "github.com/Azure/azure-sdk-for-go/storage"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"k8s.io/legacy-cloud-providers/azure"
)

// CreateVolume provisions a volume
func (d *Driver) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	if err := d.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		klog.Errorf("invalid create volume req: %v", req)
		return nil, err
	}

	volumeCapabilities := req.GetVolumeCapabilities()
	name := req.GetName()
	if len(name) == 0 {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume Name must be provided")
	}
	if len(volumeCapabilities) == 0 {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume Volume capabilities must be provided")
	}

	volSizeBytes := int64(req.GetCapacityRange().GetRequiredBytes())
	requestGiB := int(util.RoundUpGiB(volSizeBytes))

	parameters := req.GetParameters()
	var storageAccountType, resourceGroup, location, account, containerName, protocol, customTags string

	// Apply ProvisionerParameters (case-insensitive). We leave validation of
	// the values to the cloud provider.
	for k, v := range parameters {
		switch strings.ToLower(k) {
		case "skuname":
			storageAccountType = v
		case "storageaccounttype":
			storageAccountType = v
		case "location":
			location = v
		case "storageaccount":
			account = v
		case "resourcegroup":
			resourceGroup = v
		case "containername":
			containerName = v
		case protocolField:
			protocol = v
		case tagsField:
			customTags = v
		}
	}

	if resourceGroup == "" {
		resourceGroup = d.cloud.ResourceGroup
	}

	if protocol == "" {
		protocol = fuse
	}
	if !isSupportedProtocol(protocol) {
		return nil, status.Errorf(codes.InvalidArgument, "protocol(%s) is not supported, supported protocol list: %v", protocol, supportedProtocolList)
	}

	enableHTTPSTrafficOnly := true
	if protocol == nfs {
		if account == "" {
			return nil, status.Errorf(codes.InvalidArgument, "storage account must be specified when provisioning nfs file share")
		}
		enableHTTPSTrafficOnly = false
	}

	accountKind := string(storage.StorageV2)
	if strings.HasPrefix(strings.ToLower(storageAccountType), "premium") {
		accountKind = string(storage.BlockBlobStorage)
	}

	tags, err := azure.ConvertTagsToMap(customTags)
	if err != nil {
		return nil, err
	}

	accountOptions := &azure.AccountOptions{
		Name:                   account,
		Type:                   storageAccountType,
		Kind:                   accountKind,
		ResourceGroup:          resourceGroup,
		Location:               location,
		EnableHTTPSTrafficOnly: enableHTTPSTrafficOnly,
		Tags:                   tags,
	}

	var accountName, accountKey string
	if len(req.GetSecrets()) == 0 { // check whether account is provided by secret
		lockKey := account + storageAccountType + accountKind + resourceGroup + location
		d.volLockMap.LockEntry(lockKey)
		defer d.volLockMap.UnlockEntry(lockKey)

		err = wait.ExponentialBackoff(d.cloud.RequestBackoff(), func() (bool, error) {
			var retErr error
			accountName, accountKey, retErr = d.cloud.EnsureStorageAccount(accountOptions, protocol)
			if isRetriableError(retErr) {
				klog.Warningf("EnsureStorageAccount(%s) failed with error(%v), waiting for retrying", account, retErr)
				return false, nil
			}
			return true, retErr
		})
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to ensure storage account: %v", err)
		}
	} else {
		accountName, accountKey, err = getStorageAccount(req.GetSecrets())
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to get storage account from secrets: %v", err)
		}
	}

	if containerName == "" {
		containerName = getValidContainerName(name, protocol)
	}

	klog.V(2).Infof("begin to create container(%s) on account(%s) type(%s) rg(%s) location(%s) size(%d)", containerName, accountName, storageAccountType, resourceGroup, location, requestGiB)
	client, err := azstorage.NewBasicClientOnSovereignCloud(accountName, accountKey, d.cloud.Environment)
	if err != nil {
		return nil, err
	}
	blobClient := client.GetBlobService()
	container := blobClient.GetContainerReference(containerName)
	_, err = container.CreateIfNotExists(&azstorage.CreateContainerOptions{Access: azstorage.ContainerAccessTypePrivate})
	if err != nil {
		return nil, fmt.Errorf("failed to create container(%s) on account(%s) type(%s) rg(%s) location(%s) size(%d), error: %v", containerName, accountName, storageAccountType, resourceGroup, location, requestGiB, err)
	}

	volumeID := fmt.Sprintf(volumeIDTemplate, resourceGroup, accountName, containerName)

	/* todo: snapshot support
	if req.GetVolumeContentSource() != nil {
		contentSource := req.GetVolumeContentSource()
		if contentSource.GetSnapshot() != nil {
		}
	}
	*/
	klog.V(2).Infof("create container %s on storage account %s successfully", containerName, accountName)

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      volumeID,
			CapacityBytes: req.GetCapacityRange().GetRequiredBytes(),
			VolumeContext: parameters,
		},
	}, nil
}

// DeleteVolume delete a volume
func (d *Driver) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}

	if err := d.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		return nil, fmt.Errorf("invalid delete volume req: %v", req)
	}

	volumeID := req.VolumeId
	resourceGroupName, accountName, containerName, err := GetContainerInfo(volumeID)
	if err != nil {
		klog.Errorf("GetContainerInfo(%s) in DeleteVolume failed with error: %v", volumeID, err)
		return &csi.DeleteVolumeResponse{}, nil
	}

	if resourceGroupName == "" {
		resourceGroupName = d.cloud.ResourceGroup
	}

	var accountKey string
	if len(req.GetSecrets()) == 0 { // check whether account is provided by secret
		accountKey, err = d.cloud.GetStorageAccesskey(accountName, resourceGroupName)
		if err != nil {
			return nil, fmt.Errorf("no key for storage account(%s) under resource group(%s), err %v", accountName, resourceGroupName, err)
		}
	} else {
		accountName, accountKey, err = getStorageAccount(req.GetSecrets())
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to get storage account from secrets: %v", err)
		}
	}

	klog.V(2).Infof("deleting container(%s) rg(%s) account(%s) volumeID(%s)", containerName, resourceGroupName, accountName, volumeID)
	client, err := azstorage.NewBasicClientOnSovereignCloud(accountName, accountKey, d.cloud.Environment)
	if err != nil {
		return nil, err
	}
	blobClient := client.GetBlobService()
	container := blobClient.GetContainerReference(containerName)
	// todo: check what value to add into DeleteContainerOptions
	err = wait.ExponentialBackoff(d.cloud.RequestBackoff(), func() (bool, error) {
		_, err := container.DeleteIfExists(nil)
		if err != nil && !strings.Contains(err.Error(), "ContainerBeingDeleted") {
			return false, fmt.Errorf("failed to delete container(%s) on account(%s), error: %v", containerName, accountName, err)
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}

	klog.V(2).Infof("container(%s) under rg(%s) account(%s) volumeID(%s) is deleted successfully", containerName, resourceGroupName, accountName, volumeID)

	return &csi.DeleteVolumeResponse{}, nil
}

// ValidateVolumeCapabilities return the capabilities of the volume
func (d *Driver) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}
	if req.GetVolumeCapabilities() == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume capabilities missing in request")
	}

	volumeID := req.VolumeId
	resourceGroupName, accountName, containerName, err := GetContainerInfo(volumeID)
	if err != nil {
		klog.Errorf("GetContainerInfo(%s) in ValidateVolumeCapabilities failed with error: %v", volumeID, err)
		return nil, status.Error(codes.NotFound, err.Error())
	}

	if resourceGroupName == "" {
		resourceGroupName = d.cloud.ResourceGroup
	}

	var accountKey string
	if len(req.GetSecrets()) == 0 { // check whether account is provided by secret
		accountKey, err = d.cloud.GetStorageAccesskey(accountName, resourceGroupName)
		if err != nil {
			return nil, fmt.Errorf("no key for storage account(%s) under resource group(%s), err %v", accountName, resourceGroupName, err)
		}
	} else {
		accountName, accountKey, err = getStorageAccount(req.GetSecrets())
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to get storage account from secrets: %v", err)
		}
	}

	client, err := azstorage.NewBasicClientOnSovereignCloud(accountName, accountKey, d.cloud.Environment)
	if err != nil {
		return nil, err
	}
	blobClient := client.GetBlobService()
	container := blobClient.GetContainerReference(containerName)

	exist, err := container.Exists()
	if err != nil {
		return nil, err
	}
	if !exist {
		return nil, status.Error(codes.NotFound, "the requested volume does not exist")
	}

	// blob driver supports all AccessModes, no need to check capabilities here
	return &csi.ValidateVolumeCapabilitiesResponse{Message: ""}, nil
}

// ControllerGetCapabilities returns the capabilities of the Controller plugin
func (d *Driver) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	klog.V(5).Infof("Using default ControllerGetCapabilities")

	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: d.Cap,
	}, nil
}

// GetCapacity returns the capacity of the total available storage pool
func (d *Driver) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

// ListVolumes return all available volumes
func (d *Driver) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

// ControllerPublishVolume make a volume available on some required node
// N/A for blob driver
func (d *Driver) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

// ControllerUnpublishVolume make the volume unavailable on a specified node
// N/A for blob driver
func (d *Driver) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

// CreateSnapshot create a snapshot (todo)
func (d *Driver) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

// DeleteSnapshot delete a snapshot (todo)
func (d *Driver) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

// ListSnapshots list all snapshots (todo)
func (d *Driver) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

// ControllerExpandVolume controller expand volume
func (d *Driver) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ControllerExpandVolume is not yet implemented")
}
