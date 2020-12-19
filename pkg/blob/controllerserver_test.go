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

package blob

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Azure/azure-sdk-for-go/services/storage/mgmt/2019-06-01/storage"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"sigs.k8s.io/cloud-provider-azure/pkg/azureclients/storageaccountclient/mockstorageaccountclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/provider"
	"sigs.k8s.io/cloud-provider-azure/pkg/retry"
)

func TestControllerGetCapabilities(t *testing.T) {
	d := NewFakeDriver()
	controlCap := []*csi.ControllerServiceCapability{
		{
			Type: &csi.ControllerServiceCapability_Rpc{},
		},
	}
	d.Cap = controlCap
	req := csi.ControllerGetCapabilitiesRequest{}
	resp, err := d.ControllerGetCapabilities(context.Background(), &req)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, resp.Capabilities, controlCap)
}

func TestCreateVolume(t *testing.T) {
	stdVolumeCapability := &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Mount{
			Mount: &csi.VolumeCapability_MountVolume{},
		},
	}
	stdVolumeCapabilities := []*csi.VolumeCapability{
		stdVolumeCapability,
	}
	controllerservicecapabilityRPC := &csi.ControllerServiceCapability_RPC{
		Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
	}
	controllerServiceCapability := &csi.ControllerServiceCapability{
		Type: &csi.ControllerServiceCapability_Rpc{
			Rpc: controllerservicecapabilityRPC,
		},
	}
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "invalid create volume req",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				req := &csi.CreateVolumeRequest{}
				_, err := d.CreateVolume(context.Background(), req)
				expectedErr := status.Error(codes.InvalidArgument, "CREATE_DELETE_VOLUME")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "volume Name missing",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.Cap = []*csi.ControllerServiceCapability{
					controllerServiceCapability,
				}
				req := &csi.CreateVolumeRequest{}
				_, err := d.CreateVolume(context.Background(), req)
				expectedErr := status.Error(codes.InvalidArgument, "CreateVolume Name must be provided")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "volume capacity missing",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				req := &csi.CreateVolumeRequest{
					Name: "unit-test",
				}
				d.Cap = []*csi.ControllerServiceCapability{
					controllerServiceCapability,
				}
				_, err := d.CreateVolume(context.Background(), req)
				expectedErr := status.Error(codes.InvalidArgument, "CreateVolume Volume capabilities must be provided")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "invalid protocol",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &azure.Cloud{}
				mp := make(map[string]string)
				mp["protocol"] = "unit-test"
				mp[skuNameField] = "unit-test"
				mp[storageAccountTypeField] = "unit-test"
				mp[locationField] = "unit-test"
				mp[storageAccountField] = "unit-test"
				mp[resourceGroupField] = "unit-test"
				mp["containername"] = "unit-test"
				req := &csi.CreateVolumeRequest{
					Name:               "unit-test",
					VolumeCapabilities: stdVolumeCapabilities,
					Parameters:         mp,
				}
				d.Cap = []*csi.ControllerServiceCapability{
					controllerServiceCapability,
				}
				_, err := d.CreateVolume(context.Background(), req)
				expectedErr := status.Errorf(codes.InvalidArgument, "protocol(unit-test) is not supported, supported protocol list: [fuse nfs]")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "storageacount empty while nfs",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &azure.Cloud{}
				mp := make(map[string]string)
				mp["protocol"] = "nfs"
				req := &csi.CreateVolumeRequest{
					Name:               "unit-test",
					VolumeCapabilities: stdVolumeCapabilities,
					Parameters:         mp,
				}
				d.Cap = []*csi.ControllerServiceCapability{
					controllerServiceCapability,
				}
				_, err := d.CreateVolume(context.Background(), req)
				expectedErr := status.Errorf(codes.InvalidArgument, "storage account must be specified when provisioning nfs file share")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "tags error",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &azure.Cloud{}
				mp := make(map[string]string)
				mp["tags"] = "unit-test"
				mp[storageAccountTypeField] = "premium"
				req := &csi.CreateVolumeRequest{
					Name:               "unit-test",
					VolumeCapabilities: stdVolumeCapabilities,
					Parameters:         mp,
				}
				d.Cap = []*csi.ControllerServiceCapability{
					controllerServiceCapability,
				}
				_, err := d.CreateVolume(context.Background(), req)
				expectedErr := fmt.Errorf("Tags 'unit-test' are invalid, the format should like: 'key1=value1,key2=value2'")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "getStorageAccounts error",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				mp := make(map[string]string)
				mp[skuNameField] = "unit-test"
				mp[storageAccountTypeField] = "unit-test"
				mp[locationField] = "unit-test"
				mp[storageAccountField] = "unit-test"
				mp[resourceGroupField] = "unit-test"
				mp["containername"] = "unit-test"
				req := &csi.CreateVolumeRequest{
					Name:               "unit-test",
					VolumeCapabilities: stdVolumeCapabilities,
				}
				d.Cap = []*csi.ControllerServiceCapability{
					controllerServiceCapability,
				}

				d.cloud = &azure.Cloud{}
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				mockStorageAccountsClient := mockstorageaccountclient.NewMockInterface(ctrl)
				d.cloud.StorageAccountClient = mockStorageAccountsClient
				rerr := &retry.Error{
					RawError: fmt.Errorf("test"),
				}
				mockStorageAccountsClient.EXPECT().ListByResourceGroup(gomock.Any(), gomock.Any()).Return(nil, rerr).AnyTimes()
				_, err := d.CreateVolume(context.Background(), req)
				expectedErr := status.Errorf(codes.Internal, "failed to ensure storage account: could not list storage accounts for account type : Retriable: false, RetryAfter: 0s, HTTPStatusCode: 0, RawError: test")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func TestDeleteVolume(t *testing.T) {
	controllerservicecapabilityRPC := &csi.ControllerServiceCapability_RPC{
		Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
	}
	controllerServiceCapability := &csi.ControllerServiceCapability{
		Type: &csi.ControllerServiceCapability_Rpc{
			Rpc: controllerservicecapabilityRPC,
		},
	}
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "volume ID missing",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				req := &csi.DeleteVolumeRequest{}
				_, err := d.DeleteVolume(context.Background(), req)
				expectedErr := status.Error(codes.InvalidArgument, "Volume ID missing in request")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "invalid delete volume req",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				req := &csi.DeleteVolumeRequest{
					VolumeId: "unit-test",
				}
				_, err := d.DeleteVolume(context.Background(), req)
				expectedErr := fmt.Errorf("invalid delete volume req: volume_id:\"unit-test\" ")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "invalid volume Id",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.Cap = []*csi.ControllerServiceCapability{
					controllerServiceCapability,
				}
				req := &csi.DeleteVolumeRequest{
					VolumeId: "unit-test",
				}
				_, err := d.DeleteVolume(context.Background(), req)
				expectedErr := error(nil)
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "ListKeys error",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.Cap = []*csi.ControllerServiceCapability{
					controllerServiceCapability,
				}
				req := &csi.DeleteVolumeRequest{
					VolumeId: "#test#test",
				}
				d.cloud = &azure.Cloud{}
				d.cloud.ResourceGroup = "unit"
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				mockStorageAccountsClient := mockstorageaccountclient.NewMockInterface(ctrl)
				d.cloud.StorageAccountClient = mockStorageAccountsClient
				rerr := &retry.Error{
					RawError: fmt.Errorf("test"),
				}
				accountListKeysResult := storage.AccountListKeysResult{}
				mockStorageAccountsClient.EXPECT().ListKeys(gomock.Any(), gomock.Any(), gomock.Any()).Return(accountListKeysResult, rerr).AnyTimes()
				expectedErr := fmt.Errorf("no key for storage account(test) under resource group(unit), err Retriable: false, RetryAfter: 0s, HTTPStatusCode: 0, RawError: test")
				_, err := d.DeleteVolume(context.Background(), req)
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "base storage service url empty",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.Cap = []*csi.ControllerServiceCapability{
					controllerServiceCapability,
				}
				req := &csi.DeleteVolumeRequest{
					VolumeId: "unit#test#test",
				}
				d.cloud = &azure.Cloud{}
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				mockStorageAccountsClient := mockstorageaccountclient.NewMockInterface(ctrl)
				d.cloud.StorageAccountClient = mockStorageAccountsClient
				s := "unit-test"
				accountkey := storage.AccountKey{
					Value: &s,
				}
				accountkeylist := []storage.AccountKey{}
				accountkeylist = append(accountkeylist, accountkey)
				list := storage.AccountListKeysResult{
					Keys: &accountkeylist,
				}
				mockStorageAccountsClient.EXPECT().ListKeys(gomock.Any(), gomock.Any(), gomock.Any()).Return(list, nil).AnyTimes()
				expectedErr := fmt.Errorf("azure: base storage service url required")
				_, err := d.DeleteVolume(context.Background(), req)
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func TestValidateVolumeCapabilities(t *testing.T) {
	stdVolumeCapability := &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Mount{
			Mount: &csi.VolumeCapability_MountVolume{},
		},
	}
	stdVolumeCapabilities := []*csi.VolumeCapability{
		stdVolumeCapability,
	}
	controllerservicecapabilityRPC := &csi.ControllerServiceCapability_RPC{
		Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
	}
	controllerServiceCapability := &csi.ControllerServiceCapability{
		Type: &csi.ControllerServiceCapability_Rpc{
			Rpc: controllerservicecapabilityRPC,
		},
	}
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "volume ID missing",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				req := &csi.ValidateVolumeCapabilitiesRequest{}
				_, err := d.ValidateVolumeCapabilities(context.Background(), req)
				expectedErr := status.Error(codes.InvalidArgument, "Volume ID missing in request")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "volume capability missing",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				req := &csi.ValidateVolumeCapabilitiesRequest{
					VolumeId: "unit-test",
				}
				_, err := d.ValidateVolumeCapabilities(context.Background(), req)
				expectedErr := status.Error(codes.InvalidArgument, "Volume capabilities missing in request")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "invalid volume Id",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				req := &csi.ValidateVolumeCapabilitiesRequest{
					VolumeId:           "unit-test",
					VolumeCapabilities: stdVolumeCapabilities,
				}
				_, err := d.ValidateVolumeCapabilities(context.Background(), req)
				expectedErr := status.Error(codes.NotFound, "error parsing volume id: \"unit-test\", should at least contain two #")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "ListKeys error",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.Cap = []*csi.ControllerServiceCapability{
					controllerServiceCapability,
				}
				req := &csi.ValidateVolumeCapabilitiesRequest{
					VolumeId:           "#test#test",
					VolumeCapabilities: stdVolumeCapabilities,
				}
				d.cloud = &azure.Cloud{}
				d.cloud.ResourceGroup = "unit"
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				mockStorageAccountsClient := mockstorageaccountclient.NewMockInterface(ctrl)
				d.cloud.StorageAccountClient = mockStorageAccountsClient
				rerr := &retry.Error{
					RawError: fmt.Errorf("test"),
				}
				accountListKeysResult := storage.AccountListKeysResult{}
				mockStorageAccountsClient.EXPECT().ListKeys(gomock.Any(), gomock.Any(), gomock.Any()).Return(accountListKeysResult, rerr).AnyTimes()
				expectedErr := fmt.Errorf("no key for storage account(test) under resource group(unit), err Retriable: false, RetryAfter: 0s, HTTPStatusCode: 0, RawError: test")
				_, err := d.ValidateVolumeCapabilities(context.Background(), req)
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "base storage service url empty",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.Cap = []*csi.ControllerServiceCapability{
					controllerServiceCapability,
				}
				req := &csi.ValidateVolumeCapabilitiesRequest{
					VolumeId:           "unit#test#test",
					VolumeCapabilities: stdVolumeCapabilities,
				}
				d.cloud = &azure.Cloud{}
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				mockStorageAccountsClient := mockstorageaccountclient.NewMockInterface(ctrl)
				d.cloud.StorageAccountClient = mockStorageAccountsClient
				s := "unit-test"
				accountkey := storage.AccountKey{
					Value: &s,
				}
				accountkeylist := []storage.AccountKey{}
				accountkeylist = append(accountkeylist, accountkey)
				list := storage.AccountListKeysResult{
					Keys: &accountkeylist,
				}
				mockStorageAccountsClient.EXPECT().ListKeys(gomock.Any(), gomock.Any(), gomock.Any()).Return(list, nil).AnyTimes()
				expectedErr := fmt.Errorf("azure: base storage service url required")
				_, err := d.ValidateVolumeCapabilities(context.Background(), req)
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func TestGetCapacity(t *testing.T) {
	d := NewFakeDriver()
	req := csi.GetCapacityRequest{}
	resp, err := d.GetCapacity(context.Background(), &req)
	assert.Nil(t, resp)
	if !reflect.DeepEqual(err, status.Error(codes.Unimplemented, "")) {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestListVolumes(t *testing.T) {
	d := NewFakeDriver()
	req := csi.ListVolumesRequest{}
	resp, err := d.ListVolumes(context.Background(), &req)
	assert.Nil(t, resp)
	if !reflect.DeepEqual(err, status.Error(codes.Unimplemented, "")) {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestControllerPublishVolume(t *testing.T) {
	d := NewFakeDriver()
	req := csi.ControllerPublishVolumeRequest{}
	resp, err := d.ControllerPublishVolume(context.Background(), &req)
	assert.Nil(t, resp)
	if !reflect.DeepEqual(err, status.Error(codes.Unimplemented, "")) {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestControllerUnpublishVolume(t *testing.T) {
	d := NewFakeDriver()
	req := csi.ControllerUnpublishVolumeRequest{}
	resp, err := d.ControllerUnpublishVolume(context.Background(), &req)
	assert.Nil(t, resp)
	if !reflect.DeepEqual(err, status.Error(codes.Unimplemented, "")) {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestCreateSnapshots(t *testing.T) {
	d := NewFakeDriver()
	req := csi.CreateSnapshotRequest{}
	resp, err := d.CreateSnapshot(context.Background(), &req)
	assert.Nil(t, resp)
	if !reflect.DeepEqual(err, status.Error(codes.Unimplemented, "")) {
		t.Errorf("Unexpected error: %v", err)
	}
}
func TestDeleteSnapshots(t *testing.T) {
	d := NewFakeDriver()
	req := csi.DeleteSnapshotRequest{}
	resp, err := d.DeleteSnapshot(context.Background(), &req)
	assert.Nil(t, resp)
	if !reflect.DeepEqual(err, status.Error(codes.Unimplemented, "")) {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestListSnapshots(t *testing.T) {
	d := NewFakeDriver()
	req := csi.ListSnapshotsRequest{}
	resp, err := d.ListSnapshots(context.Background(), &req)
	assert.Nil(t, resp)
	if !reflect.DeepEqual(err, status.Error(codes.Unimplemented, "")) {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestControllerExpandVolume(t *testing.T) {
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "volume ID missing",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				req := &csi.ControllerExpandVolumeRequest{}
				_, err := d.ControllerExpandVolume(context.Background(), req)
				expectedErr := status.Error(codes.InvalidArgument, "Volume ID missing in request")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "Capacity Range missing",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				req := &csi.ControllerExpandVolumeRequest{
					VolumeId: "unit-test",
				}
				_, err := d.ControllerExpandVolume(context.Background(), req)
				expectedErr := status.Error(codes.InvalidArgument, "Capacity Range missing in request")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "invalid expand volume req",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				req := &csi.ControllerExpandVolumeRequest{
					VolumeId:      "unit-test",
					CapacityRange: &csi.CapacityRange{},
				}
				_, err := d.ControllerExpandVolume(context.Background(), req)
				expectedErr := fmt.Errorf("invalid expand volume req: volume_id:\"unit-test\" capacity_range:<> ")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}
