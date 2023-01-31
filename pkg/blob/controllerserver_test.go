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
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/services/storage/mgmt/2021-09-01/storage"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/cloud-provider-azure/pkg/azureclients/blobclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azureclients/storageaccountclient/mockstorageaccountclient"
	azure "sigs.k8s.io/cloud-provider-azure/pkg/provider"
	"sigs.k8s.io/cloud-provider-azure/pkg/retry"
)

type errType int32

const (
	DATAPLANE errType = iota
	MANAGEMENT
	CUSTOM
	NULL
)

var _ blobclient.Interface = &mockBlobClient{}

// mock blobclient that returns the errortype for create/delete/get operations (default value nil)
type mockBlobClient struct {
	// type of error returned
	errorType *errType
	// custom string for error type CUSTOM
	custom  *string
	conProp *storage.ContainerProperties
}

func (c *mockBlobClient) CreateContainer(ctx context.Context, subsID, resourceGroupName, accountName, containerName string, parameters storage.BlobContainer) *retry.Error {
	switch *c.errorType {
	case DATAPLANE:
		return retry.GetError(&http.Response{}, fmt.Errorf(containerBeingDeletedDataplaneAPIError))
	case MANAGEMENT:
		return retry.GetError(&http.Response{}, fmt.Errorf(containerBeingDeletedManagementAPIError))
	case CUSTOM:
		return retry.GetError(&http.Response{}, fmt.Errorf(*c.custom))
	}
	return nil
}
func (c *mockBlobClient) DeleteContainer(ctx context.Context, subsID, resourceGroupName, accountName, containerName string) *retry.Error {
	switch *c.errorType {
	case DATAPLANE:
		return retry.GetError(&http.Response{}, fmt.Errorf(containerBeingDeletedDataplaneAPIError))
	case MANAGEMENT:
		return retry.GetError(&http.Response{}, fmt.Errorf(containerBeingDeletedManagementAPIError))
	case CUSTOM:
		return retry.GetError(&http.Response{}, fmt.Errorf(*c.custom))
	}
	return nil
}
func (c *mockBlobClient) GetContainer(ctx context.Context, subsID, resourceGroupName, accountName, containerName string) (storage.BlobContainer, *retry.Error) {
	switch *c.errorType {
	case DATAPLANE:
		return storage.BlobContainer{ContainerProperties: c.conProp}, retry.GetError(&http.Response{}, fmt.Errorf(containerBeingDeletedDataplaneAPIError))
	case MANAGEMENT:
		return storage.BlobContainer{ContainerProperties: c.conProp}, retry.GetError(&http.Response{}, fmt.Errorf(containerBeingDeletedManagementAPIError))
	case CUSTOM:
		return storage.BlobContainer{ContainerProperties: c.conProp}, retry.GetError(&http.Response{}, fmt.Errorf(*c.custom))
	}
	return storage.BlobContainer{ContainerProperties: c.conProp}, nil
}

func (c *mockBlobClient) GetServiceProperties(ctx context.Context, subsID, resourceGroupName, accountName string) (storage.BlobServiceProperties, error) {
	return storage.BlobServiceProperties{}, nil
}

func (c *mockBlobClient) SetServiceProperties(ctx context.Context, subsID, resourceGroupName, accountName string, parameters storage.BlobServiceProperties) (storage.BlobServiceProperties, error) {
	return storage.BlobServiceProperties{}, nil
}

func newMockBlobClient(errorType *errType, custom *string, conProp *storage.ContainerProperties) blobclient.Interface {
	return &mockBlobClient{errorType: errorType, custom: custom, conProp: conProp}
}

// creates and returns mock storage account client
func NewMockSAClient(ctx context.Context, ctrl *gomock.Controller, subsID, rg, accName string, keyList *[]storage.AccountKey) *mockstorageaccountclient.MockInterface {
	cl := mockstorageaccountclient.NewMockInterface(ctrl)

	cl.EXPECT().
		ListKeys(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(storage.AccountListKeysResult{
			Keys: keyList,
		}, nil).
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
				d.cloud = &azure.Cloud{}
				mp := make(map[string]string)
				mp[protocolField] = "unit-test"
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
				_, err := d.CreateVolume(context.Background(), req)
				expectedErr := status.Errorf(codes.InvalidArgument, "protocol(unit-test) is not supported, supported protocol list: [fuse fuse2 nfs]")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "storageAccount and matchTags conflict",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &azure.Cloud{}
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
				d.cloud = &azure.Cloud{}
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
				d.cloud = &azure.Cloud{}
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
				d.cloud = &azure.Cloud{}
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
				mp[resourceGroupField] = "unit-test"
				mp[containerNameField] = "unit-test"
				mp[mountPermissionsField] = "0755"
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
				mockStorageAccountsClient.EXPECT().ListByResourceGroup(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, rerr).AnyTimes()
				_, err := d.CreateVolume(context.Background(), req)
				expectedErr := status.Errorf(codes.Internal, "ensure storage account failed with could not list storage accounts for account type : Retriable: false, RetryAfter: 0s, HTTPStatusCode: 0, RawError: test")
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

				expectedErr := status.Errorf(codes.InvalidArgument, fmt.Sprintf("invalid parameter %q in storage class", "invalidparameter"))
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

				expectedErr := status.Errorf(codes.InvalidArgument, fmt.Sprintf("invalid %s %s in storage class", "mountPermissions", "0abc"))
				_, err := d.CreateVolume(context.Background(), req)
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("Unexpected error: %v", err)
				}
			},
		},
		{
			name: "NFS not supported by cross subscription",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &azure.Cloud{}
				d.cloud.SubscriptionID = "bar"
				mp := make(map[string]string)
				mp[subscriptionIDField] = "foo"
				mp[protocolField] = "nfs"
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

				expectedErr := status.Errorf(codes.InvalidArgument, fmt.Sprintf("NFS protocol is not supported in cross subscription(%s)", "foo"))
				_, err := d.CreateVolume(context.Background(), req)
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("Unexpected error: %v", err)
				}
			},
		},
		{
			name: "storeAccountKey must be set as true in cross subscription",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &azure.Cloud{}
				d.cloud.SubscriptionID = "bar"
				mp := make(map[string]string)
				mp[subscriptionIDField] = "foo"
				mp[storeAccountKeyField] = falseValue
				mp[protocolField] = "unit-test"
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

				expectedErr := status.Errorf(codes.InvalidArgument, fmt.Sprintf("storeAccountKey must set as true in cross subscription(%s)", "foo"))
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
				d.cloud = &azure.Cloud{}
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
				req := &csi.CreateVolumeRequest{
					Name:               "unit-test",
					VolumeCapabilities: stdVolumeCapabilities,
					Parameters:         mp,
				}
				d.Cap = []*csi.ControllerServiceCapability{
					controllerServiceCapability,
				}

				expectedErr := status.Errorf(codes.Internal, "update service endpoints failed with error: %v", fmt.Errorf("SubnetsClient is nil"))
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
				d.cloud = &azure.Cloud{}
				d.cloud.Config.DisableAzureStackCloud = false
				d.cloud.Config.Cloud = "AZURESTACKCLOUD"
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

				expectedErr := status.Errorf(codes.InvalidArgument, fmt.Sprintf("Invalid skuName value: %s, as Azure Stack only supports %s and %s Storage Account types.", "unit-test", storage.SkuNamePremiumLRS, storage.SkuNameStandardLRS))
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
				d.cloud = &azure.Cloud{}
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

				keyList := make([]storage.AccountKey, 0)
				d.cloud.StorageAccountClient = NewMockSAClient(context.Background(), gomock.NewController(t), "subID", "unit-test", "unit-test", &keyList)

				req := &csi.CreateVolumeRequest{
					Name:               "unit-test",
					VolumeCapabilities: stdVolumeCapabilities,
					Parameters:         mp,
				}
				d.Cap = []*csi.ControllerServiceCapability{
					controllerServiceCapability,
				}

				expectedErr := status.Errorf(codes.Internal, "failed to GetStorageAccesskey on account(%s) rg(%s), error: %v", "unit-test", "unit-test", fmt.Errorf("no valid keys"))
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
				d.cloud = &azure.Cloud{}
				d.cloud.SubscriptionID = "subID"

				keyList := make([]storage.AccountKey, 1)
				fakeKey := "fakeKey"
				fakeValue := "fakeValue"
				keyList[0] = (storage.AccountKey{
					KeyName: &fakeKey,
					Value:   &fakeValue,
				})
				d.cloud.StorageAccountClient = NewMockSAClient(context.Background(), gomock.NewController(t), "subID", "unit-test", "unit-test", &keyList)

				errorType := DATAPLANE
				d.cloud.BlobClient = &mockBlobClient{errorType: &errorType}

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
			},
		},
		{
			name: "Failed to get storage access key",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &azure.Cloud{}
				d.cloud.SubscriptionID = "subID"

				keyList := make([]storage.AccountKey, 0)
				d.cloud.StorageAccountClient = NewMockSAClient(context.Background(), gomock.NewController(t), "subID", "unit-test", "unit-test", &keyList)

				errorType := NULL
				d.cloud.BlobClient = &mockBlobClient{errorType: &errorType}

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
					codes.Internal, "failed to GetStorageAccesskey on account(%s) rg(%s), error: %v", mp[storageAccountField], mp[resourceGroupField], fmt.Errorf("no valid keys"))
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
				d.cloud = &azure.Cloud{}
				d.cloud.SubscriptionID = "subID"

				keyList := make([]storage.AccountKey, 1)
				fakeKey := "fakeKey"
				fakeValue := "fakeValue"
				keyList[0] = (storage.AccountKey{
					KeyName: &fakeKey,
					Value:   &fakeValue,
				})
				d.cloud.StorageAccountClient = NewMockSAClient(context.Background(), gomock.NewController(t), "subID", "unit-test", "unit-test", &keyList)

				errorType := NULL
				d.cloud.BlobClient = &mockBlobClient{errorType: &errorType}

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
				expectedErr := status.Errorf(codes.Internal, "invalid delete volume req: volume_id:\"unit-test\" ")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "invalid volume Id",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &azure.Cloud{}
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
				d.cloud = &azure.Cloud{}
				d.Cap = []*csi.ControllerServiceCapability{
					controllerServiceCapability,
				}
				req := &csi.DeleteVolumeRequest{
					VolumeId: "rg#accountName#containerName",
				}
				d.dataPlaneAPIVolCache.Set("rg#accountName#containerName", "accountName")
				_, err := d.DeleteVolume(context.Background(), req)
				expectedErr := status.Errorf(codes.Internal, "GetAuthEnv(%s) failed with %v", "rg#accountName#containerName", fmt.Errorf("no key for storage account(%s) under resource group(%s), err %w", "accountName", "rg", fmt.Errorf("StorageAccountClient is nil")))
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
				mockStorageAccountsClient.EXPECT().ListKeys(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(accountListKeysResult, rerr).AnyTimes()
				expectedErr := fmt.Errorf("base storage service url required")
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
				mockStorageAccountsClient.EXPECT().ListKeys(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(list, nil).AnyTimes()
				expectedErr := fmt.Errorf("azure: base storage service url required")
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
		containerProp *storage.ContainerProperties
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
			expectedErr:   status.Error(codes.Internal, "azure: base storage service url required"),
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
			containerProp: &storage.ContainerProperties{},
			expectedRes:   nil,
			expectedErr:   status.Errorf(codes.Internal, retry.GetError(&http.Response{}, fmt.Errorf(containerBeingDeletedDataplaneAPIError)).Error().Error()),
		},
		{
			name: "Requested Volume does not exist",
			req: &csi.ValidateVolumeCapabilitiesRequest{
				VolumeId:           "unit#test#test",
				VolumeCapabilities: stdVolumeCapabilities,
				Secrets:            map[string]string{},
			},
			clientErr:     NULL,
			containerProp: &storage.ContainerProperties{},
			expectedRes:   nil,
			expectedErr:   status.Errorf(codes.NotFound, fmt.Sprintf("requested volume(%s) does not exist", "unit#test#test")),
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
	d := NewFakeDriver()
	d.cloud = &azure.Cloud{}
	d.Cap = []*csi.ControllerServiceCapability{
		controllerServiceCapability,
	}
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
	mockStorageAccountsClient.EXPECT().ListKeys(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(list, nil).AnyTimes()

	for _, test := range testCases {
		res, err := d.ValidateVolumeCapabilities(context.Background(), test.req)
		d.cloud.BlobClient = newMockBlobClient(&test.clientErr, pointer.String(""), test.containerProp)
		assert.Equal(t, test.expectedErr, err, "Error in testcase (%s): Errors must match", test.name)
		assert.Equal(t, test.expectedRes, res, "Error in testcase (%s): Response must match", test.name)
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
			name: "invalid expand volume req",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				req := &csi.ControllerExpandVolumeRequest{
					VolumeId:      "unit-test",
					CapacityRange: &csi.CapacityRange{},
				}
				_, err := d.ControllerExpandVolume(context.Background(), req)
				expectedErr := status.Errorf(codes.Internal, "invalid expand volume req: volume_id:\"unit-test\" capacity_range:<> ")
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
			desc:          "Base storage service url required",
			containerName: "containerName",
			secrets: map[string]string{
				defaultSecretAccountName: "accountname",
				defaultSecretAccountKey:  "key",
			},
			clientErr:   NULL,
			expectedErr: fmt.Errorf("azure: base storage service url required"),
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
			expectedErr:   fmt.Errorf("timed out waiting for the condition"),
		},
		{
			desc:          "Management API Error",
			containerName: "containerName",
			secrets:       map[string]string{},
			clientErr:     MANAGEMENT,
			expectedErr:   fmt.Errorf("timed out waiting for the condition"),
		},
		{
			desc:          "Random Client Error",
			containerName: "containerName",
			secrets:       map[string]string{},
			clientErr:     CUSTOM,
			customErrStr:  "foobar",
			expectedErr:   retry.GetError(&http.Response{}, fmt.Errorf("foobar")).Error(),
		},
	}

	d := NewFakeDriver()
	d.cloud = &azure.Cloud{}
	conProp := &storage.ContainerProperties{}
	for _, test := range tests {
		d.cloud.BlobClient = newMockBlobClient(&test.clientErr, &test.customErrStr, conProp)
		err := d.CreateBlobContainer(context.Background(), test.subsID, test.rg, test.accountName, test.containerName, test.secrets)
		if !reflect.DeepEqual(err, test.expectedErr) {
			t.Errorf("test(%s), actualErr: (%v), expectedErr: (%v)", test.desc, err, test.expectedErr)
		}
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
			desc:          "Base storage service url required",
			containerName: "containerName",
			secrets: map[string]string{
				defaultSecretAccountName: "accountname",
				defaultSecretAccountKey:  "key",
			},
			clientErr:   NULL,
			expectedErr: fmt.Errorf("azure: base storage service url required"),
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
				retry.GetError(&http.Response{}, fmt.Errorf("foobar")).Error()),
		},
	}

	d := NewFakeDriver()
	d.cloud = &azure.Cloud{}

	connProp := &storage.ContainerProperties{}
	for _, test := range tests {
		d.cloud.BlobClient = newMockBlobClient(&test.clientErr, &test.customErrStr, connProp)
		err := d.DeleteBlobContainer(context.Background(), test.subsID, test.rg, test.accountName, test.containerName, test.secrets)
		if !reflect.DeepEqual(err, test.expectedErr) {
			t.Errorf("test(%s), actualErr: (%v), expectedErr: (%v)", test.desc, err, test.expectedErr)
		}
	}
}

func Test_parseDays(t *testing.T) {
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
