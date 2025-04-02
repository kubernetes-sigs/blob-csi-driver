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
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/blob-csi-driver/pkg/util"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/accountclient/mock_accountclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/blobcontainerclient/mock_blobcontainerclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/mock_azclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/provider/config"
	"sigs.k8s.io/cloud-provider-azure/pkg/provider/storage"
)

type errType int32

const (
	DATAPLANE errType = iota
	MANAGEMENT
	CUSTOM
	NULL
)

// creates and returns mock storage account client
func NewMockSAClient(_ context.Context, ctrl *gomock.Controller, _, _, _ string, keyList []*armstorage.AccountKey) *mock_accountclient.MockInterface {
	cl := mock_accountclient.NewMockInterface(ctrl)

	cl.EXPECT().
		ListKeys(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(keyList, nil).
		AnyTimes()
	return cl
}

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
	blockVolumeCapability := &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Block{
			Block: &csi.VolumeCapability_BlockVolume{},
		},
	}
	blockVolumeCapabilities := []*csi.VolumeCapability{
		blockVolumeCapability,
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
				expectedErr := status.Error(codes.InvalidArgument, "volume capabilities missing in request")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "block volume capability not supported",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				req := &csi.CreateVolumeRequest{
					Name:               "unit-test",
					VolumeCapabilities: blockVolumeCapabilities,
				}
				d.Cap = []*csi.ControllerServiceCapability{
					controllerServiceCapability,
				}
				_, err := d.CreateVolume(context.Background(), req)
				expectedErr := status.Error(codes.InvalidArgument, "block volume capability not supported")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "invalid protocol",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{}
				mp := map[string]string{
					protocolField: "unit-test",
				}
				req := &csi.CreateVolumeRequest{
					Name:               "unit-test",
					VolumeCapabilities: stdVolumeCapabilities,
					Parameters:         mp,
				}
				d.Cap = []*csi.ControllerServiceCapability{
					controllerServiceCapability,
				}
				_, err := d.CreateVolume(context.Background(), req)
				expectedErr := status.Errorf(codes.InvalidArgument, "protocol(unit-test) is not supported, supported protocol list: [fuse fuse2 nfs aznfs]")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "Invalid fsGroupChangePolicy",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{}
				mp := map[string]string{
					fsGroupChangePolicyField: "test_fsGroupChangePolicy",
				}
				req := &csi.CreateVolumeRequest{
					Name:               "unit-test",
					VolumeCapabilities: stdVolumeCapabilities,
					Parameters:         mp,
				}
				d.Cap = []*csi.ControllerServiceCapability{
					controllerServiceCapability,
				}
				_, err := d.CreateVolume(context.Background(), req)
				expectedErr := status.Errorf(codes.InvalidArgument, "fsGroupChangePolicy(test_fsGroupChangePolicy) is not supported, supported fsGroupChangePolicy list: [None Always OnRootMismatch]")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "invalid getLatestAccountKey value",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{}
				mp := map[string]string{
					getLatestAccountKeyField: "invalid",
				}
				req := &csi.CreateVolumeRequest{
					Name:               "unit-test",
					VolumeCapabilities: stdVolumeCapabilities,
					Parameters:         mp,
				}
				d.Cap = []*csi.ControllerServiceCapability{
					controllerServiceCapability,
				}
				_, err := d.CreateVolume(context.Background(), req)
				expectedErr := status.Errorf(codes.InvalidArgument, "invalid %s: %s in volume context", getLatestAccountKeyField, "invalid")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "storageAccount and matchTags conflict",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{}
				mp := map[string]string{
					storageAccountField: "abc",
					matchTagsField:      "true",
				}
				req := &csi.CreateVolumeRequest{
					Name:               "unit-test",
					VolumeCapabilities: stdVolumeCapabilities,
					Parameters:         mp,
				}
				d.Cap = []*csi.ControllerServiceCapability{
					controllerServiceCapability,
				}
				_, err := d.CreateVolume(context.Background(), req)
				expectedErr := status.Errorf(codes.InvalidArgument, "matchTags must set as false when storageAccount(abc) is provided")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "containerName and containerNamePrefix could not be specified together",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{}
				mp := make(map[string]string)
				mp[containerNameField] = "containerName"
				mp[containerNamePrefixField] = "containerNamePrefix"
				req := &csi.CreateVolumeRequest{
					Name:               "unit-test",
					VolumeCapabilities: stdVolumeCapabilities,
					Parameters:         mp,
				}
				d.Cap = []*csi.ControllerServiceCapability{
					controllerServiceCapability,
				}
				_, err := d.CreateVolume(context.Background(), req)
				expectedErr := status.Errorf(codes.InvalidArgument, "containerName(containerName) and containerNamePrefix(containerNamePrefix) could not be specified together")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "invalid containerNamePrefix",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				mp := make(map[string]string)
				mp[containerNamePrefixField] = "UpperCase"
				req := &csi.CreateVolumeRequest{
					Name:               "unit-test",
					VolumeCapabilities: stdVolumeCapabilities,
					Parameters:         mp,
				}
				d.Cap = []*csi.ControllerServiceCapability{
					controllerServiceCapability,
				}
				_, err := d.CreateVolume(context.Background(), req)
				expectedErr := status.Errorf(codes.InvalidArgument, "containerNamePrefix(UpperCase) can only contain lowercase letters, numbers, hyphens, and length should be less than 21")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "tags error",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{}
				mp := make(map[string]string)
				mp[tagsField] = "unit-test"
				mp[storageAccountTypeField] = "premium"
				mp[mountPermissionsField] = "0700"
				req := &csi.CreateVolumeRequest{
					Name:               "unit-test",
					VolumeCapabilities: stdVolumeCapabilities,
					Parameters:         mp,
				}
				d.Cap = []*csi.ControllerServiceCapability{
					controllerServiceCapability,
				}
				_, err := d.CreateVolume(context.Background(), req)
				expectedErr := status.Errorf(codes.InvalidArgument, "Tags 'unit-test' are invalid, the format should like: 'key1=value1,key2=value2'")
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
				mp[resourceGroupField] = "rg"
				mp[containerNameField] = "unit-test"
				mp[mountPermissionsField] = "0755"
				req := &csi.CreateVolumeRequest{
					Name:               "unit-test",
					VolumeCapabilities: stdVolumeCapabilities,
				}
				d.Cap = []*csi.ControllerServiceCapability{
					controllerServiceCapability,
				}

				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				mockStorageAccountsClient := mock_accountclient.NewMockInterface(ctrl)
				d.cloud.ComputeClientFactory = mock_azclient.NewMockClientFactory(ctrl)
				rerr := fmt.Errorf("test")
				mockStorageAccountsClient.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, rerr).AnyTimes()
				d.cloud.ComputeClientFactory.(*mock_azclient.MockClientFactory).EXPECT().GetAccountClientForSub(gomock.Any()).Return(mockStorageAccountsClient, nil).AnyTimes()
				_, err := d.CreateVolume(context.Background(), req)
				expectedErr := status.Errorf(codes.Internal, "ensure storage account failed with could not list storage accounts for account type : test")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "invalid parameter",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				mp := make(map[string]string)
				mp[skuNameField] = "unit-test"
				mp[storageAccountTypeField] = "unit-test"
				mp[locationField] = "unit-test"
				mp[storageAccountField] = "unit-test"
				mp[resourceGroupField] = "unit-test"
				mp["containername"] = "unit-test"
				mp["invalidparameter"] = "invalidvalue"
				req := &csi.CreateVolumeRequest{
					Name:               "unit-test",
					VolumeCapabilities: stdVolumeCapabilities,
					Parameters:         mp,
				}
				d.Cap = []*csi.ControllerServiceCapability{
					controllerServiceCapability,
				}

				expectedErr := status.Errorf(codes.InvalidArgument, "invalid parameter %q in storage class", "invalidparameter")
				_, err := d.CreateVolume(context.Background(), req)
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("Unexpected error: %v", err)
				}
			},
		},
		{
			name: "invalid mountPermissions",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				mp := make(map[string]string)
				mp[mountPermissionsField] = "0abc"
				req := &csi.CreateVolumeRequest{
					Name:               "unit-test",
					VolumeCapabilities: stdVolumeCapabilities,
					Parameters:         mp,
				}
				d.Cap = []*csi.ControllerServiceCapability{
					controllerServiceCapability,
				}

				expectedErr := status.Errorf(codes.InvalidArgument, "invalid %s %s in storage class", "mountPermissions", "0abc")
				_, err := d.CreateVolume(context.Background(), req)
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("Unexpected error: %v", err)
				}
			},
		},
		{
			name: "invalid privateEndpoint and subnetName combination",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				mp := map[string]string{
					networkEndpointTypeField: "privateendpoint",
					subnetNameField:          "subnet1,subnet2",
				}
				req := &csi.CreateVolumeRequest{
					Name:               "unit-test",
					VolumeCapabilities: stdVolumeCapabilities,
					Parameters:         mp,
				}
				d.Cap = []*csi.ControllerServiceCapability{
					controllerServiceCapability,
				}

				expectedErr := status.Errorf(codes.InvalidArgument, "subnetName(subnet1,subnet2) can only contain one subnet for private endpoint")
				_, err := d.CreateVolume(context.Background(), req)
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("Unexpected error: %v", err)
				}
			},
		},
		{
			name: "Update service endpoints failed (protocol = nfs)",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud.SubscriptionID = "subID"
				mp := make(map[string]string)
				mp[storeAccountKeyField] = falseValue
				mp[protocolField] = "nfs"
				mp[skuNameField] = "unit-test"
				mp[storageAccountTypeField] = "unit-test"
				mp[locationField] = "unit-test"
				mp[storageAccountField] = "unit-test"
				mp[resourceGroupField] = "unit-test"
				mp[containerNameField] = "unit-test"
				mp[mountPermissionsField] = "0750"
				mp[storageAuthTypeField] = "msi"
				mp[storageIdentityClientIDField] = "msi"
				mp[storageIdentityObjectIDField] = "msi"
				mp[storageIdentityResourceIDField] = "msi"
				mp[msiEndpointField] = "msi"
				mp[storageAADEndpointField] = "msi"
				req := &csi.CreateVolumeRequest{
					Name:               "unit-test",
					VolumeCapabilities: stdVolumeCapabilities,
					Parameters:         mp,
				}
				d.Cap = []*csi.ControllerServiceCapability{
					controllerServiceCapability,
				}

				expectedErr := status.Errorf(codes.Internal, "update service endpoints failed with error: %v", fmt.Errorf("networkClientFactory is nil"))
				_, err := d.CreateVolume(context.Background(), req)
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("Unexpected error: %v", err)
				}
			},
		},
		{
			name: "Azure Stack only supports Storage Account types : (Premium_LRS) and (Standard_LRS)",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{}
				d.cloud.DisableAzureStackCloud = false
				d.cloud.Cloud = "AZURESTACKCLOUD"
				d.cloud.SubscriptionID = "subID"
				mp := make(map[string]string)
				mp[storeAccountKeyField] = falseValue
				mp[protocolField] = "fuse"
				mp[skuNameField] = "unit-test"
				mp[storageAccountTypeField] = "unit-test"
				mp[locationField] = "unit-test"
				mp[storageAccountField] = "unit-test"
				mp[resourceGroupField] = "unit-test"
				mp[containerNameField] = "unit-test"
				mp[mountPermissionsField] = "0750"
				req := &csi.CreateVolumeRequest{
					Name:               "unit-test",
					VolumeCapabilities: stdVolumeCapabilities,
					Parameters:         mp,
				}
				d.Cap = []*csi.ControllerServiceCapability{
					controllerServiceCapability,
				}

				expectedErr := status.Errorf(codes.InvalidArgument, "Invalid skuName value: %s, as Azure Stack only supports %s and %s Storage Account types.", "unit-test", armstorage.SKUNamePremiumLRS, armstorage.SKUNameStandardLRS)
				_, err := d.CreateVolume(context.Background(), req)
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("Unexpected error: %v", err)
				}
			},
		},
		{
			name: "Failed to get storage access key (Dataplane API)",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{}
				d.cloud.SubscriptionID = "subID"
				mp := make(map[string]string)
				mp[useDataPlaneAPIField] = trueValue
				mp[protocolField] = "fuse"
				mp[skuNameField] = "unit-test"
				mp[storageAccountTypeField] = "unit-test"
				mp[locationField] = "unit-test"
				mp[storageAccountField] = "unit-test"
				mp[resourceGroupField] = "unit-test"
				mp[containerNameField] = "unit-test"
				mp[mountPermissionsField] = "0750"

				keyList := make([]*armstorage.AccountKey, 0)
				mockStorageAccountsClient := NewMockSAClient(context.Background(), gomock.NewController(t), "subID", "unit-test", "unit-test", keyList)
				d.cloud.ComputeClientFactory = mock_azclient.NewMockClientFactory(gomock.NewController(t))
				d.cloud.ComputeClientFactory.(*mock_azclient.MockClientFactory).EXPECT().GetAccountClient().Return(mockStorageAccountsClient).AnyTimes()
				d.cloud.ComputeClientFactory.(*mock_azclient.MockClientFactory).EXPECT().GetAccountClientForSub(gomock.Any()).Return(mockStorageAccountsClient, nil).AnyTimes()

				req := &csi.CreateVolumeRequest{
					Name:               "unit-test",
					VolumeCapabilities: stdVolumeCapabilities,
					Parameters:         mp,
				}
				d.Cap = []*csi.ControllerServiceCapability{
					controllerServiceCapability,
				}

				expectedErr := status.Errorf(codes.Internal, "failed to GetStorageAccesskey on account(%s) rg(%s), error: %v", "unit-test", "unit-test", fmt.Errorf("empty keys"))
				_, err := d.CreateVolume(context.Background(), req)
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("Unexpected error: %v", err)
				}
			},
		},
		{
			name: "Failed to Create Blob Container",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{}
				d.cloud.SubscriptionID = "subID"

				keyList := make([]*armstorage.AccountKey, 1)
				fakeKey := "fakeKey"
				fakeValue := "fakeValue"
				keyList[0] = (&armstorage.AccountKey{
					KeyName: &fakeKey,
					Value:   &fakeValue,
				})
				NewMockSAClient(context.Background(), gomock.NewController(t), "subID", "unit-test", "unit-test", keyList)
				controller := gomock.NewController(t)

				clientFactoryMock := mock_azclient.NewMockClientFactory(controller)
				blobClientMock := mock_blobcontainerclient.NewMockInterface(controller)
				blobClientMock.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("timed out waiting for the condition"))
				clientFactoryMock.EXPECT().GetBlobContainerClientForSub(gomock.Any()).Return(blobClientMock, nil)
				d.clientFactory = clientFactoryMock

				mp := make(map[string]string)
				mp[protocolField] = "fuse"
				mp[skuNameField] = "unit-test"
				mp[storageAccountTypeField] = "unit-test"
				mp[locationField] = "unit-test"
				mp[storageAccountField] = "unittest"
				mp[resourceGroupField] = "unit-test"
				mp[containerNameField] = "unit-test"
				mp[mountPermissionsField] = "0750"
				req := &csi.CreateVolumeRequest{
					Name:               "unit-test",
					VolumeCapabilities: stdVolumeCapabilities,
					Parameters:         mp,
				}
				d.Cap = []*csi.ControllerServiceCapability{
					controllerServiceCapability,
				}

				e := fmt.Errorf("timed out waiting for the condition")
				expectedErr := status.Errorf(codes.Internal, "failed to create container(%s) on account(%s) type(%s) rg(%s) location(%s) size(%d), error: %v", "unit-test", mp[storageAccountField], "unit-test", "unit-test", "unit-test", 0, e)
				_, err := d.CreateVolume(context.Background(), req)
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("Unexpected error: %v", err)
				}
				controller.Finish()
			},
		},
		{
			name: "Failed to get storage access key",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{}
				d.cloud.SubscriptionID = "subID"
				controller := gomock.NewController(t)
				var mockKeyList []*armstorage.AccountKey
				mockStorageAccountsClient := NewMockSAClient(context.Background(), gomock.NewController(t), "subID", "unit-test", "unit-test", mockKeyList)
				d.cloud.ComputeClientFactory = mock_azclient.NewMockClientFactory(gomock.NewController(t))
				d.cloud.ComputeClientFactory.(*mock_azclient.MockClientFactory).EXPECT().GetAccountClient().Return(mockStorageAccountsClient).AnyTimes()
				d.cloud.ComputeClientFactory.(*mock_azclient.MockClientFactory).EXPECT().GetAccountClientForSub(gomock.Any()).Return(mockStorageAccountsClient, nil).AnyTimes()
				clientFactoryMock := mock_azclient.NewMockClientFactory(controller)
				blobClientMock := mock_blobcontainerclient.NewMockInterface(controller)
				blobClientMock.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil)
				clientFactoryMock.EXPECT().GetBlobContainerClientForSub(gomock.Any()).Return(blobClientMock, nil)
				d.clientFactory = clientFactoryMock

				mp := make(map[string]string)
				mp[storeAccountKeyField] = trueValue
				mp[protocolField] = "fuse"
				mp[skuNameField] = "unit-test"
				mp[storageAccountTypeField] = "unit-test"
				mp[locationField] = "unit-test"
				mp[storageAccountField] = "unittest"
				mp[resourceGroupField] = "unit-test"
				mp[containerNameField] = "unit-test"
				mp[mountPermissionsField] = "0750"
				req := &csi.CreateVolumeRequest{
					Name:               "unit-test",
					VolumeCapabilities: stdVolumeCapabilities,
					Parameters:         mp,
				}
				d.Cap = []*csi.ControllerServiceCapability{
					controllerServiceCapability,
				}

				expectedErr := status.Errorf(
					codes.Internal, "failed to GetStorageAccesskey on account(%s) rg(%s), error: %v", mp[storageAccountField], mp[resourceGroupField], fmt.Errorf("empty keys"))
				_, err := d.CreateVolume(context.Background(), req)
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("Unexpected error: %v\nExpected error: %v", err, expectedErr)
				}
				controller.Finish()
			},
		},
		{
			name: "Failed with invalid allowSharedKeyAccess value",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{}
				d.cloud.SubscriptionID = "subID"

				mp := make(map[string]string)
				mp[allowSharedKeyAccessField] = "invalid"
				req := &csi.CreateVolumeRequest{
					Name:               "unit-test",
					VolumeCapabilities: stdVolumeCapabilities,
					Parameters:         mp,
				}
				d.Cap = []*csi.ControllerServiceCapability{
					controllerServiceCapability,
				}

				expectedErr := status.Errorf(codes.InvalidArgument, "invalid %s: invalid in volume context", allowSharedKeyAccessField)
				_, err := d.CreateVolume(context.Background(), req)
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("Unexpected error: %v\nExpected error: %v", err, expectedErr)
				}
			},
		},
		{
			name: "Failed with storeAccountKey is not supported for account with shared access key disabled",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{}
				d.cloud.SubscriptionID = "subID"

				mp := make(map[string]string)
				mp[protocolField] = "fuse"
				mp[skuNameField] = "unit-test"
				mp[storageAccountTypeField] = "unit-test"
				mp[locationField] = "unit-test"
				mp[storageAccountField] = "unittest"
				mp[resourceGroupField] = "unit-test"
				mp[containerNameField] = "unit-test"
				mp[mountPermissionsField] = "0750"
				mp[allowSharedKeyAccessField] = falseValue
				req := &csi.CreateVolumeRequest{
					Name:               "unit-test",
					VolumeCapabilities: stdVolumeCapabilities,
					Parameters:         mp,
				}
				d.Cap = []*csi.ControllerServiceCapability{
					controllerServiceCapability,
				}

				expectedErr := status.Errorf(codes.InvalidArgument, "storeAccountKey is not supported for account with shared access key disabled")
				_, err := d.CreateVolume(context.Background(), req)
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("Unexpected error: %v\nExpected error: %v", err, expectedErr)
				}
			},
		},
		{
			name: "Successful I/O",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{}
				d.cloud.SubscriptionID = "subID"

				keyList := make([]*armstorage.AccountKey, 1)
				fakeKey := "fakeKey"
				fakeValue := "fakeValue"
				keyList[0] = (&armstorage.AccountKey{
					KeyName: &fakeKey,
					Value:   &fakeValue,
				})
				controller := gomock.NewController(t)

				mockStorageAccountsClient := NewMockSAClient(context.Background(), gomock.NewController(t), "subID", "unit-test", "unit-test", keyList)
				d.cloud.ComputeClientFactory = mock_azclient.NewMockClientFactory(gomock.NewController(t))
				d.cloud.ComputeClientFactory.(*mock_azclient.MockClientFactory).EXPECT().GetAccountClient().Return(mockStorageAccountsClient).AnyTimes()
				d.cloud.ComputeClientFactory.(*mock_azclient.MockClientFactory).EXPECT().GetAccountClientForSub(gomock.Any()).Return(mockStorageAccountsClient, nil).AnyTimes()

				clientFactoryMock := mock_azclient.NewMockClientFactory(controller)
				blobClientMock := mock_blobcontainerclient.NewMockInterface(controller)
				blobClientMock.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil)
				clientFactoryMock.EXPECT().GetBlobContainerClientForSub(gomock.Any()).Return(blobClientMock, nil)
				d.clientFactory = clientFactoryMock

				mp := make(map[string]string)
				mp[protocolField] = "fuse"
				mp[skuNameField] = "unit-test"
				mp[storageAccountTypeField] = "unit-test"
				mp[locationField] = "unit-test"
				mp[storageAccountField] = "unittest"
				mp[resourceGroupField] = "unit-test"
				mp[containerNameField] = "unit-test"
				mp[mountPermissionsField] = "0750"
				req := &csi.CreateVolumeRequest{
					Name:               "unit-test",
					VolumeCapabilities: stdVolumeCapabilities,
					Parameters:         mp,
				}
				d.Cap = []*csi.ControllerServiceCapability{
					controllerServiceCapability,
				}

				var expectedErr error
				_, err := d.CreateVolume(context.Background(), req)
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("Unexpected error: %v", err)
				}
				controller.Finish()
			},
		},
		//nolint:dupl
		{
			name: "create volume from copy volumesnapshot is not supported",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{}
				d.cloud.SubscriptionID = "subID"

				keyList := make([]*armstorage.AccountKey, 1)
				fakeKey := "fakeKey"
				fakeValue := "fakeValue"
				keyList[0] = (&armstorage.AccountKey{
					KeyName: &fakeKey,
					Value:   &fakeValue,
				})
				NewMockSAClient(context.Background(), gomock.NewController(t), "subID", "unit-test", "unit-test", keyList)
				d.cloud.Config.AzureAuthConfig.UseManagedIdentityExtension = true

				d.clientFactory = mock_azclient.NewMockClientFactory(gomock.NewController(t))

				mp := make(map[string]string)
				mp[protocolField] = "fuse"
				mp[skuNameField] = "unit-test"
				mp[storageAccountTypeField] = "unit-test"
				mp[locationField] = "unit-test"
				mp[storageAccountField] = "unittest"
				mp[resourceGroupField] = "unit-test"
				mp[containerNameField] = "unit-test"
				mp[mountPermissionsField] = "0750"

				volumeSnapshotSource := &csi.VolumeContentSource_SnapshotSource{
					SnapshotId: "unit-test",
				}
				volumeContentSourceSnapshotSource := &csi.VolumeContentSource_Snapshot{
					Snapshot: volumeSnapshotSource,
				}
				volumecontensource := csi.VolumeContentSource{
					Type: volumeContentSourceSnapshotSource,
				}
				req := &csi.CreateVolumeRequest{
					Name:                "unit-test",
					VolumeCapabilities:  stdVolumeCapabilities,
					Parameters:          mp,
					VolumeContentSource: &volumecontensource,
				}
				d.Cap = []*csi.ControllerServiceCapability{
					controllerServiceCapability,
				}

				ctrl := gomock.NewController(t)
				defer ctrl.Finish()

				m := util.NewMockEXEC(ctrl)
				d.azcopy.ExecCmd = m

				expectedErr := status.Errorf(codes.InvalidArgument, "VolumeContentSource Snapshot is not yet implemented")
				_, err := d.CreateVolume(context.Background(), req)
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("Unexpected error: %v", err)
				}
			},
		},
		//nolint:dupl
		{
			name: "create volume from copy volume not found",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{}
				d.cloud.SubscriptionID = "subID"

				keyList := make([]*armstorage.AccountKey, 1)
				fakeKey := "fakeKey"
				fakeValue := "fakeValue"
				keyList[0] = (&armstorage.AccountKey{
					KeyName: &fakeKey,
					Value:   &fakeValue,
				})
				NewMockSAClient(context.Background(), gomock.NewController(t), "subID", "unit-test", "unit-test", keyList)
				d.cloud.Config.AzureAuthConfig.UseManagedIdentityExtension = true

				mp := make(map[string]string)
				mp[protocolField] = "fuse"
				mp[skuNameField] = "unit-test"
				mp[storageAccountTypeField] = "unit-test"
				mp[locationField] = "unit-test"
				mp[storageAccountField] = "unittest"
				mp[resourceGroupField] = "unit-test"
				mp[containerNameField] = "unit-test"
				mp[mountPermissionsField] = "0750"

				volumeSource := &csi.VolumeContentSource_VolumeSource{
					VolumeId: "unit-test",
				}
				volumeContentSourceVolumeSource := &csi.VolumeContentSource_Volume{
					Volume: volumeSource,
				}
				volumecontensource := csi.VolumeContentSource{
					Type: volumeContentSourceVolumeSource,
				}
				req := &csi.CreateVolumeRequest{
					Name:                "unit-test",
					VolumeCapabilities:  stdVolumeCapabilities,
					Parameters:          mp,
					VolumeContentSource: &volumecontensource,
				}
				d.Cap = []*csi.ControllerServiceCapability{
					controllerServiceCapability,
				}

				ctrl := gomock.NewController(t)
				defer ctrl.Finish()

				clientFactoryMock := mock_azclient.NewMockClientFactory(ctrl)
				blobClientMock := mock_blobcontainerclient.NewMockInterface(ctrl)
				blobClientMock.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil)
				clientFactoryMock.EXPECT().GetBlobContainerClientForSub(gomock.Any()).Return(blobClientMock, nil)
				d.clientFactory = clientFactoryMock

				expectedErr := status.Errorf(codes.NotFound, "error parsing volume id: \"unit-test\", should at least contain two #")
				_, err := d.CreateVolume(context.Background(), req)
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("Unexpected error: %v", err)
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
			name: "invalid volume Id",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{}
				d.Cap = []*csi.ControllerServiceCapability{
					controllerServiceCapability,
				}
				req := &csi.DeleteVolumeRequest{
					VolumeId: "unit-test",
				}
				_, err := d.DeleteVolume(context.Background(), req)
				expectedErr := error(nil)
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: %v, expectedErr:(%v", err, expectedErr)
				}
			},
		},
		{
			name: "GetAuthEnv() Failed (useDataPlaneAPI)",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{}
				keyList := make([]*armstorage.AccountKey, 0)
				mockStorageAccountsClient := NewMockSAClient(context.Background(), gomock.NewController(t), "subID", "unit-test", "unit-test", keyList)
				d.cloud.ComputeClientFactory = mock_azclient.NewMockClientFactory(gomock.NewController(t))
				d.cloud.ComputeClientFactory.(*mock_azclient.MockClientFactory).EXPECT().GetAccountClient().Return(mockStorageAccountsClient).AnyTimes()
				d.cloud.ComputeClientFactory.(*mock_azclient.MockClientFactory).EXPECT().GetAccountClientForSub(gomock.Any()).Return(mockStorageAccountsClient, nil).AnyTimes()
				d.Cap = []*csi.ControllerServiceCapability{
					controllerServiceCapability,
				}
				req := &csi.DeleteVolumeRequest{
					VolumeId: "rg#accountName#containerName",
				}
				d.dataPlaneAPIVolCache.Set("rg#accountName#containerName", "accountName")
				_, err := d.DeleteVolume(context.Background(), req)
				expectedErr := status.Errorf(codes.Internal, "GetAuthEnv(%s) failed with %v", "rg#accountName#containerName", fmt.Errorf("no key for storage account(%s) under resource group(%s), err %w", "accountName", "rg", fmt.Errorf("empty keys")))
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("\nactualErr: %v, \nexpectedErr:(%v", err, expectedErr)
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
					Secrets: map[string]string{
						defaultSecretAccountName: "accountname",
						defaultSecretAccountKey:  "b",
					},
				}
				d.cloud = &storage.AccountRepo{}
				d.cloud.ResourceGroup = "unit"
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				mockStorageAccountsClient := mock_accountclient.NewMockInterface(ctrl)
				d.cloud.ComputeClientFactory = mock_azclient.NewMockClientFactory(gomock.NewController(t))
				d.cloud.ComputeClientFactory.(*mock_azclient.MockClientFactory).EXPECT().GetAccountClient().Return(mockStorageAccountsClient).AnyTimes()
				d.cloud.ComputeClientFactory.(*mock_azclient.MockClientFactory).EXPECT().GetAccountClientForSub(gomock.Any()).Return(mockStorageAccountsClient, nil).AnyTimes()
				rerr := fmt.Errorf("test")
				accountListKeysResult := []*armstorage.AccountKey{}
				mockStorageAccountsClient.EXPECT().ListKeys(gomock.Any(), gomock.Any(), gomock.Any()).Return(accountListKeysResult, rerr).AnyTimes()
				expectedErr := fmt.Errorf("azure: malformed storage account key: illegal base64 data at input byte 0")
				_, err := d.DeleteVolume(context.Background(), req)
				if !strings.EqualFold(err.Error(), expectedErr.Error()) && !strings.Contains(err.Error(), expectedErr.Error()) {
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
					Secrets: map[string]string{
						defaultSecretAccountName: "accountname",
						defaultSecretAccountKey:  "b",
					},
				}
				d.cloud = &storage.AccountRepo{}
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				mockStorageAccountsClient := mock_accountclient.NewMockInterface(ctrl)
				s := "unit-test"
				accountkey := &armstorage.AccountKey{
					Value: &s,
				}
				accountkeylist := []*armstorage.AccountKey{}
				accountkeylist = append(accountkeylist, accountkey)
				mockStorageAccountsClient.EXPECT().ListKeys(gomock.Any(), gomock.Any(), gomock.Any()).Return(accountkeylist, nil).AnyTimes()
				expectedErr := fmt.Errorf("azure: malformed storage account key: illegal base64 data at input byte 0")
				_, err := d.DeleteVolume(context.Background(), req)
				if !reflect.DeepEqual(err, expectedErr) && !strings.Contains(err.Error(), expectedErr.Error()) {
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
	blockVolumeCapability := &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Block{
			Block: &csi.VolumeCapability_BlockVolume{},
		},
	}
	blockVolumeCapabilities := []*csi.VolumeCapability{
		blockVolumeCapability,
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
		name          string
		req           *csi.ValidateVolumeCapabilitiesRequest
		clientErr     errType
		containerProp *armstorage.ContainerProperties
		expectedRes   *csi.ValidateVolumeCapabilitiesResponse
		expectedErr   error
	}{
		{
			name:          "volume ID missing",
			req:           &csi.ValidateVolumeCapabilitiesRequest{},
			clientErr:     NULL,
			containerProp: nil,
			expectedRes:   nil,
			expectedErr:   status.Error(codes.InvalidArgument, "Volume ID missing in request"),
		},
		{
			name: "volume capability missing",
			req: &csi.ValidateVolumeCapabilitiesRequest{
				VolumeId: "unit-test",
			},
			clientErr:     NULL,
			containerProp: nil,
			expectedRes:   nil,
			expectedErr:   status.Error(codes.InvalidArgument, "volume capabilities missing in request"),
		},
		{
			name: "block volume capability not supported",
			req: &csi.ValidateVolumeCapabilitiesRequest{
				VolumeId:           "unit-test",
				VolumeCapabilities: blockVolumeCapabilities,
			},
			clientErr:     NULL,
			containerProp: nil,
			expectedRes:   nil,
			expectedErr:   status.Error(codes.InvalidArgument, "block volume capability not supported"),
		},
		{
			name: "invalid volume id",
			req: &csi.ValidateVolumeCapabilitiesRequest{
				VolumeId:           "unit-test",
				VolumeCapabilities: stdVolumeCapabilities,
			},
			clientErr:     NULL,
			containerProp: nil,
			expectedRes:   nil,
			expectedErr:   status.Error(codes.NotFound, "error parsing volume id: \"unit-test\", should at least contain two #"),
		},
		{
			name: "base storage service url empty",
			req: &csi.ValidateVolumeCapabilitiesRequest{
				VolumeId:           "unit#test#test",
				VolumeCapabilities: stdVolumeCapabilities,
				Secrets: map[string]string{
					defaultSecretAccountName: "accountname",
					defaultSecretAccountKey:  "b",
				},
			},
			clientErr:     NULL,
			containerProp: nil,
			expectedRes:   nil,
			expectedErr:   status.Error(codes.Internal, "azure: malformed storage account key: illegal base64 data at input byte 0"),
		},
		{
			name: "ContainerProperties of volume is nil",
			req: &csi.ValidateVolumeCapabilitiesRequest{
				VolumeId:           "unit#test#test",
				VolumeCapabilities: stdVolumeCapabilities,
				Secrets:            map[string]string{},
			},
			clientErr:     NULL,
			containerProp: nil,
			expectedRes:   nil,
			expectedErr:   status.Error(codes.Internal, fmt.Sprintf("ContainerProperties of volume(%s) is nil", "unit#test#test")),
		},
		{
			name: "Client Error",
			req: &csi.ValidateVolumeCapabilitiesRequest{
				VolumeId:           "unit#test#test",
				VolumeCapabilities: stdVolumeCapabilities,
				Secrets:            map[string]string{},
			},
			clientErr:     DATAPLANE,
			containerProp: &armstorage.ContainerProperties{},
			expectedRes:   nil,
			expectedErr:   status.Errorf(codes.Internal, "%v", containerBeingDeletedDataplaneAPIError),
		},
		{
			name: "Requested Volume does not exist",
			req: &csi.ValidateVolumeCapabilitiesRequest{
				VolumeId:           "unit#test#test",
				VolumeCapabilities: stdVolumeCapabilities,
				Secrets:            map[string]string{},
			},
			clientErr:     NULL,
			containerProp: &armstorage.ContainerProperties{},
			expectedRes:   nil,
			expectedErr:   status.Errorf(codes.NotFound, "requested volume(%s) does not exist", "unit#test#test"),
		},
		/*{ //Volume being shown as not existing. ContainerProperties.Deleted not setting correctly??
			name: "Successful I/O",
			req: &csi.ValidateVolumeCapabilitiesRequest{
				VolumeId:           "unit#test#test",
				VolumeCapabilities: stdVolumeCapabilities,
				Secrets:            map[string]string{},
			},
			clientErr:     NULL,
			containerProp: &storage.ContainerProperties{Deleted: &[]bool{false}[0]},
			expectedRes: &csi.ValidateVolumeCapabilitiesResponse{
				Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
					VolumeCapabilities: stdVolumeCapabilities,
				},
				Message: "",
			},
			expectedErr: nil,
		},*/
	}

	for _, test := range testCases {
		d := NewFakeDriver()
		d.cloud = &storage.AccountRepo{}
		d.Cap = []*csi.ControllerServiceCapability{
			controllerServiceCapability,
		}
		ctrl := gomock.NewController(t)

		mockStorageAccountsClient := mock_accountclient.NewMockInterface(ctrl)
		s := "unit-test"
		accountkey := &armstorage.AccountKey{
			Value: &s,
		}
		accountkeylist := []*armstorage.AccountKey{}
		accountkeylist = append(accountkeylist, accountkey)
		mockStorageAccountsClient.EXPECT().ListKeys(gomock.Any(), gomock.Any(), gomock.Any()).Return(accountkeylist, nil).AnyTimes()

		clientFactoryMock := mock_azclient.NewMockClientFactory(ctrl)
		blobClientMock := mock_blobcontainerclient.NewMockInterface(ctrl)
		clientFactoryMock.EXPECT().GetBlobContainerClientForSub(gomock.Any()).Return(blobClientMock, nil).AnyTimes()
		blobClientMock.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, _ string, _ string, _ string) (result *armstorage.BlobContainer, _ error) {
				switch test.clientErr {
				case DATAPLANE:
					return nil, fmt.Errorf(containerBeingDeletedDataplaneAPIError)
				case MANAGEMENT:
					return nil, fmt.Errorf(containerBeingDeletedManagementAPIError)
				}
				return &armstorage.BlobContainer{
					ContainerProperties: test.containerProp,
				}, nil
			}).AnyTimes()
		d.clientFactory = clientFactoryMock
		res, err := d.ValidateVolumeCapabilities(context.Background(), test.req)
		assert.Equal(t, test.expectedErr, err, "Error in testcase (%s): Errors must match", test.name)
		assert.Equal(t, test.expectedRes, res, "Error in testcase (%s): Response must match", test.name)
		ctrl.Finish()
	}
}

func TestControllerGetVolume(t *testing.T) {
	d := NewFakeDriver()
	req := csi.ControllerGetVolumeRequest{}
	resp, err := d.ControllerGetVolume(context.Background(), &req)
	assert.Nil(t, resp)
	if !reflect.DeepEqual(err, status.Error(codes.Unimplemented, "ControllerGetVolume is not yet implemented")) {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestGetCapacity(t *testing.T) {
	d := NewFakeDriver()
	req := csi.GetCapacityRequest{}
	resp, err := d.GetCapacity(context.Background(), &req)
	assert.Nil(t, resp)
	if !reflect.DeepEqual(err, status.Error(codes.Unimplemented, "GetCapacity is not yet implemented")) {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestListVolumes(t *testing.T) {
	d := NewFakeDriver()
	req := csi.ListVolumesRequest{}
	resp, err := d.ListVolumes(context.Background(), &req)
	assert.Nil(t, resp)
	if !reflect.DeepEqual(err, status.Error(codes.Unimplemented, "ListVolumes is not yet implemented")) {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestControllerPublishVolume(t *testing.T) {
	d := NewFakeDriver()
	req := csi.ControllerPublishVolumeRequest{}
	resp, err := d.ControllerPublishVolume(context.Background(), &req)
	assert.Nil(t, resp)
	if !reflect.DeepEqual(err, status.Error(codes.Unimplemented, "ControllerPublishVolume is not yet implemented")) {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestControllerUnpublishVolume(t *testing.T) {
	d := NewFakeDriver()
	req := csi.ControllerUnpublishVolumeRequest{}
	resp, err := d.ControllerUnpublishVolume(context.Background(), &req)
	assert.Nil(t, resp)
	if !reflect.DeepEqual(err, status.Error(codes.Unimplemented, "ControllerUnpublishVolume is not yet implemented")) {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestCreateSnapshots(t *testing.T) {
	d := NewFakeDriver()
	req := csi.CreateSnapshotRequest{}
	resp, err := d.CreateSnapshot(context.Background(), &req)
	assert.Nil(t, resp)
	if !reflect.DeepEqual(err, status.Error(codes.Unimplemented, "CreateSnapshot is not yet implemented")) {
		t.Errorf("Unexpected error: %v", err)
	}
}
func TestDeleteSnapshots(t *testing.T) {
	d := NewFakeDriver()
	req := csi.DeleteSnapshotRequest{}
	resp, err := d.DeleteSnapshot(context.Background(), &req)
	assert.Nil(t, resp)
	if !reflect.DeepEqual(err, status.Error(codes.Unimplemented, "DeleteSnapshot is not yet implemented")) {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestListSnapshots(t *testing.T) {
	d := NewFakeDriver()
	req := csi.ListSnapshotsRequest{}
	resp, err := d.ListSnapshots(context.Background(), &req)
	assert.Nil(t, resp)
	if !reflect.DeepEqual(err, status.Error(codes.Unimplemented, "ListSnapshots is not yet implemented")) {
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
			name: "Volume Size exceeds Max Container Size",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{csi.ControllerServiceCapability_RPC_EXPAND_VOLUME})
				req := &csi.ControllerExpandVolumeRequest{
					VolumeId: "unit-test",
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: containerMaxSize + 1,
					},
				}
				_, err := d.ControllerExpandVolume(context.Background(), req)
				expectedErr := status.Errorf(codes.OutOfRange, "required bytes (%d) exceeds the maximum supported bytes (%d)", req.CapacityRange.RequiredBytes, containerMaxSize)
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "Error = nil",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{csi.ControllerServiceCapability_RPC_EXPAND_VOLUME})
				req := &csi.ControllerExpandVolumeRequest{
					VolumeId: "unit-test",
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: containerMaxSize,
					},
				}
				_, err := d.ControllerExpandVolume(context.Background(), req)
				if !reflect.DeepEqual(err, nil) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, nil)
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func TestCreateBlobContainer(t *testing.T) {
	tests := []struct {
		desc          string
		subsID        string
		rg            string
		accountName   string
		containerName string
		secrets       map[string]string
		customErrStr  string
		clientErr     errType
		expectedErr   error
	}{
		{
			desc:        "containerName is empty",
			clientErr:   NULL,
			expectedErr: fmt.Errorf("containerName is empty"),
		},
		{
			desc:          "azure: malformed storage account key: illegal base64 data at input byte 0",
			containerName: "containerName",
			secrets: map[string]string{
				defaultSecretAccountName: "accountname",
				defaultSecretAccountKey:  "key",
			},
			clientErr:   NULL,
			expectedErr: fmt.Errorf("azure: malformed storage account key: illegal base64 data at input byte 0"),
		},
		{
			desc:          "Secrets is Empty",
			containerName: "containerName",
			secrets:       map[string]string{},
			clientErr:     NULL,
			expectedErr:   nil,
		},
		{
			desc:          "Dataplane API Error",
			containerName: "containerName",
			secrets:       map[string]string{},
			clientErr:     DATAPLANE,
			expectedErr:   wait.ErrWaitTimeout,
		},
		{
			desc:          "Management API Error",
			containerName: "containerName",
			secrets:       map[string]string{},
			clientErr:     MANAGEMENT,
			expectedErr:   wait.ErrWaitTimeout,
		},
		{
			desc:          "Random Client Error",
			containerName: "containerName",
			secrets:       map[string]string{},
			clientErr:     CUSTOM,
			customErrStr:  "foobar",
			expectedErr:   fmt.Errorf("foobar"),
		},
	}

	d := NewFakeDriver()
	d.cloud = &storage.AccountRepo{}
	for _, test := range tests {
		controller := gomock.NewController(t)
		clientFactoryMock := mock_azclient.NewMockClientFactory(controller)
		blobClientMock := mock_blobcontainerclient.NewMockInterface(controller)
		accountClientMock := mock_accountclient.NewMockInterface(controller)
		clientFactoryMock.EXPECT().GetAccountClientForSub(gomock.Any()).Return(accountClientMock, nil).AnyTimes()
		clientFactoryMock.EXPECT().GetBlobContainerClientForSub(gomock.Any()).Return(blobClientMock, nil).AnyTimes()
		d.clientFactory = clientFactoryMock
		blobClientMock.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, _, _, _ string, _ armstorage.BlobContainer) (*armstorage.BlobContainer, error) {
				if test.clientErr == DATAPLANE {
					return nil, fmt.Errorf(containerBeingDeletedDataplaneAPIError)
				}
				if test.clientErr == MANAGEMENT {
					return nil, fmt.Errorf(containerBeingDeletedManagementAPIError)
				}
				if test.clientErr == CUSTOM {
					return nil, fmt.Errorf("%v", test.customErrStr)
				}
				return nil, nil
			}).AnyTimes()
		err := d.CreateBlobContainer(context.Background(), test.subsID, test.rg, test.accountName, test.containerName, "core.windows.net", test.secrets)
		if !reflect.DeepEqual(err, test.expectedErr) {
			t.Errorf("test(%s), actualErr: (%v), expectedErr: (%v)", test.desc, err, test.expectedErr)
		}
		controller.Finish()
	}
}

func TestDeleteBlobContainer(t *testing.T) {
	tests := []struct {
		desc          string
		subsID        string
		rg            string
		accountName   string
		containerName string
		secrets       map[string]string
		clientErr     errType
		customErrStr  string
		expectedErr   error
	}{
		{
			desc:        "containerName is empty",
			clientErr:   NULL,
			expectedErr: fmt.Errorf("containerName is empty"),
		},
		{
			desc:          "azure: malformed storage account key: illegal base64 data at input byte 0",
			containerName: "containerName",
			secrets: map[string]string{
				defaultSecretAccountName: "accountname",
				defaultSecretAccountKey:  "key",
			},
			clientErr:   NULL,
			expectedErr: fmt.Errorf("azure: malformed storage account key: illegal base64 data at input byte 0"),
		},
		{
			desc:          "Secrets is Empty",
			containerName: "containerName",
			secrets:       map[string]string{},
			clientErr:     NULL,
			expectedErr:   nil,
		},
		{
			desc:          "Dataplane API Error",
			containerName: "containerName",
			secrets:       map[string]string{},
			clientErr:     DATAPLANE,
			expectedErr:   nil,
		},
		{
			desc:          "Management API Error",
			containerName: "containerName",
			secrets:       map[string]string{},
			clientErr:     MANAGEMENT,
			expectedErr:   nil,
		},
		{
			desc:          "Random Client Error",
			containerName: "containerName",
			secrets:       map[string]string{},
			clientErr:     CUSTOM,
			customErrStr:  "foobar",
			expectedErr: fmt.Errorf("failed to delete container(%s) on account(%s), error: %w", "containerName", "",
				fmt.Errorf("foobar")),
		},
	}

	d := NewFakeDriver()

	for _, test := range tests {
		controller := gomock.NewController(t)
		clientFactoryMock := mock_azclient.NewMockClientFactory(controller)
		blobClientMock := mock_blobcontainerclient.NewMockInterface(controller)
		accountClientMock := mock_accountclient.NewMockInterface(controller)
		clientFactoryMock.EXPECT().GetAccountClientForSub(gomock.Any()).Return(accountClientMock, nil).AnyTimes()
		clientFactoryMock.EXPECT().GetBlobContainerClientForSub(gomock.Any()).Return(blobClientMock, nil).AnyTimes()
		d.clientFactory = clientFactoryMock
		blobClientMock.EXPECT().DeleteContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, _, _, _ string) error {
				if test.clientErr == DATAPLANE {
					return fmt.Errorf(containerBeingDeletedDataplaneAPIError)
				}
				if test.clientErr == MANAGEMENT {
					return fmt.Errorf(containerBeingDeletedManagementAPIError)
				}
				if test.clientErr == CUSTOM {
					return fmt.Errorf("%v", test.customErrStr)
				}
				return nil
			}).AnyTimes()
		err := d.DeleteBlobContainer(context.Background(), test.subsID, test.rg, test.accountName, test.containerName, test.secrets)
		if !reflect.DeepEqual(err, test.expectedErr) {
			t.Errorf("test(%s), actualErr: (%v), expectedErr: (%v)", test.desc, err, test.expectedErr)
		}
		controller.Finish()
	}
}

func TestCopyVolume(t *testing.T) {
	stdVolumeCapability := &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Mount{
			Mount: &csi.VolumeCapability_MountVolume{},
		},
	}
	stdVolumeCapabilities := []*csi.VolumeCapability{
		stdVolumeCapability,
	}
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "copy volume from volumeSnapshot is not supported",
			testFunc: func(t *testing.T) {
				ctx := context.Background()
				d := NewFakeDriver()
				mp := map[string]string{}

				volumeSnapshotSource := &csi.VolumeContentSource_SnapshotSource{
					SnapshotId: "unit-test",
				}
				volumeContentSourceSnapshotSource := &csi.VolumeContentSource_Snapshot{
					Snapshot: volumeSnapshotSource,
				}
				volumecontensource := csi.VolumeContentSource{
					Type: volumeContentSourceSnapshotSource,
				}
				req := &csi.CreateVolumeRequest{
					Name:                "unit-test",
					VolumeCapabilities:  stdVolumeCapabilities,
					Parameters:          mp,
					VolumeContentSource: &volumecontensource,
				}

				expectedErr := status.Errorf(codes.InvalidArgument, "VolumeContentSource Snapshot is not yet implemented")
				err := d.copyVolume(ctx, req, "", "", nil, "", "", nil, defaultStorageEndPointSuffix)
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("Unexpected error: %v", err)
				}
			},
		},
		{
			name: "copy volume from volume not found",
			testFunc: func(t *testing.T) {
				ctx := context.Background()
				d := NewFakeDriver()
				mp := map[string]string{}

				volumeSource := &csi.VolumeContentSource_VolumeSource{
					VolumeId: "unit-test",
				}
				volumeContentSourceVolumeSource := &csi.VolumeContentSource_Volume{
					Volume: volumeSource,
				}
				volumecontensource := csi.VolumeContentSource{
					Type: volumeContentSourceVolumeSource,
				}

				req := &csi.CreateVolumeRequest{
					Name:                "unit-test",
					VolumeCapabilities:  stdVolumeCapabilities,
					Parameters:          mp,
					VolumeContentSource: &volumecontensource,
				}

				expectedErr := status.Errorf(codes.NotFound, "error parsing volume id: \"unit-test\", should at least contain two #")
				err := d.copyVolume(ctx, req, "", "", nil, "dstContainer", "", nil, defaultStorageEndPointSuffix)
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("Unexpected error: %v", err)
				}
			},
		},
		{
			name: "src blob container is empty",
			testFunc: func(t *testing.T) {
				ctx := context.Background()
				d := NewFakeDriver()
				mp := map[string]string{}

				volumeSource := &csi.VolumeContentSource_VolumeSource{
					VolumeId: "rg#unit-test##",
				}
				volumeContentSourceVolumeSource := &csi.VolumeContentSource_Volume{
					Volume: volumeSource,
				}
				volumecontensource := csi.VolumeContentSource{
					Type: volumeContentSourceVolumeSource,
				}

				req := &csi.CreateVolumeRequest{
					Name:                "unit-test",
					VolumeCapabilities:  stdVolumeCapabilities,
					Parameters:          mp,
					VolumeContentSource: &volumecontensource,
				}

				expectedErr := fmt.Errorf("srcAccountName(unit-test) or srcContainerName() or dstContainerName(dstContainer) is empty")
				err := d.copyVolume(ctx, req, "", "", nil, "dstContainer", "", nil, defaultStorageEndPointSuffix)
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("Unexpected error: %v", err)
				}
			},
		},
		{
			name: "dst blob container is empty",
			testFunc: func(t *testing.T) {
				ctx := context.Background()
				d := NewFakeDriver()
				mp := map[string]string{}

				volumeSource := &csi.VolumeContentSource_VolumeSource{
					VolumeId: "vol_1#f5713de20cde511e8ba4900#fileshare#",
				}
				volumeContentSourceVolumeSource := &csi.VolumeContentSource_Volume{
					Volume: volumeSource,
				}
				volumecontensource := csi.VolumeContentSource{
					Type: volumeContentSourceVolumeSource,
				}

				req := &csi.CreateVolumeRequest{
					Name:                "unit-test",
					VolumeCapabilities:  stdVolumeCapabilities,
					Parameters:          mp,
					VolumeContentSource: &volumecontensource,
				}

				expectedErr := fmt.Errorf("srcAccountName(f5713de20cde511e8ba4900) or srcContainerName(fileshare) or dstContainerName() is empty")
				err := d.copyVolume(ctx, req, "", "", nil, "", "", nil, defaultStorageEndPointSuffix)
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("Unexpected error: %v", err)
				}
			},
		},
		{
			name: "dstAccount Name is different and storageAccountClient is nil",
			testFunc: func(t *testing.T) {
				ctx := context.Background()
				d := NewFakeDriver()
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				mockStorageAccountsClient := mock_accountclient.NewMockInterface(ctrl)
				d.cloud.ComputeClientFactory = mock_azclient.NewMockClientFactory(gomock.NewController(t))
				d.cloud.ComputeClientFactory.(*mock_azclient.MockClientFactory).EXPECT().GetAccountClient().Return(mockStorageAccountsClient).AnyTimes()
				mockStorageAccountsClient.EXPECT().ListKeys(gomock.Any(), gomock.Any(), gomock.Any()).Return([]*armstorage.AccountKey{}, nil).AnyTimes()
				d.cloud.ComputeClientFactory.(*mock_azclient.MockClientFactory).EXPECT().GetAccountClientForSub(gomock.Any()).Return(mockStorageAccountsClient, nil).AnyTimes()
				mp := map[string]string{}

				volumeSource := &csi.VolumeContentSource_VolumeSource{
					VolumeId: "vol_1#f5713de20cde511e8ba4900#fileshare#",
				}
				volumeContentSourceVolumeSource := &csi.VolumeContentSource_Volume{
					Volume: volumeSource,
				}
				volumecontensource := csi.VolumeContentSource{
					Type: volumeContentSourceVolumeSource,
				}

				req := &csi.CreateVolumeRequest{
					Name:                "unit-test",
					VolumeCapabilities:  stdVolumeCapabilities,
					Parameters:          mp,
					VolumeContentSource: &volumecontensource,
				}

				expectedErr := fmt.Errorf("empty keys")
				err := d.copyVolume(ctx, req, "dstAccount", "sastoken", nil, "dstContainer", "", &storage.AccountOptions{Name: "dstAccount", ResourceGroup: "rg", SubscriptionID: "subsID", GetLatestAccountKey: true}, defaultStorageEndPointSuffix)
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("Unexpected error: %v", err)
				}
			},
		},
		{
			name: "azcopy job is already completed",
			testFunc: func(t *testing.T) {
				ctx := context.Background()
				d := NewFakeDriver()
				mp := map[string]string{}

				volumeSource := &csi.VolumeContentSource_VolumeSource{
					VolumeId: "vol_1#f5713de20cde511e8ba4900#fileshare#",
				}
				volumeContentSourceVolumeSource := &csi.VolumeContentSource_Volume{
					Volume: volumeSource,
				}
				volumecontensource := csi.VolumeContentSource{
					Type: volumeContentSourceVolumeSource,
				}

				req := &csi.CreateVolumeRequest{
					Name:                "unit-test",
					VolumeCapabilities:  stdVolumeCapabilities,
					Parameters:          mp,
					VolumeContentSource: &volumecontensource,
				}

				ctrl := gomock.NewController(t)
				defer ctrl.Finish()

				m := util.NewMockEXEC(ctrl)
				listStr := "JobId: ed1c3833-eaff-fe42-71d7-513fb065a9d9\nStart Time: Monday, 07-Aug-23 03:29:54 UTC\nStatus: Completed\nCommand: copy https://{accountName}.file.core.windows.net/{srcFileshare}{SAStoken} https://{accountName}.file.core.windows.net/{dstFileshare}{SAStoken} --recursive --check-length=false"
				m.EXPECT().RunCommand(gomock.Eq("azcopy jobs list | grep dstContainer -B 3"), gomock.Any()).Return(listStr, nil)

				d.azcopy.ExecCmd = m

				var expectedErr error
				err := d.copyVolume(ctx, req, "", "sastoken", nil, "dstContainer", "", nil, defaultStorageEndPointSuffix)
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("Unexpected error: %v", err)
				}
			},
		},
		{
			name: "azcopy job is in progress",
			testFunc: func(t *testing.T) {
				ctx := context.Background()
				accountOptions := storage.AccountOptions{}
				d := NewFakeDriver()
				mp := map[string]string{}

				volumeSource := &csi.VolumeContentSource_VolumeSource{
					VolumeId: "vol_1#f5713de20cde511e8ba4900#fileshare#",
				}
				volumeContentSourceVolumeSource := &csi.VolumeContentSource_Volume{
					Volume: volumeSource,
				}
				volumecontensource := csi.VolumeContentSource{
					Type: volumeContentSourceVolumeSource,
				}

				req := &csi.CreateVolumeRequest{
					Name:                "unit-test",
					VolumeCapabilities:  stdVolumeCapabilities,
					Parameters:          mp,
					VolumeContentSource: &volumecontensource,
				}

				ctrl := gomock.NewController(t)
				defer ctrl.Finish()

				m := util.NewMockEXEC(ctrl)
				listStr1 := "JobId: ed1c3833-eaff-fe42-71d7-513fb065a9d9\nStart Time: Monday, 07-Aug-23 03:29:54 UTC\nStatus: InProgress\nCommand: copy https://{accountName}.file.core.windows.net/{srcFileshare}{SAStoken} https://{accountName}.file.core.windows.net/{dstFileshare}{SAStoken} --recursive --check-length=false"
				m.EXPECT().RunCommand(gomock.Eq("azcopy jobs list | grep dstContainer -B 3"), gomock.Any()).Return(listStr1, nil).AnyTimes()
				m.EXPECT().RunCommand(gomock.Not("azcopy jobs list | grep dstBlobContainer -B 3"), gomock.Any()).Return("Percent Complete (approx): 50.0", nil).AnyTimes()

				d.azcopy.ExecCmd = m
				d.waitForAzCopyTimeoutMinutes = 1

				err := d.copyVolume(ctx, req, "", "sastoken", nil, "dstContainer", "", &accountOptions, defaultStorageEndPointSuffix)
				if !reflect.DeepEqual(err, wait.ErrWaitTimeout) {
					t.Errorf("Unexpected error: %v", err)
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func TestParseDays(t *testing.T) {
	type args struct {
		dayStr string
	}
	tests := []struct {
		name    string
		args    args
		want    int32
		wantErr bool
	}{
		{
			name: "empty string",
			args: args{
				dayStr: "",
			},
			want:    0,
			wantErr: true,
		},
		{
			name: "out of range",
			args: args{
				dayStr: "366",
			},
			want:    0,
			wantErr: true,
		},
		{
			name: "ok",
			args: args{
				dayStr: "365",
			},
			want:    365,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDays(tt.args.dayStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseDays() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("parseDays() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenerateSASToken(t *testing.T) {
	d := NewFakeDriver()
	storageEndpointSuffix := defaultStorageEndPointSuffix
	tests := []struct {
		name        string
		accountName string
		accountKey  string
		want        string
		expectedErr error
	}{
		{
			name:        "accountName nil",
			accountName: "",
			accountKey:  "",
			want:        "se=",
			expectedErr: nil,
		},
		{
			name:        "account key illegal",
			accountName: "unit-test",
			accountKey:  "fakeValue",
			want:        "",
			expectedErr: status.Errorf(codes.Internal, "failed to generate sas token in creating new shared key credential, accountName: %s, err: %s", "unit-test", "decode account key: illegal base64 data at input byte 8"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sas, err := d.generateSASToken(context.Background(), tt.accountName, tt.accountKey, storageEndpointSuffix, 30)
			if !reflect.DeepEqual(err, tt.expectedErr) {
				t.Errorf("generateSASToken error = %v, expectedErr %v, sas token = %v, want %v", err, tt.expectedErr, sas, tt.want)
				return
			}
			if !strings.Contains(sas, tt.want) {
				t.Errorf("sas token = %v, want %v", sas, tt.want)
			}
		})
	}
}

func TestAuthorizeAzcopyWithIdentity(t *testing.T) {
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "use service principal to authorize azcopy",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{
					Config: config.Config{
						AzureClientConfig: config.AzureClientConfig{
							ARMClientConfig: azclient.ARMClientConfig{
								TenantID: "TenantID",
							},
							AzureAuthConfig: azclient.AzureAuthConfig{
								AADClientID:     "AADClientID",
								AADClientSecret: "AADClientSecret",
							},
						},
					},
				}
				expectedAuthAzcopyEnv := []string{
					fmt.Sprintf(azcopyAutoLoginType + "=SPN"),
					fmt.Sprintf(azcopySPAApplicationID + "=AADClientID"),
					fmt.Sprintf(azcopySPAClientSecret + "=AADClientSecret"),
					fmt.Sprintf(azcopyTenantID + "=TenantID"),
				}
				var expectedErr error
				authAzcopyEnv, err := d.authorizeAzcopyWithIdentity()
				if !reflect.DeepEqual(authAzcopyEnv, expectedAuthAzcopyEnv) || !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("Unexpected authAzcopyEnv: %v, Unexpected error: %v", authAzcopyEnv, err)
				}
			},
		},
		{
			name: "use service principal to authorize azcopy but client id is empty",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{
					Config: config.Config{
						AzureClientConfig: config.AzureClientConfig{
							ARMClientConfig: azclient.ARMClientConfig{
								TenantID: "TenantID",
							},
							AzureAuthConfig: azclient.AzureAuthConfig{
								AADClientSecret: "AADClientSecret",
							},
						},
					},
				}
				expectedAuthAzcopyEnv := []string{}
				expectedErr := fmt.Errorf("AADClientID and TenantID must be set when use service principal")
				authAzcopyEnv, err := d.authorizeAzcopyWithIdentity()
				if !reflect.DeepEqual(authAzcopyEnv, expectedAuthAzcopyEnv) || !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("Unexpected authAzcopyEnv: %v, Unexpected error: %v", authAzcopyEnv, err)
				}
			},
		},
		{
			name: "use user assigned managed identity to authorize azcopy",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{
					Config: config.Config{
						AzureClientConfig: config.AzureClientConfig{
							AzureAuthConfig: azclient.AzureAuthConfig{
								UseManagedIdentityExtension: true,
								UserAssignedIdentityID:      "UserAssignedIdentityID",
							},
						},
					},
				}
				expectedAuthAzcopyEnv := []string{
					fmt.Sprintf(azcopyAutoLoginType + "=MSI"),
					fmt.Sprintf(azcopyMSIClientID + "=UserAssignedIdentityID"),
				}
				var expectedErr error
				authAzcopyEnv, err := d.authorizeAzcopyWithIdentity()
				if !reflect.DeepEqual(authAzcopyEnv, expectedAuthAzcopyEnv) || !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("Unexpected authAzcopyEnv: %v, Unexpected error: %v", authAzcopyEnv, err)
				}
			},
		},
		{
			name: "use system assigned managed identity to authorize azcopy",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{
					Config: config.Config{
						AzureClientConfig: config.AzureClientConfig{
							AzureAuthConfig: azclient.AzureAuthConfig{
								UseManagedIdentityExtension: true,
							},
						},
					},
				}
				expectedAuthAzcopyEnv := []string{
					fmt.Sprintf(azcopyAutoLoginType + "=MSI"),
				}
				var expectedErr error
				authAzcopyEnv, err := d.authorizeAzcopyWithIdentity()
				if !reflect.DeepEqual(authAzcopyEnv, expectedAuthAzcopyEnv) || !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("Unexpected authAzcopyEnv: %v, Unexpected error: %v", authAzcopyEnv, err)
				}
			},
		},
		{
			name: "AADClientSecret be nil and useManagedIdentityExtension is false",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{
					Config: config.Config{
						AzureClientConfig: config.AzureClientConfig{},
					},
				}
				expectedAuthAzcopyEnv := []string{}
				expectedErr := fmt.Errorf("service principle or managed identity are both not set")
				authAzcopyEnv, err := d.authorizeAzcopyWithIdentity()
				if !reflect.DeepEqual(authAzcopyEnv, expectedAuthAzcopyEnv) || !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("Unexpected authAzcopyEnv: %v, Unexpected error: %v", authAzcopyEnv, err)
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func TestGetAzcopyAuth(t *testing.T) {
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "failed to get accountKey in secrets",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{
					Config: config.Config{},
				}
				secrets := map[string]string{
					defaultSecretAccountName: "accountName",
				}

				ctx := context.Background()
				expectedAccountSASToken := ""
				expectedErr := fmt.Errorf("could not find accountkey or azurestorageaccountkey field in secrets")
				accountSASToken, _, err := d.getAzcopyAuth(ctx, "accountName", "", defaultStorageEndPointSuffix, &storage.AccountOptions{}, secrets, "secretsName", "secretsNamespace", false)
				if !reflect.DeepEqual(err, expectedErr) || !reflect.DeepEqual(accountSASToken, expectedAccountSASToken) {
					t.Errorf("Unexpected accountSASToken: %s, Unexpected error: %v", accountSASToken, err)
				}
			},
		},
		{
			name: "generate sas token using account key",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{
					Config: config.Config{
						AzureClientConfig: config.AzureClientConfig{
							AzureAuthConfig: azclient.AzureAuthConfig{
								UseManagedIdentityExtension: true,
							},
						},
					},
				}
				secrets := map[string]string{
					defaultSecretAccountName: "accountName",
					defaultSecretAccountKey:  "YWNjb3VudGtleQo=",
				}
				accountSASToken, _, err := d.getAzcopyAuth(context.Background(), "accountName", "", defaultStorageEndPointSuffix, &storage.AccountOptions{}, secrets, "secretsName", "secretsNamespace", false)
				if !reflect.DeepEqual(err, nil) || !strings.Contains(accountSASToken, "?se=") {
					t.Errorf("Unexpected accountSASToken: %s, Unexpected error: %v", accountSASToken, err)
				}
			},
		},
		{
			name: "fall back to generate SAS token failed for illegal account key",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{
					Config: config.Config{
						AzureClientConfig: config.AzureClientConfig{
							AzureAuthConfig: azclient.AzureAuthConfig{
								UseManagedIdentityExtension: true,
							},
						},
					},
				}
				secrets := map[string]string{
					defaultSecretAccountName: "accountName",
					defaultSecretAccountKey:  "fakeValue",
				}

				expectedAccountSASToken := ""
				expectedErr := status.Errorf(codes.Internal, "failed to generate sas token in creating new shared key credential, accountName: %s, err: %s", "accountName", "decode account key: illegal base64 data at input byte 8")
				accountSASToken, _, err := d.getAzcopyAuth(context.Background(), "accountName", "", defaultStorageEndPointSuffix, &storage.AccountOptions{}, secrets, "secretsName", "secretsNamespace", false)
				if !reflect.DeepEqual(err, expectedErr) || !reflect.DeepEqual(accountSASToken, expectedAccountSASToken) {
					t.Errorf("Unexpected accountSASToken: %s, Unexpected error: %v", accountSASToken, err)
				}
			},
		},
		{
			name: "generate SAS token failed for illegal account key",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{
					Config: config.Config{},
				}
				secrets := map[string]string{
					defaultSecretAccountName: "accountName",
					defaultSecretAccountKey:  "fakeValue",
				}

				ctx := context.Background()
				expectedAccountSASToken := ""
				expectedErr := status.Errorf(codes.Internal, "failed to generate sas token in creating new shared key credential, accountName: %s, err: %s", "accountName", "decode account key: illegal base64 data at input byte 8")
				accountSASToken, _, err := d.getAzcopyAuth(ctx, "accountName", "", defaultStorageEndPointSuffix, &storage.AccountOptions{}, secrets, "secretsName", "secretsNamespace", false)
				if !reflect.DeepEqual(err, expectedErr) || !reflect.DeepEqual(accountSASToken, expectedAccountSASToken) {
					t.Errorf("Unexpected accountSASToken: %s, Unexpected error: %v", accountSASToken, err)
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}
