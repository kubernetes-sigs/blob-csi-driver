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

package blob

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"golang.org/x/sync/errgroup"
	v1api "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/blob-csi-driver/pkg/util"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/accountclient/mock_accountclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/mock_azclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/provider/storage"
)

const (
	fakeNodeID     = "fakeNodeID"
	fakeDriverName = "fake"
	vendorVersion  = "0.3.0"
)

func NewFakeDriver() *Driver {
	driverOptions := DriverOptions{
		NodeID:                      fakeNodeID,
		DriverName:                  DefaultDriverName,
		BlobfuseProxyEndpoint:       "",
		EnableBlobfuseProxy:         false,
		BlobfuseProxyConnTimeout:    5,
		WaitForAzCopyTimeoutMinutes: 1,
		EnableBlobMockMount:         false,
	}
	driver := NewDriver(&driverOptions, nil, &storage.AccountRepo{})
	driver.Name = fakeDriverName
	driver.Version = vendorVersion
	driver.subnetLockMap = util.NewLockMap()
	return driver
}

func TestNewFakeDriver(t *testing.T) {
	driverOptions := DriverOptions{
		NodeID:                   fakeNodeID,
		DriverName:               DefaultDriverName,
		BlobfuseProxyEndpoint:    "",
		EnableBlobfuseProxy:      false,
		BlobfuseProxyConnTimeout: 5,
		EnableBlobMockMount:      false,
	}
	d := NewDriver(&driverOptions, nil, &storage.AccountRepo{})
	assert.NotNil(t, d)
}

func TestNewDriver(t *testing.T) {
	driverOptions := DriverOptions{
		NodeID:                      fakeNodeID,
		DriverName:                  DefaultDriverName,
		BlobfuseProxyEndpoint:       "",
		EnableBlobfuseProxy:         false,
		BlobfuseProxyConnTimeout:    5,
		WaitForAzCopyTimeoutMinutes: 1,
		EnableBlobMockMount:         false,
	}
	driver := NewDriver(&driverOptions, nil, &storage.AccountRepo{})
	fakedriver := NewFakeDriver()
	fakedriver.Name = DefaultDriverName
	fakedriver.Version = driverVersion
	fakedriver.accountSearchCache = driver.accountSearchCache
	fakedriver.dataPlaneAPIVolCache = driver.dataPlaneAPIVolCache
	fakedriver.azcopySasTokenCache = driver.azcopySasTokenCache
	fakedriver.volStatsCache = driver.volStatsCache
	fakedriver.subnetCache = driver.subnetCache
	fakedriver.cloud = driver.cloud
	assert.Equal(t, driver, fakedriver)
}

func TestRun(t *testing.T) {
	fakeCredFile := "fake-cred-file.json"
	fakeCredContent := `{
    "tenantId": "1234",
    "subscriptionId": "12345",
    "aadClientId": "123456",
    "aadClientSecret": "1234567",
    "resourceGroup": "rg1",
    "location": "loc"
}`

	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "Successful run",
			testFunc: func(t *testing.T) {
				if err := os.WriteFile(fakeCredFile, []byte(fakeCredContent), 0666); err != nil {
					t.Error(err)
				}

				defer func() {
					if err := os.Remove(fakeCredFile); err != nil {
						t.Error(err)
					}
				}()

				originalCredFile, ok := os.LookupEnv(DefaultAzureCredentialFileEnv)
				if ok {
					defer os.Setenv(DefaultAzureCredentialFileEnv, originalCredFile)
				} else {
					defer os.Unsetenv(DefaultAzureCredentialFileEnv)
				}
				os.Setenv(DefaultAzureCredentialFileEnv, fakeCredFile)

				d := NewFakeDriver()

				ctx, cancelFn := context.WithCancel(context.Background())
				var routines errgroup.Group
				routines.Go(func() error { return d.Run(ctx, "tcp://127.0.0.1:0") })
				time.Sleep(time.Millisecond * 500)
				cancelFn()
				time.Sleep(time.Millisecond * 500)
				err := routines.Wait()
				assert.Nil(t, err)
			},
		},
		{
			name: "Successful run with node ID missing",
			testFunc: func(t *testing.T) {
				if err := os.WriteFile(fakeCredFile, []byte(fakeCredContent), 0666); err != nil {
					t.Error(err)
				}

				defer func() {
					if err := os.Remove(fakeCredFile); err != nil {
						t.Error(err)
					}
				}()

				originalCredFile, ok := os.LookupEnv(DefaultAzureCredentialFileEnv)
				if ok {
					defer os.Setenv(DefaultAzureCredentialFileEnv, originalCredFile)
				} else {
					defer os.Unsetenv(DefaultAzureCredentialFileEnv)
				}
				os.Setenv(DefaultAzureCredentialFileEnv, fakeCredFile)

				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{}
				d.NodeID = ""
				ctx, cancelFn := context.WithCancel(context.Background())
				var routines errgroup.Group
				routines.Go(func() error { return d.Run(ctx, "tcp://127.0.0.1:0") })
				time.Sleep(time.Millisecond * 500)
				cancelFn()
				time.Sleep(time.Millisecond * 500)
				err := routines.Wait()
				assert.Nil(t, err)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func TestGetContainerInfo(t *testing.T) {
	tests := []struct {
		volumeID      string
		rg            string
		account       string
		container     string
		namespace     string
		subsID        string
		expectedError error
	}{
		{
			volumeID:      "rg#f5713de20cde511e8ba4900#container#uuid#namespace",
			rg:            "rg",
			account:       "f5713de20cde511e8ba4900",
			container:     "container",
			namespace:     "namespace",
			expectedError: nil,
		},
		{
			volumeID:      "rg#f5713de20cde511e8ba4900#container##namespace",
			rg:            "rg",
			account:       "f5713de20cde511e8ba4900",
			container:     "container",
			namespace:     "namespace",
			expectedError: nil,
		},
		{
			volumeID:      "rg#f5713de20cde511e8ba4900#container##",
			rg:            "rg",
			account:       "f5713de20cde511e8ba4900",
			container:     "container",
			namespace:     "",
			expectedError: nil,
		},
		{
			volumeID:      "rg#f5713de20cde511e8ba4900#pvc-file-dynamic-17e43f84-f474-11e8-acd0-000d3a00df41#uuid",
			rg:            "rg",
			account:       "f5713de20cde511e8ba4900",
			container:     "pvc-file-dynamic-17e43f84-f474-11e8-acd0-000d3a00df41",
			expectedError: nil,
		},
		{
			volumeID:      "rg#f5713de20cde511e8ba4900#pvc-file-dynamic-17e43f84-f474-11e8-acd0-000d3a00df41#",
			rg:            "rg",
			account:       "f5713de20cde511e8ba4900",
			container:     "pvc-file-dynamic-17e43f84-f474-11e8-acd0-000d3a00df41",
			expectedError: nil,
		},
		{
			volumeID:      "rg#f5713de20cde511e8ba4900#pvc-file-dynamic-17e43f84-f474-11e8-acd0-000d3a00df41",
			rg:            "rg",
			account:       "f5713de20cde511e8ba4900",
			container:     "pvc-file-dynamic-17e43f84-f474-11e8-acd0-000d3a00df41",
			expectedError: nil,
		},
		{
			volumeID:      "rg#f5713de20cde511e8ba4900",
			rg:            "",
			account:       "",
			container:     "",
			expectedError: fmt.Errorf("error parsing volume id: \"rg#f5713de20cde511e8ba4900\", should at least contain two #"),
		},
		{
			volumeID:      "rg",
			rg:            "",
			account:       "",
			container:     "",
			expectedError: fmt.Errorf("error parsing volume id: \"rg\", should at least contain two #"),
		},
		{
			volumeID:      "",
			rg:            "",
			account:       "",
			container:     "",
			expectedError: fmt.Errorf("error parsing volume id: \"\", should at least contain two #"),
		},
		{
			volumeID:      "rg#f5713de20cde511e8ba4900#container#uuid#namespace#subsID",
			rg:            "rg",
			account:       "f5713de20cde511e8ba4900",
			container:     "container",
			namespace:     "namespace",
			subsID:        "subsID",
			expectedError: nil,
		},
		{
			volumeID:      "rg#f5713de20cde511e8ba4900#container###subsID",
			rg:            "rg",
			account:       "f5713de20cde511e8ba4900",
			container:     "container",
			subsID:        "subsID",
			expectedError: nil,
		},
		{
			volumeID:      "rg#f5713de20cde511e8ba4900#container#uuid#namespace#",
			rg:            "rg",
			account:       "f5713de20cde511e8ba4900",
			container:     "container",
			namespace:     "namespace",
			expectedError: nil,
		},
	}

	for _, test := range tests {
		rg, account, container, ns, subsID, err := GetContainerInfo(test.volumeID)
		if !reflect.DeepEqual(rg, test.rg) || !reflect.DeepEqual(account, test.account) ||
			!reflect.DeepEqual(container, test.container) || !reflect.DeepEqual(err, test.expectedError) ||
			!reflect.DeepEqual(ns, test.namespace) || !reflect.DeepEqual(subsID, test.subsID) {
			t.Errorf("input: %q, GetContainerInfo rg: %q, rg: %q, account: %q, account: %q, container: %q, container: %q, namespace: %q, namespace: %q, err: %q, expectedError: %q", test.volumeID, rg, test.rg, account, test.account,
				container, test.container, ns, test.namespace, err, test.expectedError)
		}
	}
}

func TestIsRetriableError(t *testing.T) {
	tests := []struct {
		desc         string
		rpcErr       error
		expectedBool bool
	}{
		{
			desc:         "non-retriable error",
			rpcErr:       nil,
			expectedBool: false,
		},
		{
			desc:         "accountNotProvisioned",
			rpcErr:       errors.New("could not get storage key for storage account : could not get storage key for storage account f233333: Retriable: true, RetryAfter: 0001-01-01 00:00:00 +0000 UTC, HTTPStatusCode: 409, RawError: storage.AccountsClient#ListKeys: Failure sending request: StatusCode=409 -- Original Error: autorest/azure: Service returned an error. Status=<nil> Code=\"StorageAccountIsNotProvisioned\" Message=\"The storage account provisioning state must be 'Succeeded' before executing the operation.\""),
			expectedBool: true,
		},
		{
			desc:         "tooManyRequests",
			rpcErr:       errors.New("could not get storage key for storage account : could not list storage accounts for account type Premium_LRS: Retriable: true, RetryAfter: 0001-01-01 00:00:00 +0000 UTC m=+231.866923225, HTTPStatusCode: 429, RawError: storage.AccountsClient#ListByResourceGroup: Failure responding to request: StatusCode=429 -- Original Error: autorest/azure: Service returned an error. Status=429 Code=\"TooManyRequests\" Message=\"The request is being throttled as the limit has been reached for operation type - List. For more information, see - https://aka.ms/srpthrottlinglimits\""),
			expectedBool: true,
		},
		{
			desc:         "clientThrottled",
			rpcErr:       errors.New("could not list storage accounts for account type : Retriable: true, RetryAfter: 16s, HTTPStatusCode: 0, RawError: azure cloud provider throttled for operation StorageAccountListByResourceGroup with reason \"client throttled\""),
			expectedBool: true,
		},
	}

	for _, test := range tests {
		result := isRetriableError(test.rpcErr)
		if result != test.expectedBool {
			t.Errorf("desc: (%s), input: rpcErr(%v), isRetriableError returned with bool(%v), not equal to expectedBool(%v)",
				test.desc, test.rpcErr, result, test.expectedBool)
		}
	}
}

func TestGetValidContainerName(t *testing.T) {
	tests := []struct {
		volumeName string
		protocol   string
		expected   string
	}{
		{
			volumeName: "aqz",
			expected:   "aqz",
		},
		{
			volumeName: "029",
			expected:   "029",
		},
		{
			volumeName: "a--z",
			expected:   "a-z",
		},
		{
			volumeName: "A2Z",
			expected:   "a2z",
		},
		{
			volumeName: "1234567891234567891234567891234567891234567891234567891234567891",
			expected:   "123456789123456789123456789123456789123456789123456789123456789",
		},
	}

	for _, test := range tests {
		result := getValidContainerName(test.volumeName, "")
		if !reflect.DeepEqual(result, test.expected) {
			t.Errorf("input: %q, getValidContainerName result: %q, expected: %q", test.volumeName, result, test.expected)
		}
	}
}

func TestCheckContainerNameBeginAndEnd(t *testing.T) {
	tests := []struct {
		containerName string
		expected      bool
	}{
		{
			containerName: "aqz",
			expected:      true,
		},
		{
			containerName: "029",
			expected:      true,
		},
		{
			containerName: "a-9",
			expected:      true,
		},
		{
			containerName: "0-z",
			expected:      true,
		},
		{
			containerName: "-1-",
			expected:      false,
		},
		{
			containerName: ":1p",
			expected:      false,
		},
	}

	for _, test := range tests {
		result := checkContainerNameBeginAndEnd(test.containerName)
		if !reflect.DeepEqual(result, test.expected) {
			t.Errorf("input: %q, checkContainerNameBeginAndEnd result: %v, expected: %v", test.containerName, result, test.expected)
		}
	}
}

func TestIsSASToken(t *testing.T) {
	tests := []struct {
		key      string
		expected bool
	}{
		{
			key:      "?sv=2017-03-28&ss=bfqt&srt=sco&sp=rwdlacup",
			expected: true,
		},
		{
			key:      "&ss=bfqt&srt=sco&sp=rwdlacup",
			expected: false,
		},
		{
			key:      "123456789vbDWANIJ319Fqabcded3wwLRnxK70zRJ",
			expected: false,
		},
	}

	for _, test := range tests {
		result := isSASToken(test.key)
		if !reflect.DeepEqual(result, test.expected) {
			t.Errorf("input: %q, isSASToken result: %v, expected: %v", test.key, result, test.expected)
		}
	}
}

func TestIsCorruptedDir(t *testing.T) {
	existingMountPath, err := os.MkdirTemp(os.TempDir(), "blob-csi-mount-test")
	if err != nil {
		t.Fatalf("failed to create tmp dir: %v", err)
	}
	defer os.RemoveAll(existingMountPath)

	tests := []struct {
		desc           string
		dir            string
		expectedResult bool
	}{
		{
			desc:           "NotExist dir",
			dir:            "/tmp/NotExist",
			expectedResult: false,
		},
		{
			desc:           "Existing dir",
			dir:            existingMountPath,
			expectedResult: false,
		},
	}

	for i, test := range tests {
		isCorruptedDir := IsCorruptedDir(test.dir)
		assert.Equal(t, test.expectedResult, isCorruptedDir, "TestCase[%d]: %s", i, test.desc)
	}
}

func TestIsSupportedProtocol(t *testing.T) {
	tests := []struct {
		protocol       string
		expectedResult bool
	}{
		{
			protocol:       "",
			expectedResult: true,
		},
		{
			protocol:       "fuse",
			expectedResult: true,
		},
		{
			protocol:       "nfs",
			expectedResult: true,
		},
		{
			protocol:       "invalid",
			expectedResult: false,
		},
	}

	for _, test := range tests {
		result := isSupportedProtocol(test.protocol)
		if result != test.expectedResult {
			t.Errorf("isSupportedProtocol(%s) returned with %v, not equal to %v", test.protocol, result, test.expectedResult)
		}
	}
}

func TestGetAuthEnv(t *testing.T) {
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "Get storage access key error",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				attrib := make(map[string]string)
				secret := make(map[string]string)
				attrib["containername"] = "unit-test"
				attrib["keyvaultsecretname"] = "unit-test"
				attrib["keyvaultsecretversion"] = "unit-test"
				attrib["storageaccountname"] = "storageaccountname"
				attrib["azurestorageidentityclientid"] = "unit-test"
				attrib["azurestorageidentityobjectid"] = "unit-test"
				attrib["azurestorageidentityresourceid"] = "unit-test"
				attrib["msiendpoint"] = "unit-test"
				attrib["azurestoragespnclientid"] = "unit-test"
				attrib["azurestoragespntenantid"] = "unit-test"
				attrib["azurestorageaadendpoint"] = "unit-test"
				volumeID := "rg#f5713de20cde511e8ba4900#pvc-fuse-dynamic-17e43f84-f474-11e8-acd0-000d3a00df41"
				d.cloud = &storage.AccountRepo{}
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				mockStorageAccountsClient := mock_accountclient.NewMockInterface(ctrl)
				d.cloud.ComputeClientFactory = mock_azclient.NewMockClientFactory(ctrl)
				d.cloud.ComputeClientFactory.(*mock_azclient.MockClientFactory).EXPECT().GetAccountClient().Return(mockStorageAccountsClient).AnyTimes()
				accountListKeysResult := []*armstorage.AccountKey{}
				rerr := fmt.Errorf("test")
				mockStorageAccountsClient.EXPECT().ListKeys(gomock.Any(), gomock.Any(), gomock.Any()).Return(accountListKeysResult, rerr).AnyTimes()
				d.cloud.ComputeClientFactory.(*mock_azclient.MockClientFactory).EXPECT().GetAccountClientForSub(gomock.Any()).Return(mockStorageAccountsClient, nil).AnyTimes()
				_, _, _, _, _, err := d.GetAuthEnv(context.TODO(), volumeID, "", attrib, secret)
				expectedErr := fmt.Errorf("no key for storage account(storageaccountname) under resource group(rg), err test")
				if !strings.EqualFold(err.Error(), expectedErr.Error()) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "valid request",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				attrib := map[string]string{
					subscriptionIDField:          "subID",
					resourceGroupField:           "rg",
					storageAccountField:          "accountname",
					storageAccountNameField:      "accountname",
					secretNameField:              "secretName",
					secretNamespaceField:         "sNS",
					containerNameField:           "containername",
					mountWithWITokenField:        "false",
					pvcNamespaceKey:              "pvcNSKey",
					getAccountKeyFromSecretField: "false",
					storageAuthTypeField:         "key",
					msiEndpointField:             "msiEndpoint",
					getLatestAccountKeyField:     "true",
					tenantIDField:                "tenantID",
					serviceAccountTokenField:     "serviceAccountToken",
				}
				secret := make(map[string]string)
				volumeID := "rg#f5713de20cde511e8ba4900#pvc-fuse-dynamic-17e43f84-f474-11e8-acd0-000d3a00df41"
				d.cloud = &storage.AccountRepo{}
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				mockStorageAccountsClient := mock_accountclient.NewMockInterface(ctrl)
				d.cloud.ComputeClientFactory = mock_azclient.NewMockClientFactory(ctrl)
				d.cloud.ComputeClientFactory.(*mock_azclient.MockClientFactory).EXPECT().GetAccountClient().Return(mockStorageAccountsClient).AnyTimes()
				s := "unit-test"
				accountkey := armstorage.AccountKey{Value: &s}
				list := []*armstorage.AccountKey{&accountkey}
				mockStorageAccountsClient.EXPECT().ListKeys(gomock.Any(), gomock.Any(), gomock.Any()).Return(list, nil).AnyTimes()
				d.cloud.ComputeClientFactory.(*mock_azclient.MockClientFactory).EXPECT().GetAccountClientForSub(gomock.Any()).Return(mockStorageAccountsClient, nil).AnyTimes()
				_, _, _, _, _, err := d.GetAuthEnv(context.TODO(), volumeID, "", attrib, secret)
				expectedErr := error(nil)
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "valid request with MSIAuthTypeAddsIdentityEnv",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{}
				d.cloud.Config.AzureAuthConfig = azclient.AzureAuthConfig{
					UserAssignedIdentityID: "unit-test-identity-id",
				}

				attrib := map[string]string{
					subscriptionIDField:          "subID",
					resourceGroupField:           "rg",
					storageAccountField:          "accountname",
					storageAccountNameField:      "accountname",
					secretNameField:              "secretName",
					secretNamespaceField:         "sNS",
					containerNameField:           "containername",
					mountWithWITokenField:        "false",
					pvcNamespaceKey:              "pvcNSKey",
					getAccountKeyFromSecretField: "false",
					storageAuthTypeField:         storageAuthTypeMSI,
					msiEndpointField:             "msiEndpoint",
					getLatestAccountKeyField:     "true",
				}
				secret := make(map[string]string)
				volumeID := "rg#f5713de20cde511e8ba4900#pvc-fuse-dynamic-17e43f84-f474-11e8-acd0-000d3a00df41"
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				mockStorageAccountsClient := mock_accountclient.NewMockInterface(ctrl)
				d.cloud.ComputeClientFactory = mock_azclient.NewMockClientFactory(ctrl)
				d.cloud.ComputeClientFactory.(*mock_azclient.MockClientFactory).EXPECT().GetAccountClient().Return(mockStorageAccountsClient).AnyTimes()
				s := "unit-test"
				accountkey := armstorage.AccountKey{Value: &s}
				list := []*armstorage.AccountKey{&accountkey}
				mockStorageAccountsClient.EXPECT().ListKeys(gomock.Any(), gomock.Any(), gomock.Any()).Return(list, nil).AnyTimes()
				d.cloud.ComputeClientFactory.(*mock_azclient.MockClientFactory).EXPECT().GetAccountClientForSub(gomock.Any()).Return(mockStorageAccountsClient, nil).AnyTimes()
				_, _, _, _, authEnv, err := d.GetAuthEnv(context.TODO(), volumeID, "", attrib, secret)
				assert.NoError(t, err)
				found := false
				for _, env := range authEnv {
					if env == "AZURE_STORAGE_IDENTITY_CLIENT_ID=unit-test-identity-id" {
						found = true
						break
					}
				}
				assert.True(t, found, "AZURE_STORAGE_IDENTITY_CLIENT_ID should be present in authEnv")
			},
		},
		{
			name: "invalid getLatestAccountKey value",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				attrib := map[string]string{
					getLatestAccountKeyField: "invalid",
				}
				secret := make(map[string]string)
				volumeID := "rg#f5713de20cde511e8ba4900#pvc-fuse-dynamic-17e43f84-f474-11e8-acd0-000d3a00df41"
				_, _, _, _, _, err := d.GetAuthEnv(context.TODO(), volumeID, "", attrib, secret)
				expectedErr := fmt.Errorf("invalid getlatestaccountkey: %s in volume context", "invalid")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "invalid mountWithWIToken value",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				attrib := map[string]string{
					mountWithWITokenField: "invalid",
				}
				secret := make(map[string]string)
				volumeID := "rg#f5713de20cde511e8ba4900#pvc-fuse-dynamic-17e43f84-f474-11e8-acd0-000d3a00df41"
				_, _, _, _, _, err := d.GetAuthEnv(context.TODO(), volumeID, "", attrib, secret)
				expectedErr := fmt.Errorf("invalid %s: %s in volume context", mountWithWITokenField, "invalid")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "secret not empty",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				attrib := make(map[string]string)
				secret := make(map[string]string)
				volumeID := "rg#f5713de20cde511e8ba4900#containername"
				secret["accountname"] = "accountname"
				secret["azurestorageaccountname"] = "accountname"
				secret["accountkey"] = "unit-test"
				secret["azurestorageaccountkey"] = "unit-test"
				secret["azurestorageaccountsastoken"] = "unit-test"
				secret["msisecret"] = "unit-test"
				secret["azurestoragespnclientsecret"] = "unit-test"
				rg, accountName, accountkey, containerName, _, err := d.GetAuthEnv(context.TODO(), volumeID, "", attrib, secret)
				if err != nil {
					t.Errorf("actualErr: (%v), expectedErr: nil", err)
				}
				assert.Equal(t, "rg", rg)
				assert.Equal(t, "accountname", accountName)
				assert.Equal(t, "unit-test", accountkey)
				assert.Equal(t, "containername", containerName)
			},
		},
		{
			name: "nfs protocol",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				attrib := make(map[string]string)
				secret := make(map[string]string)
				volumeID := "unique-volumeid"
				attrib[storageAccountField] = "accountname"
				attrib[containerNameField] = "containername"
				rg, accountName, accountkey, containerName, authEnv, err := d.GetAuthEnv(context.TODO(), volumeID, NFS, attrib, secret)
				if err != nil {
					t.Errorf("actualErr: (%v), expect no error", err)
				}

				assert.Equal(t, "", rg)
				assert.Equal(t, "accountname", accountName)
				assert.Equal(t, "", accountkey)
				assert.Equal(t, "containername", containerName)
				assert.Equal(t, len(authEnv), 0)
			},
		},
		{
			name: "failed to get keyvaultClient",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{}
				d.cloud.ResourceGroup = "rg"
				attrib := make(map[string]string)
				secret := make(map[string]string)
				volumeID := "unique-volumeid"
				attrib[keyVaultURLField] = "kvURL"
				attrib[storageAccountField] = "accountname"
				attrib[containerNameField] = "containername"
				_, _, _, _, _, err := d.GetAuthEnv(context.TODO(), volumeID, "", attrib, secret)
				expectedErrStr := "parameter name cannot be empty"
				assert.Error(t, err)
				assert.Contains(t, err.Error(), expectedErrStr)
			},
		},
		{
			name: "valid request with all other attr",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				attrib := make(map[string]string)
				attrib[subscriptionIDField] = "subID"
				attrib[secretNameField] = "secretName"
				attrib[secretNamespaceField] = "sNS"
				attrib[pvcNamespaceKey] = "pvcNSKey"
				attrib[getAccountKeyFromSecretField] = "akFromSecret"

				secret := make(map[string]string)
				volumeID := "rg#f5713de20cde511e8ba4900#pvc-fuse-dynamic-17e43f84-f474-11e8-acd0-000d3a00df41"
				d.cloud = &storage.AccountRepo{}
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				mockStorageAccountsClient := mock_accountclient.NewMockInterface(ctrl)
				d.cloud.ComputeClientFactory = mock_azclient.NewMockClientFactory(ctrl)
				d.cloud.ComputeClientFactory.(*mock_azclient.MockClientFactory).EXPECT().GetAccountClient().Return(mockStorageAccountsClient).AnyTimes()
				s := "unit-test"
				accountkey := armstorage.AccountKey{Value: &s}
				list := []*armstorage.AccountKey{&accountkey}
				mockStorageAccountsClient.EXPECT().ListKeys(gomock.Any(), gomock.Any(), gomock.Any()).Return(list, nil).AnyTimes()
				d.cloud.ComputeClientFactory.(*mock_azclient.MockClientFactory).EXPECT().GetAccountClientForSub(gomock.Any()).Return(mockStorageAccountsClient, nil).AnyTimes()
				rg, accountName, _, containerName, _, err := d.GetAuthEnv(context.TODO(), volumeID, "", attrib, secret)
				expectedErr := error(nil)
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
				assert.Equal(t, rg, "rg")
				assert.Equal(t, "f5713de20cde511e8ba4900", accountName)
				assert.Equal(t, "pvc-fuse-dynamic-17e43f84-f474-11e8-acd0-000d3a00df41", containerName)
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func TestGetStorageAccountAndContainer(t *testing.T) {
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "Get storage access key error",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				attrib := make(map[string]string)
				secret := make(map[string]string)
				attrib["containername"] = "unit-test"
				attrib["keyvaultsecretname"] = "unit-test"
				attrib["keyvaultsecretversion"] = "unit-test"
				attrib["storageaccountname"] = "unit-test"
				volumeID := "rg#f5713de20cde511e8ba4900#pvc-fuse-dynamic-17e43f84-f474-11e8-acd0-000d3a00df41"
				d.cloud = &storage.AccountRepo{}
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				mockStorageAccountsClient := mock_accountclient.NewMockInterface(ctrl)
				d.cloud.ComputeClientFactory = mock_azclient.NewMockClientFactory(ctrl)
				d.cloud.ComputeClientFactory.(*mock_azclient.MockClientFactory).EXPECT().GetAccountClient().Return(mockStorageAccountsClient).AnyTimes()
				accountListKeysResult := []*armstorage.AccountKey{}
				rerr := fmt.Errorf("test")
				mockStorageAccountsClient.EXPECT().ListKeys(gomock.Any(), gomock.Any(), gomock.Any()).Return(accountListKeysResult, rerr).AnyTimes()
				d.cloud.ComputeClientFactory.(*mock_azclient.MockClientFactory).EXPECT().GetAccountClientForSub(gomock.Any()).Return(mockStorageAccountsClient, nil).AnyTimes()
				_, _, _, _, err := d.GetStorageAccountAndContainer(context.TODO(), volumeID, attrib, secret)
				expectedErr := fmt.Errorf("no key for storage account(f5713de20cde511e8ba4900) under resource group(rg), err test")
				if !strings.EqualFold(err.Error(), expectedErr.Error()) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "valid request",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				attrib := make(map[string]string)
				secret := make(map[string]string)
				volumeID := "rg#f5713de20cde511e8ba4900#pvc-fuse-dynamic-17e43f84-f474-11e8-acd0-000d3a00df41"
				d.cloud = &storage.AccountRepo{}
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				mockStorageAccountsClient := mock_accountclient.NewMockInterface(ctrl)
				d.cloud.ComputeClientFactory = mock_azclient.NewMockClientFactory(ctrl)
				d.cloud.ComputeClientFactory.(*mock_azclient.MockClientFactory).EXPECT().GetAccountClient().Return(mockStorageAccountsClient).AnyTimes()
				s := "unit-test"
				accountkey := armstorage.AccountKey{Value: &s}
				list := []*armstorage.AccountKey{&accountkey}
				mockStorageAccountsClient.EXPECT().ListKeys(gomock.Any(), gomock.Any(), gomock.Any()).Return(list, nil).AnyTimes()
				d.cloud.ComputeClientFactory.(*mock_azclient.MockClientFactory).EXPECT().GetAccountClientForSub(gomock.Any()).Return(mockStorageAccountsClient, nil).AnyTimes()
				_, _, _, _, err := d.GetStorageAccountAndContainer(context.TODO(), volumeID, attrib, secret)
				expectedErr := error(nil)
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "invalid getLatestAccountKey value",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				attrib := map[string]string{
					getLatestAccountKeyField: "invalid",
				}
				secret := make(map[string]string)
				volumeID := "rg#f5713de20cde511e8ba4900#pvc-fuse-dynamic-17e43f84-f474-11e8-acd0-000d3a00df41"
				d.cloud = &storage.AccountRepo{}
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				mockStorageAccountsClient := mock_accountclient.NewMockInterface(ctrl)
				d.cloud.ComputeClientFactory = mock_azclient.NewMockClientFactory(ctrl)
				d.cloud.ComputeClientFactory.(*mock_azclient.MockClientFactory).EXPECT().GetAccountClient().Return(mockStorageAccountsClient).AnyTimes()
				s := "unit-test"
				accountkey := armstorage.AccountKey{Value: &s}
				list := []*armstorage.AccountKey{&accountkey}
				mockStorageAccountsClient.EXPECT().ListKeys(gomock.Any(), gomock.Any(), gomock.Any()).Return(list, nil).AnyTimes()
				_, _, _, _, err := d.GetStorageAccountAndContainer(context.TODO(), volumeID, attrib, secret)
				expectedErr := fmt.Errorf("invalid getlatestaccountkey: %s in volume context", "invalid")
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

func TestGetStorageAccount(t *testing.T) {
	emptyAccountKeyMap := map[string]string{
		"accountname": "testaccount",
		"accountkey":  "",
	}

	emptyAccountNameMap := map[string]string{
		"azurestorageaccountname": "",
		"azurestorageaccountkey":  "testkey",
	}

	emptyAzureAccountKeyMap := map[string]string{
		"azurestorageaccountname": "testaccount",
		"azurestorageaccountkey":  "",
	}

	emptyAzureAccountNameMap := map[string]string{
		"azurestorageaccountname": "",
		"azurestorageaccountkey":  "testkey",
	}

	tests := []struct {
		options             map[string]string
		expectedAccountName string
		expectedAccountKey  string
		expectedError       error
	}{
		{
			options: map[string]string{
				"accountname": "testaccount",
				"accountkey":  "testkey",
			},
			expectedAccountName: "testaccount",
			expectedAccountKey:  "testkey",
			expectedError:       nil,
		},
		{
			options: map[string]string{
				"azurestorageaccountname": "testaccount",
				"azurestorageaccountkey":  "testkey",
			},
			expectedAccountName: "testaccount",
			expectedAccountKey:  "testkey",
			expectedError:       nil,
		},
		{
			options: map[string]string{
				"accountname": "",
				"accountkey":  "",
			},
			expectedAccountName: "",
			expectedAccountKey:  "",
			expectedError:       fmt.Errorf("could not find accountname or azurestorageaccountname field"),
		},
		{
			options:             emptyAccountKeyMap,
			expectedAccountName: "testaccount",
			expectedAccountKey:  "",
			expectedError:       fmt.Errorf("could not find accountkey or azurestorageaccountkey field in secrets"),
		},
		{
			options:             emptyAccountNameMap,
			expectedAccountName: "",
			expectedAccountKey:  "testkey",
			expectedError:       fmt.Errorf("could not find accountname or azurestorageaccountname field in secrets"),
		},
		{
			options:             emptyAzureAccountKeyMap,
			expectedAccountName: "testaccount",
			expectedAccountKey:  "",
			expectedError:       fmt.Errorf("could not find accountkey or azurestorageaccountkey field in secrets"),
		},
		{
			options:             emptyAzureAccountNameMap,
			expectedAccountName: "",
			expectedAccountKey:  "testkey",
			expectedError:       fmt.Errorf("could not find accountname or azurestorageaccountname field in secrets"),
		},
		{
			options:             nil,
			expectedAccountName: "",
			expectedAccountKey:  "",
			expectedError:       fmt.Errorf("unexpected: getStorageAccount secrets is nil"),
		},
	}

	for _, test := range tests {
		accountName, accountKey, err := getStorageAccount(test.options)
		if !reflect.DeepEqual(accountName, test.expectedAccountName) || !reflect.DeepEqual(accountKey, test.expectedAccountKey) {
			t.Errorf("input: %q, getStorageAccount accountName: %q, expectedAccountName: %q, accountKey: %q, expectedAccountKey: %q, err: %q, expectedError: %q", test.options, accountName, test.expectedAccountName, accountKey, test.expectedAccountKey,
				err, test.expectedError)
		} else {
			if accountName == "" || accountKey == "" {
				assert.Error(t, err)
			}
		}
	}
}

// needs editing, could only get past first error for testing, could not get a fake environment running
func TestGetContainerReference(t *testing.T) {
	fakeAccountName := "storageaccountname"
	fakeAccountKey := "dGVzdC1rZXkK"
	fakeContainerName := "test-con"
	testCases := []struct {
		name           string
		containerName  string
		endpointSuffix string
		secrets        map[string]string
		expectedError  error
	}{
		{
			name:          "failed to retrieve accountName",
			containerName: fakeContainerName,
			secrets: map[string]string{
				"accountKey": fakeAccountKey,
			},
			expectedError: fmt.Errorf("could not find %s or %s field in secrets", accountNameField, defaultSecretAccountName),
		},
		{
			name:          "failed to retrieve accountKey",
			containerName: fakeContainerName,
			secrets: map[string]string{
				"accountName": fakeAccountName,
			},
			expectedError: fmt.Errorf("could not find %s or %s field in secrets", accountKeyField, defaultSecretAccountKey),
		},
		{
			name:          "failed to obtain client",
			containerName: fakeContainerName,
			secrets: map[string]string{
				"accountName": fakeAccountName,
				"accountKey":  fakeAccountKey,
			},
			expectedError: fmt.Errorf("azure: base storage service url required"),
		},
		{
			name:           "Successful I/O",
			containerName:  fakeContainerName,
			endpointSuffix: "endpointSuffix",
			secrets: map[string]string{
				"accountName": "devstoreaccount1",
				"accountKey":  fakeAccountKey,
			},
			expectedError: nil,
		},
	}

	d := NewFakeDriver()
	d.cloud = &storage.AccountRepo{
		Environment: &azclient.Environment{},
	}
	d.KubeClient = fake.NewSimpleClientset()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := getContainerReference(tc.containerName, tc.secrets, tc.endpointSuffix)
			assert.Equal(t, tc.expectedError, err)
		})
	}
}

func TestSetAzureCredentials(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()

	tests := []struct {
		desc            string
		kubeClient      kubernetes.Interface
		accountName     string
		accountKey      string
		secretNamespace string
		expectedName    string
		expectedErr     error
	}{
		{
			desc:        "[failure] accountName is nil",
			kubeClient:  fakeClient,
			expectedErr: fmt.Errorf("the account info is not enough, accountName(), accountKey()"),
		},
		{
			desc:        "[failure] accountKey is nil",
			kubeClient:  fakeClient,
			accountName: "testName",
			accountKey:  "",
			expectedErr: fmt.Errorf("the account info is not enough, accountName(testName), accountKey()"),
		},
		{
			desc:        "[success] kubeClient is nil",
			kubeClient:  nil,
			expectedErr: nil,
		},
		{
			desc:         "[success] normal scenario",
			kubeClient:   fakeClient,
			accountName:  "testName",
			accountKey:   "testKey",
			expectedName: "azure-storage-account-testName-secret",
			expectedErr:  nil,
		},
		{
			desc:         "[success] already exist",
			kubeClient:   fakeClient,
			accountName:  "testName",
			accountKey:   "testKey",
			expectedName: "azure-storage-account-testName-secret",
			expectedErr:  nil,
		},
	}

	for _, test := range tests {
		result, err := setAzureCredentials(context.TODO(), test.kubeClient, test.accountName, test.accountKey, test.secretNamespace)
		if result != test.expectedName || !reflect.DeepEqual(err, test.expectedErr) {
			t.Errorf("desc: %s,\n input: kubeClient(%v), accountName(%v), accountKey(%v),\n setAzureCredentials result: %v, expectedName: %v err: %v, expectedErr: %v",
				test.desc, test.kubeClient, test.accountName, test.accountKey, result, test.expectedName, err, test.expectedErr)
		}
	}
}

func TestGetStorageAccesskey(t *testing.T) {
	options := &storage.AccountOptions{
		Name:           "test-sa",
		SubscriptionID: "test-subID",
		ResourceGroup:  "test-rg",
	}
	fakeAccName := options.Name
	fakeAccKey := "test-key"
	secretNamespace := "test-ns"
	testCases := []struct {
		name          string
		secrets       map[string]string
		secretName    string
		expectedError error
	}{
		{
			name: "Secrets is larger than 0",
			secrets: map[string]string{
				"accountName":              fakeAccName,
				"accountNameField":         fakeAccName,
				"defaultSecretAccountName": fakeAccName,
				"accountKey":               fakeAccKey,
				"accountKeyField":          fakeAccKey,
				"defaultSecretAccountKey":  fakeAccKey,
			},
			expectedError: nil,
		},
		{
			name:          "secretName is Empty",
			secrets:       make(map[string]string),
			secretName:    "",
			expectedError: nil,
		},
		{
			name:          "error is not nil",
			secrets:       make(map[string]string),
			secretName:    "foobar",
			expectedError: errors.New(""),
		},
		{
			name:          "successful input/error is nil",
			secrets:       make(map[string]string),
			secretName:    fmt.Sprintf(secretNameTemplate, options.Name),
			expectedError: nil,
		},
	}
	d := NewFakeDriver()
	d.KubeClient = fake.NewSimpleClientset()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStorageAccountsClient := mock_accountclient.NewMockInterface(ctrl)
	d.cloud.ComputeClientFactory = mock_azclient.NewMockClientFactory(gomock.NewController(t))
	d.cloud.ComputeClientFactory.(*mock_azclient.MockClientFactory).EXPECT().GetAccountClient().Return(mockStorageAccountsClient).AnyTimes()
	accountListKeysResult := []*armstorage.AccountKey{}
	mockStorageAccountsClient.EXPECT().ListKeys(gomock.Any(), gomock.Any(), gomock.Any()).Return(accountListKeysResult, nil).AnyTimes()
	d.cloud.ComputeClientFactory.(*mock_azclient.MockClientFactory).EXPECT().GetAccountClientForSub(gomock.Any()).Return(mockStorageAccountsClient, nil).AnyTimes()
	secret := &v1api.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: secretNamespace,
			Name:      fmt.Sprintf(secretNameTemplate, options.Name),
		},
		Data: map[string][]byte{
			defaultSecretAccountName: []byte(fakeAccName),
			defaultSecretAccountKey:  []byte(fakeAccKey),
		},
		Type: "Opaque",
	}
	secret.Namespace = secretNamespace
	_, secretCreateErr := d.KubeClient.CoreV1().Secrets(secretNamespace).Create(context.TODO(), secret, metav1.CreateOptions{})
	if secretCreateErr != nil {
		t.Error("failed to create secret")
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			accName, accKey, err := d.GetStorageAccesskey(context.TODO(), options, tc.secrets, tc.secretName, secretNamespace)
			if tc.expectedError != nil {
				assert.Error(t, err, "there should be an error")
			} else {
				assert.Equal(t, nil, err, "error should be nil")
				assert.Equal(t, fakeAccName, accName, "account names must match")
				assert.Equal(t, fakeAccKey, accKey, "account keys must match")
			}
		})
	}
}

func TestGetStorageAccesskeyWithSubsID(t *testing.T) {
	testCases := []struct {
		name          string
		expectedError error
	}{
		{
			name:          "Get storage access key error with cloud is nil",
			expectedError: fmt.Errorf("could not get account key: cloud or ComputeClientFactory is nil"),
		},
		{
			name:          "Get storage access key error with ComputeClientFactory is nil",
			expectedError: fmt.Errorf("could not get account key: cloud or ComputeClientFactory is nil"),
		},
		{
			name:          "Get storage access key successfully",
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		d := NewFakeDriver()
		if !strings.Contains(tc.name, "is nil") {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockStorageAccountsClient := mock_accountclient.NewMockInterface(ctrl)
			d.cloud.ComputeClientFactory = mock_azclient.NewMockClientFactory(ctrl)
			d.cloud.ComputeClientFactory.(*mock_azclient.MockClientFactory).EXPECT().GetAccountClient().Return(mockStorageAccountsClient).AnyTimes()
			s := "unit-test"
			accountkey := armstorage.AccountKey{Value: &s}
			mockStorageAccountsClient.EXPECT().ListKeys(gomock.Any(), gomock.Any(), gomock.Any()).Return([]*armstorage.AccountKey{&accountkey}, tc.expectedError).AnyTimes()
			d.cloud.ComputeClientFactory.(*mock_azclient.MockClientFactory).EXPECT().GetAccountClientForSub(gomock.Any()).Return(mockStorageAccountsClient, nil).AnyTimes()
		}
		_, err := d.GetStorageAccesskeyWithSubsID(context.TODO(), "test-subID", "test-rg", "test-sa", true)
		assert.Equal(t, tc.expectedError, err)
	}
}

func TestGetInfoFromSecret(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "KubeClient is nil",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{}
				d.KubeClient = nil
				secretName := "foo"
				secretNamespace := "bar"
				_, _, _, _, _, _, _, err := d.GetInfoFromSecret(context.TODO(), secretName, secretNamespace)
				expectedErr := fmt.Errorf("could not get account key from secret(%s): KubeClient is nil", secretName)
				if assert.Error(t, err) {
					assert.Equal(t, expectedErr, err)
				}
			},
		},
		{
			name: "Could not get secret",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.KubeClient = fakeClient
				secretName := ""
				secretNamespace := ""
				_, _, _, _, _, _, _, err := d.GetInfoFromSecret(context.TODO(), secretName, secretNamespace)
				// expectedErr := fmt.Errorf("could not get secret(%v): %w", secretName, err)
				assert.Error(t, err) // could not check what type of error, needs fix
				/*if assert.Error(t, err) {
					assert.Equal(t, expectedErr, err)
				}*/
			},
		},
		{
			name: "get account name from secret",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.KubeClient = fakeClient
				secretName := "store_account_name_key"
				secretNamespace := "namespace"
				accountName := "bar"
				accountKey := "foo"
				secret := &v1api.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: secretNamespace,
						Name:      secretName,
					},
					Data: map[string][]byte{
						defaultSecretAccountName: []byte(accountName),
						defaultSecretAccountKey:  []byte(accountKey),
					},
					Type: "Opaque",
				}
				_, secretCreateErr := d.KubeClient.CoreV1().Secrets(secretNamespace).Create(context.TODO(), secret, metav1.CreateOptions{})
				if secretCreateErr != nil {
					t.Error("failed to create secret")
				}
				an, ak, accountSasToken, msiSecret, storageSPNClientSecret, storageSPNClientID, storageSPNTenantID, err := d.GetInfoFromSecret(context.TODO(), secretName, secretNamespace)
				assert.Equal(t, accountName, an, "accountName should match")
				assert.Equal(t, accountKey, ak, "accountKey should match")
				assert.Equal(t, "", accountSasToken, "accountSasToken should be empty")
				assert.Equal(t, "", msiSecret, "msiSecret should be empty")
				assert.Equal(t, "", storageSPNClientSecret, "storageSPNClientSecret should be empty")
				assert.Equal(t, "", storageSPNClientID, "storageSPNClientID should be empty")
				assert.Equal(t, "", storageSPNTenantID, "storageSPNTenantID should be empty")
				assert.Equal(t, nil, err, "error should be nil")
			},
		},
		{
			name: "get other info from secret",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{}
				d.KubeClient = fakeClient
				secretName := "store_other_info"
				secretNamespace := "namespace"
				accountName := "bar"
				accountSasTokenValue := "foo"
				msiSecretValue := "msiSecret"
				storageSPNClientSecretValue := "storageSPNClientSecret"
				storageSPNClientIDValue := "storageSPNClientID"
				storageSPNTenantIDValue := "storageSPNTenantID"
				secret := &v1api.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: secretNamespace,
						Name:      secretName,
					},
					Data: map[string][]byte{
						defaultSecretAccountName:    []byte(accountName),
						accountSasTokenField:        []byte(accountSasTokenValue),
						msiSecretField:              []byte(msiSecretValue),
						storageSPNClientSecretField: []byte(storageSPNClientSecretValue),
						storageSPNClientIDField:     []byte(storageSPNClientIDValue),
						storageSPNTenantIDField:     []byte(storageSPNTenantIDValue),
					},
					Type: "Opaque",
				}
				_, secretCreateErr := d.KubeClient.CoreV1().Secrets(secretNamespace).Create(context.TODO(), secret, metav1.CreateOptions{})
				if secretCreateErr != nil {
					t.Error("failed to create secret")
				}
				an, ak, accountSasToken, msiSecret, storageSPNClientSecret, storageSPNClientID, storageSPNTenantID, err := d.GetInfoFromSecret(context.TODO(), secretName, secretNamespace)
				assert.Equal(t, accountName, an, "accountName should match")
				assert.Equal(t, "", ak, "accountKey should be empty")
				assert.Equal(t, accountSasTokenValue, accountSasToken, "sasToken should match")
				assert.Equal(t, msiSecretValue, msiSecret, "msiSecret should match")
				assert.Equal(t, storageSPNClientSecretValue, storageSPNClientSecret, "storageSPNClientSecret should match")
				assert.Equal(t, storageSPNClientIDValue, storageSPNClientID, "storageSPNClientID should match")
				assert.Equal(t, storageSPNTenantIDValue, storageSPNTenantID, "storageSPNTenantID should match")
				assert.Equal(t, nil, err, "error should be nil")
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func TestGetSubnetResourceID(t *testing.T) {
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "NetworkResourceSubscriptionID is Empty",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{}
				d.cloud.SubscriptionID = "fakeSubID"
				d.cloud.NetworkResourceSubscriptionID = ""
				d.cloud.ResourceGroup = "foo"
				d.cloud.VnetResourceGroup = "foo"
				actualOutput := d.getSubnetResourceID("", "", "")
				expectedOutput := fmt.Sprintf(subnetTemplate, d.cloud.SubscriptionID, "foo", d.cloud.VnetName, d.cloud.SubnetName)
				assert.Equal(t, actualOutput, expectedOutput, "config.SubscriptionID should be used as the SubID")
			},
		},
		{
			name: "NetworkResourceSubscriptionID is not Empty",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{}
				d.cloud.SubscriptionID = "fakeSubID"
				d.cloud.NetworkResourceSubscriptionID = "fakeNetSubID"
				d.cloud.ResourceGroup = "foo"
				d.cloud.VnetResourceGroup = "foo"
				actualOutput := d.getSubnetResourceID("", "", "")
				expectedOutput := fmt.Sprintf(subnetTemplate, d.cloud.NetworkResourceSubscriptionID, "foo", d.cloud.VnetName, d.cloud.SubnetName)
				assert.Equal(t, actualOutput, expectedOutput, "cloud.NetworkResourceSubscriptionID should be used as the SubID")
			},
		},
		{
			name: "VnetResourceGroup is Empty",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{}
				d.cloud.SubscriptionID = "bar"
				d.cloud.NetworkResourceSubscriptionID = "bar"
				d.cloud.ResourceGroup = "fakeResourceGroup"
				d.cloud.VnetResourceGroup = ""
				actualOutput := d.getSubnetResourceID("", "", "")
				expectedOutput := fmt.Sprintf(subnetTemplate, "bar", d.cloud.ResourceGroup, d.cloud.VnetName, d.cloud.SubnetName)
				assert.Equal(t, actualOutput, expectedOutput, "config.ResourceGroup should be used as the rg")
			},
		},
		{
			name: "VnetResourceGroup is not Empty",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{}
				d.cloud.SubscriptionID = "bar"
				d.cloud.NetworkResourceSubscriptionID = "bar"
				d.cloud.ResourceGroup = "fakeResourceGroup"
				d.cloud.VnetResourceGroup = "fakeVnetResourceGroup"
				actualOutput := d.getSubnetResourceID("", "", "")
				expectedOutput := fmt.Sprintf(subnetTemplate, "bar", d.cloud.VnetResourceGroup, d.cloud.VnetName, d.cloud.SubnetName)
				assert.Equal(t, actualOutput, expectedOutput, "config.VnetResourceGroup should be used as the rg")
			},
		},
		{
			name: "VnetResourceGroup, vnetName, subnetName is specified",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{}
				d.cloud.SubscriptionID = "bar"
				d.cloud.NetworkResourceSubscriptionID = "bar"
				d.cloud.ResourceGroup = "fakeResourceGroup"
				d.cloud.VnetResourceGroup = "fakeVnetResourceGroup"
				actualOutput := d.getSubnetResourceID("vnetrg", "vnetName", "subnetName")
				expectedOutput := fmt.Sprintf(subnetTemplate, "bar", "vnetrg", "vnetName", "subnetName")
				assert.Equal(t, actualOutput, expectedOutput, "VnetResourceGroup, vnetName, subnetName is specified")
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func TestUseDataPlaneAPI(t *testing.T) {
	fakeVolumeID := "unit-test-id"
	fakeAccountName := "unit-test-account"
	ctx := context.Background()
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "volumeID loads correctly",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.dataPlaneAPIVolCache.Set(fakeVolumeID, "foo")
				output := d.useDataPlaneAPI(ctx, fakeVolumeID, "")
				if !output {
					t.Errorf("Actual Output: %t, Expected Output: %t", output, true)
				}
			},
		},
		{
			name: "account loads correctly",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.dataPlaneAPIVolCache.Set(fakeAccountName, "foo")
				output := d.useDataPlaneAPI(ctx, fakeAccountName, "")
				if !output {
					t.Errorf("Actual Output: %t, Expected Output: %t", output, true)
				}
			},
		},
		{
			name: "invalid volumeID and account",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				output := d.useDataPlaneAPI(ctx, "", "")
				if output {
					t.Errorf("Actual Output: %t, Expected Output: %t", output, false)
				}
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func TestAppendDefaultMountOptions(t *testing.T) {
	tests := []struct {
		options       []string
		tmpPath       string
		containerName string
		expected      []string
	}{
		{
			options:       []string{"targetPath"},
			tmpPath:       "/tmp",
			containerName: "containerName",
			expected: []string{"--cancel-list-on-mount-seconds=10",
				"--container-name=containerName",
				"--pre-mount-validate=true",
				"--empty-dir-check=false",
				"--tmp-path=/tmp",
				"--use-https=true",
				"targetPath",
			},
		},
		{
			options:       []string{"targetPath", "--cancel-list-on-mount-seconds=0", "--pre-mount-validate=false"},
			tmpPath:       "/tmp",
			containerName: "containerName",
			expected: []string{"--cancel-list-on-mount-seconds=0",
				"--container-name=containerName",
				"--pre-mount-validate=false",
				"--empty-dir-check=false",
				"--tmp-path=/tmp",
				"--use-https=true",
				"targetPath",
			},
		},
		{
			options:       []string{"targetPath", "--tmp-path=/var/log", "--pre-mount-validate=false"},
			tmpPath:       "/tmp",
			containerName: "containerName",
			expected: []string{"--cancel-list-on-mount-seconds=10",
				"--container-name=containerName",
				"--pre-mount-validate=false",
				"--empty-dir-check=false",
				"--tmp-path=/var/log",
				"--use-https=true",
				"targetPath",
			},
		},
	}

	for _, test := range tests {
		result := appendDefaultMountOptions(test.options, test.tmpPath, test.containerName)
		sort.Strings(result)
		sort.Strings(test.expected)

		if !reflect.DeepEqual(result, test.expected) {
			t.Errorf("input: %q, appendDefaultMountOptions result: %q, expected: %q", test.options, result, test.expected)
		}
	}
}

func TestIsSupportedContainerNamePrefix(t *testing.T) {
	tests := []struct {
		prefix         string
		expectedResult bool
	}{
		{
			prefix:         "",
			expectedResult: true,
		},
		{
			prefix:         "ext3",
			expectedResult: true,
		},
		{
			prefix:         "ext-2",
			expectedResult: true,
		},
		{
			prefix:         "-xfs",
			expectedResult: false,
		},
		{
			prefix:         "Absdf",
			expectedResult: false,
		},
		{
			prefix:         "tooooooooooooooooooooooooolong",
			expectedResult: false,
		},
		{
			prefix:         "+invalid",
			expectedResult: false,
		},
		{
			prefix:         " invalidspace",
			expectedResult: false,
		},
	}

	for _, test := range tests {
		result := isSupportedContainerNamePrefix(test.prefix)
		if result != test.expectedResult {
			t.Errorf("isSupportedContainerNamePrefix(%s) returned with %v, not equal to %v", test.prefix, result, test.expectedResult)
		}
	}
}

func TestChmodIfPermissionMismatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test on Windows")
	}

	permissionMatchingPath, _ := getWorkDirPath("permissionMatchingPath")
	_ = makeDir(permissionMatchingPath)
	defer os.RemoveAll(permissionMatchingPath)

	permissionMismatchPath, _ := getWorkDirPath("permissionMismatchPath")
	_ = os.MkdirAll(permissionMismatchPath, os.FileMode(0721))
	defer os.RemoveAll(permissionMismatchPath)

	permissionMatchGidMismatchPath, _ := getWorkDirPath("permissionMatchGidMismatchPath")
	_ = os.MkdirAll(permissionMatchGidMismatchPath, os.FileMode(0755))
	_ = os.Chmod(permissionMatchGidMismatchPath, 0755|os.ModeSetgid) // Setgid bit is set
	defer os.RemoveAll(permissionMatchGidMismatchPath)

	permissionMismatchGidMismatch, _ := getWorkDirPath("permissionMismatchGidMismatch")
	_ = os.MkdirAll(permissionMismatchGidMismatch, os.FileMode(0721))
	_ = os.Chmod(permissionMismatchGidMismatch, 0721|os.ModeSetgid) // Setgid bit is set
	defer os.RemoveAll(permissionMismatchGidMismatch)

	tests := []struct {
		desc           string
		path           string
		mode           os.FileMode
		expectedPerms  os.FileMode
		expectedGidBit bool
		expectedError  error
	}{
		{
			desc:          "Invalid path",
			path:          "invalid-path",
			mode:          0755,
			expectedError: fmt.Errorf("CreateFile invalid-path: The system cannot find the file specified"),
		},
		{
			desc:           "permission matching path",
			path:           permissionMatchingPath,
			mode:           0755,
			expectedPerms:  0755,
			expectedGidBit: false,
			expectedError:  nil,
		},
		{
			desc:           "permission mismatch path",
			path:           permissionMismatchPath,
			mode:           0755,
			expectedPerms:  0755,
			expectedGidBit: false,
			expectedError:  nil,
		},
		{
			desc:           "permission mismatch path",
			path:           permissionMismatchPath,
			mode:           0755,
			expectedPerms:  0755,
			expectedGidBit: false,
			expectedError:  nil,
		},
		{
			desc:           "only match the permission mode bits",
			path:           permissionMatchGidMismatchPath,
			mode:           0755,
			expectedPerms:  0755,
			expectedGidBit: true,
			expectedError:  nil,
		},
		{
			desc:           "only change the permission mode bits when gid is set",
			path:           permissionMismatchGidMismatch,
			mode:           0755,
			expectedPerms:  0755,
			expectedGidBit: true,
			expectedError:  nil,
		},
		{
			desc:           "only change the permission mode bits when gid is not set but mode bits have gid set",
			path:           permissionMismatchPath,
			mode:           02755,
			expectedPerms:  0755,
			expectedGidBit: false,
			expectedError:  nil,
		},
	}

	for _, test := range tests {
		err := chmodIfPermissionMismatch(test.path, test.mode)
		if !reflect.DeepEqual(err, test.expectedError) {
			if err == nil || test.expectedError == nil && !strings.Contains(err.Error(), test.expectedError.Error()) {
				t.Errorf("test[%s]: unexpected error: %v, expected error: %v", test.desc, err, test.expectedError)
			}
		}

		if test.expectedError == nil {
			info, _ := os.Lstat(test.path)
			if test.expectedError == nil && (info.Mode()&os.ModePerm != test.expectedPerms) {
				t.Errorf("test[%s]: unexpected perms: %v, expected perms: %v, ", test.desc, info.Mode()&os.ModePerm, test.expectedPerms)
			}

			if (info.Mode()&os.ModeSetgid != 0) != test.expectedGidBit {
				t.Errorf("test[%s]: unexpected gid bit: %v, expected gid bit: %v", test.desc, info.Mode()&os.ModeSetgid != 0, test.expectedGidBit)
			}
		}
	}

}

// getWorkDirPath returns the path to the current working directory
func getWorkDirPath(dir string) (string, error) {
	path, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%c%s", path, os.PathSeparator, dir), nil
}

func TestCreateStorageAccountSecret(t *testing.T) {
	result := createStorageAccountSecret("TestAccountName", "TestAccountKey")
	if result[defaultSecretAccountName] != "TestAccountName" || result[defaultSecretAccountKey] != "TestAccountKey" {
		t.Errorf("Expected account name(%s), Actual account name(%s); Expected account key(%s), Actual account key(%s)", "TestAccountName", result[defaultSecretAccountName], "TestAccountKey", result[defaultSecretAccountKey])
	}
}

func TestSetKeyValueInMap(t *testing.T) {
	tests := []struct {
		desc     string
		m        map[string]string
		key      string
		value    string
		expected map[string]string
	}{
		{
			desc:  "nil map",
			key:   "key",
			value: "value",
		},
		{
			desc:     "empty map",
			m:        map[string]string{},
			key:      "key",
			value:    "value",
			expected: map[string]string{"key": "value"},
		},
		{
			desc:  "non-empty map",
			m:     map[string]string{"k": "v"},
			key:   "key",
			value: "value",
			expected: map[string]string{
				"k":   "v",
				"key": "value",
			},
		},
		{
			desc:     "same key already exists",
			m:        map[string]string{"subDir": "value2"},
			key:      "subDir",
			value:    "value",
			expected: map[string]string{"subDir": "value"},
		},
		{
			desc:     "case insensitive key already exists",
			m:        map[string]string{"subDir": "value2"},
			key:      "subdir",
			value:    "value",
			expected: map[string]string{"subDir": "value"},
		},
	}

	for _, test := range tests {
		setKeyValueInMap(test.m, test.key, test.value)
		if !reflect.DeepEqual(test.m, test.expected) {
			t.Errorf("test[%s]: unexpected output: %v, expected result: %v", test.desc, test.m, test.expected)
		}
	}
}

func TestGetValueInMap(t *testing.T) {
	tests := []struct {
		desc     string
		m        map[string]string
		key      string
		expected string
	}{
		{
			desc:     "nil map",
			key:      "key",
			expected: "",
		},
		{
			desc:     "empty map",
			m:        map[string]string{},
			key:      "key",
			expected: "",
		},
		{
			desc:     "non-empty map",
			m:        map[string]string{"k": "v"},
			key:      "key",
			expected: "",
		},
		{
			desc:     "same key already exists",
			m:        map[string]string{"subDir": "value2"},
			key:      "subDir",
			expected: "value2",
		},
		{
			desc:     "case insensitive key already exists",
			m:        map[string]string{"subDir": "value2"},
			key:      "subdir",
			expected: "value2",
		},
	}

	for _, test := range tests {
		result := getValueInMap(test.m, test.key)
		if result != test.expected {
			t.Errorf("test[%s]: unexpected output: %v, expected result: %v", test.desc, result, test.expected)
		}
	}
}

func TestReplaceWithMap(t *testing.T) {
	tests := []struct {
		desc     string
		str      string
		m        map[string]string
		expected string
	}{
		{
			desc:     "empty string",
			str:      "",
			expected: "",
		},
		{
			desc:     "empty map",
			str:      "",
			m:        map[string]string{},
			expected: "",
		},
		{
			desc:     "empty key",
			str:      "prefix-" + pvNameMetadata,
			m:        map[string]string{"": "pv"},
			expected: "prefix-" + pvNameMetadata,
		},
		{
			desc:     "empty value",
			str:      "prefix-" + pvNameMetadata,
			m:        map[string]string{pvNameMetadata: ""},
			expected: "prefix-",
		},
		{
			desc:     "one replacement",
			str:      "prefix-" + pvNameMetadata,
			m:        map[string]string{pvNameMetadata: "pv"},
			expected: "prefix-pv",
		},
		{
			desc:     "multiple replacements",
			str:      pvcNamespaceMetadata + pvcNameMetadata,
			m:        map[string]string{pvcNamespaceMetadata: "namespace", pvcNameMetadata: "pvcname"},
			expected: "namespacepvcname",
		},
	}

	for _, test := range tests {
		result := replaceWithMap(test.str, test.m)
		if result != test.expected {
			t.Errorf("test[%s]: unexpected output: %v, expected result: %v", test.desc, result, test.expected)
		}
	}
}

func TestIsSupportedAccessTier(t *testing.T) {
	tests := []struct {
		accessTier     string
		expectedResult bool
	}{
		{
			accessTier:     "",
			expectedResult: true,
		},
		{
			accessTier:     "TransactionOptimized",
			expectedResult: false,
		},
		{
			accessTier:     "Hot",
			expectedResult: true,
		},
		{
			accessTier:     "Cool",
			expectedResult: true,
		},
		{
			accessTier:     "Premium",
			expectedResult: true,
		},
		{
			accessTier:     "transactionOptimized",
			expectedResult: false,
		},
		{
			accessTier:     "premium",
			expectedResult: false,
		},
		{
			accessTier:     "unknown",
			expectedResult: false,
		},
	}

	for _, test := range tests {
		result := isSupportedAccessTier(test.accessTier)
		if result != test.expectedResult {
			t.Errorf("isSupportedTier(%s) returned with %v, not equal to %v", test.accessTier, result, test.expectedResult)
		}
	}
}

func TestIsNFSProtocol(t *testing.T) {
	tests := []struct {
		protocol       string
		expectedResult bool
	}{
		{
			protocol:       "",
			expectedResult: false,
		},
		{
			protocol:       "NFS",
			expectedResult: true,
		},
		{
			protocol:       "nfs",
			expectedResult: true,
		},
		{
			protocol:       "Nfs",
			expectedResult: true,
		},
		{
			protocol:       "nfsv3",
			expectedResult: true,
		},
		{
			protocol:       "aznfs",
			expectedResult: true,
		},
		{
			protocol:       "azNfs",
			expectedResult: true,
		},
	}

	for _, test := range tests {
		result := isNFSProtocol(test.protocol)
		if result != test.expectedResult {
			t.Errorf("isNFSVolume(%s) returned with %v, not equal to %v", test.protocol, result, test.expectedResult)
		}
	}
}

func TestDriverOptions_AddFlags(t *testing.T) {
	t.Run("test options", func(t *testing.T) {
		option := DriverOptions{}
		option.AddFlags()
		typeInfo := reflect.TypeOf(option)
		numOfExpectedOptions := typeInfo.NumField()
		count := 0
		flag.CommandLine.VisitAll(func(f *flag.Flag) {
			if !strings.Contains(f.Name, "test") {
				count++
			}
		})
		if numOfExpectedOptions != count {
			t.Errorf("expected %d flags, but found %d flag in DriverOptions", numOfExpectedOptions, count)
		}
	})
}

func TestIsSupportedFSGroupChangePolicy(t *testing.T) {
	tests := []struct {
		policy         string
		expectedResult bool
	}{
		{
			policy:         "",
			expectedResult: true,
		},
		{
			policy:         "None",
			expectedResult: true,
		},
		{
			policy:         "Always",
			expectedResult: true,
		},
		{
			policy:         "OnRootMismatch",
			expectedResult: true,
		},
		{
			policy:         "onRootMismatch",
			expectedResult: false,
		},
		{
			policy:         "invalid",
			expectedResult: false,
		},
	}

	for _, test := range tests {
		result := isSupportedFSGroupChangePolicy(test.policy)
		if result != test.expectedResult {
			t.Errorf("isSupportedFSGroupChangePolicy(%s) returned with %v, not equal to %v", test.policy, result, test.expectedResult)
		}
	}
}

func TestIsReadOnlyFromCapability(t *testing.T) {
	testCases := []struct {
		name           string
		vc             *csi.VolumeCapability
		expectedResult bool
	}{
		{
			name:           "false with empty capabilities",
			vc:             &csi.VolumeCapability{},
			expectedResult: false,
		},
		{
			name: "fail with capabilities no access mode",
			vc: &csi.VolumeCapability{
				AccessType: &csi.VolumeCapability_Mount{
					Mount: &csi.VolumeCapability_MountVolume{},
				},
			},
		},
		{
			name: "false with SINGLE_NODE_WRITER capabilities",
			vc: &csi.VolumeCapability{
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				},
			},
			expectedResult: false,
		},
		{
			name: "true with MULTI_NODE_READER_ONLY capabilities",
			vc: &csi.VolumeCapability{
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
				},
			},
			expectedResult: true,
		},
		{
			name: "true with SINGLE_NODE_READER_ONLY capabilities",
			vc: &csi.VolumeCapability{
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY,
				},
			},
			expectedResult: true,
		},
	}

	for _, test := range testCases {
		result := isReadOnlyFromCapability(test.vc)
		if result != test.expectedResult {
			t.Errorf("case(%s): isReadOnlyFromCapability returned with %v, not equal to %v", test.name, result, test.expectedResult)
		}
	}
}

func TestGenerateVolumeName(t *testing.T) {
	// Normal operation, no truncate
	v1 := generateVolumeName("kubernetes", "pv-cinder-abcde", 255)
	if v1 != "kubernetes-dynamic-pv-cinder-abcde" {
		t.Errorf("Expected kubernetes-dynamic-pv-cinder-abcde, got %s", v1)
	}
	// Truncate trailing "6789-dynamic"
	prefix := strings.Repeat("0123456789", 9) // 90 characters prefix + 8 chars. of "-dynamic"
	v2 := generateVolumeName(prefix, "pv-cinder-abcde", 100)
	expect := prefix[:84] + "-pv-cinder-abcde"
	if v2 != expect {
		t.Errorf("Expected %s, got %s", expect, v2)
	}
	// Truncate really long cluster name
	prefix = strings.Repeat("0123456789", 1000) // 10000 characters prefix
	v3 := generateVolumeName(prefix, "pv-cinder-abcde", 100)
	if v3 != expect {
		t.Errorf("Expected %s, got %s", expect, v3)
	}
}

func TestParseServiceAccountTokenError(t *testing.T) {
	cases := []struct {
		desc     string
		saTokens string
	}{
		{
			desc:     "empty serviceaccount tokens",
			saTokens: "",
		},
		{
			desc:     "invalid serviceaccount tokens",
			saTokens: "invalid",
		},
		{
			desc:     "token for audience not found",
			saTokens: `{"aud1":{"token":"eyJhbGciOiJSUzI1NiIsImtpZCI6InRhVDBxbzhQVEZ1ajB1S3BYUUxIclRsR01XakxjemJNOTlzWVMxSlNwbWcifQ.eyJhdWQiOlsiYXBpOi8vQXp1cmVBRGlUb2tlbkV4Y2hhbmdlIl0sImV4cCI6MTY0MzIzNDY0NywiaWF0IjoxNjQzMjMxMDQ3LCJpc3MiOiJodHRwczovL2t1YmVybmV0ZXMuZGVmYXVsdC5zdmMuY2x1c3Rlci5sb2NhbCIsImt1YmVybmV0ZXMuaW8iOnsibmFtZXNwYWNlIjoidGVzdC12MWFscGhhMSIsInBvZCI6eyJuYW1lIjoic2VjcmV0cy1zdG9yZS1pbmxpbmUtY3JkIiwidWlkIjoiYjBlYmZjMzUtZjEyNC00ZTEyLWI3N2UtYjM0MjM2N2IyMDNmIn0sInNlcnZpY2VhY2NvdW50Ijp7Im5hbWUiOiJkZWZhdWx0IiwidWlkIjoiMjViNGY1NzgtM2U4MC00NTczLWJlOGQtZTdmNDA5ZDI0MmI2In19LCJuYmYiOjE2NDMyMzEwNDcsInN1YiI6InN5c3RlbTpzZXJ2aWNlYWNjb3VudDp0ZXN0LXYxYWxwaGExOmRlZmF1bHQifQ.ALE46aKmtTV7dsuFOwDZqvEjdHFUTNP-JVjMxexTemmPA78fmPTUZF0P6zANumA03fjX3L-MZNR3PxmEZgKA9qEGIDsljLsUWsVBEquowuBh8yoBYkGkMJmRfmbfS3y7_4Q7AU3D9Drw4iAHcn1GwedjOQC0i589y3dkNNqf8saqHfXkbSSLtSE0f2uzI-PjuTKvR1kuojEVNKlEcA4wsKfoiRpkua17sHkHU0q9zxCMDCr_1f8xbigRnRx0wscU3vy-8KhF3zQtpcWkk3r4C5YSXut9F3xjz5J9DUQn2vNMfZg4tOdcR-9Xv9fbY5iujiSlS58GEktSEa3SE9wrCw\",\"expirationTimestamp\":\"2022-01-26T22:04:07Z\"},\"gcp\":{\"token\":\"eyJhbGciOiJSUzI1NiIsImtpZCI6InRhVDBxbzhQVEZ1ajB1S3BYUUxIclRsR01XakxjemJNOTlzWVMxSlNwbWcifQ.eyJhdWQiOlsiZ2NwIl0sImV4cCI6MTY0MzIzNDY0NywiaWF0IjoxNjQzMjMxMDQ3LCJpc3MiOiJodHRwczovL2t1YmVybmV0ZXMuZGVmYXVsdC5zdmMuY2x1c3Rlci5sb2NhbCIsImt1YmVybmV0ZXMuaW8iOnsibmFtZXNwYWNlIjoidGVzdC12MWFscGhhMSIsInBvZCI6eyJuYW1lIjoic2VjcmV0cy1zdG9yZS1pbmxpbmUtY3JkIiwidWlkIjoiYjBlYmZjMzUtZjEyNC00ZTEyLWI3N2UtYjM0MjM2N2IyMDNmIn0sInNlcnZpY2VhY2NvdW50Ijp7Im5hbWUiOiJkZWZhdWx0IiwidWlkIjoiMjViNGY1NzgtM2U4MC00NTczLWJlOGQtZTdmNDA5ZDI0MmI2In19LCJuYmYiOjE2NDMyMzEwNDcsInN1YiI6InN5c3RlbTpzZXJ2aWNlYWNjb3VudDp0ZXN0LXYxYWxwaGExOmRlZmF1bHQifQ.BT0YGI7bGdSNaIBqIEnVL0Ky5t-fynaemSGxjGdKOPl0E22UIVGDpAMUhaS19i20c-Dqs-Kn0N-R5QyDNpZg8vOL5KIFqu2kSYNbKxtQW7TPYIsV0d9wUZjLSr54DKrmyXNMGRoT2bwcF4yyfmO46eMmZSaXN8Y4lgapeabg6CBVVQYHD-GrgXf9jVLeJfCQkTuojK1iXOphyD6NqlGtVCaY1jWxbBMibN0q214vKvQboub8YMuvclGdzn_l_ZQSTjvhBj9I-W1t-JArVjqHoIb8_FlR9BSgzgL7V3Jki55vmiOdEYqMErJWrIZPP3s8qkU5hhO9rSVEd3LJHponvQ","expirationTimestamp":"2022-01-26T22:04:07Z"}}`, //nolint
		},
		{
			desc:     "token incorrect format",
			saTokens: `{"api://AzureADTokenExchange":{"tokens":"eyJhbGciOiJSUzI1NiIsImtpZCI6InRhVDBxbzhQVEZ1ajB1S3BYUUxIclRsR01XakxjemJNOTlzWVMxSlNwbWcifQ.eyJhdWQiOlsiYXBpOi8vQXp1cmVBRGlUb2tlbkV4Y2hhbmdlIl0sImV4cCI6MTY0MzIzNDY0NywiaWF0IjoxNjQzMjMxMDQ3LCJpc3MiOiJodHRwczovL2t1YmVybmV0ZXMuZGVmYXVsdC5zdmMuY2x1c3Rlci5sb2NhbCIsImt1YmVybmV0ZXMuaW8iOnsibmFtZXNwYWNlIjoidGVzdC12MWFscGhhMSIsInBvZCI6eyJuYW1lIjoic2VjcmV0cy1zdG9yZS1pbmxpbmUtY3JkIiwidWlkIjoiYjBlYmZjMzUtZjEyNC00ZTEyLWI3N2UtYjM0MjM2N2IyMDNmIn0sInNlcnZpY2VhY2NvdW50Ijp7Im5hbWUiOiJkZWZhdWx0IiwidWlkIjoiMjViNGY1NzgtM2U4MC00NTczLWJlOGQtZTdmNDA5ZDI0MmI2In19LCJuYmYiOjE2NDMyMzEwNDcsInN1YiI6InN5c3RlbTpzZXJ2aWNlYWNjb3VudDp0ZXN0LXYxYWxwaGExOmRlZmF1bHQifQ.ALE46aKmtTV7dsuFOwDZqvEjdHFUTNP-JVjMxexTemmPA78fmPTUZF0P6zANumA03fjX3L-MZNR3PxmEZgKA9qEGIDsljLsUWsVBEquowuBh8yoBYkGkMJmRfmbfS3y7_4Q7AU3D9Drw4iAHcn1GwedjOQC0i589y3dkNNqf8saqHfXkbSSLtSE0f2uzI-PjuTKvR1kuojEVNKlEcA4wsKfoiRpkua17sHkHU0q9zxCMDCr_1f8xbigRnRx0wscU3vy-8KhF3zQtpcWkk3r4C5YSXut9F3xjz5J9DUQn2vNMfZg4tOdcR-9Xv9fbY5iujiSlS58GEktSEa3SE9wrCw\",\"expirationTimestamp\":\"2022-01-26T22:04:07Z\"},\"gcp\":{\"token\":\"eyJhbGciOiJSUzI1NiIsImtpZCI6InRhVDBxbzhQVEZ1ajB1S3BYUUxIclRsR01XakxjemJNOTlzWVMxSlNwbWcifQ.eyJhdWQiOlsiZ2NwIl0sImV4cCI6MTY0MzIzNDY0NywiaWF0IjoxNjQzMjMxMDQ3LCJpc3MiOiJodHRwczovL2t1YmVybmV0ZXMuZGVmYXVsdC5zdmMuY2x1c3Rlci5sb2NhbCIsImt1YmVybmV0ZXMuaW8iOnsibmFtZXNwYWNlIjoidGVzdC12MWFscGhhMSIsInBvZCI6eyJuYW1lIjoic2VjcmV0cy1zdG9yZS1pbmxpbmUtY3JkIiwidWlkIjoiYjBlYmZjMzUtZjEyNC00ZTEyLWI3N2UtYjM0MjM2N2IyMDNmIn0sInNlcnZpY2VhY2NvdW50Ijp7Im5hbWUiOiJkZWZhdWx0IiwidWlkIjoiMjViNGY1NzgtM2U4MC00NTczLWJlOGQtZTdmNDA5ZDI0MmI2In19LCJuYmYiOjE2NDMyMzEwNDcsInN1YiI6InN5c3RlbTpzZXJ2aWNlYWNjb3VudDp0ZXN0LXYxYWxwaGExOmRlZmF1bHQifQ.BT0YGI7bGdSNaIBqIEnVL0Ky5t-fynaemSGxjGdKOPl0E22UIVGDpAMUhaS19i20c-Dqs-Kn0N-R5QyDNpZg8vOL5KIFqu2kSYNbKxtQW7TPYIsV0d9wUZjLSr54DKrmyXNMGRoT2bwcF4yyfmO46eMmZSaXN8Y4lgapeabg6CBVVQYHD-GrgXf9jVLeJfCQkTuojK1iXOphyD6NqlGtVCaY1jWxbBMibN0q214vKvQboub8YMuvclGdzn_l_ZQSTjvhBj9I-W1t-JArVjqHoIb8_FlR9BSgzgL7V3Jki55vmiOdEYqMErJWrIZPP3s8qkU5hhO9rSVEd3LJHponvQ","expirationTimestamp":"2022-01-26T22:04:07Z"}}`, //nolint
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			if _, err := parseServiceAccountToken(tc.saTokens); err == nil {
				t.Errorf("ParseServiceAccountToken(%s) = nil, want error", tc.saTokens)
			}
		})
	}
}

func TestParseServiceAccountToken(t *testing.T) {
	saTokens := `{"api://AzureADTokenExchange":{"token":"eyJhbGciOiJSUzI1NiIsImtpZCI6InRhVDBxbzhQVEZ1ajB1S3BYUUxIclRsR01XakxjemJNOTlzWVMxSlNwbWcifQ.eyJhdWQiOlsiYXBpOi8vQXp1cmVBRGlUb2tlbkV4Y2hhbmdlIl0sImV4cCI6MTY0MzIzNDY0NywiaWF0IjoxNjQzMjMxMDQ3LCJpc3MiOiJodHRwczovL2t1YmVybmV0ZXMuZGVmYXVsdC5zdmMuY2x1c3Rlci5sb2NhbCIsImt1YmVybmV0ZXMuaW8iOnsibmFtZXNwYWNlIjoidGVzdC12MWFscGhhMSIsInBvZCI6eyJuYW1lIjoic2VjcmV0cy1zdG9yZS1pbmxpbmUtY3JkIiwidWlkIjoiYjBlYmZjMzUtZjEyNC00ZTEyLWI3N2UtYjM0MjM2N2IyMDNmIn0sInNlcnZpY2VhY2NvdW50Ijp7Im5hbWUiOiJkZWZhdWx0IiwidWlkIjoiMjViNGY1NzgtM2U4MC00NTczLWJlOGQtZTdmNDA5ZDI0MmI2In19LCJuYmYiOjE2NDMyMzEwNDcsInN1YiI6InN5c3RlbTpzZXJ2aWNlYWNjb3VudDp0ZXN0LXYxYWxwaGExOmRlZmF1bHQifQ.ALE46aKmtTV7dsuFOwDZqvEjdHFUTNP-JVjMxexTemmPA78fmPTUZF0P6zANumA03fjX3L-MZNR3PxmEZgKA9qEGIDsljLsUWsVBEquowuBh8yoBYkGkMJmRfmbfS3y7_4Q7AU3D9Drw4iAHcn1GwedjOQC0i589y3dkNNqf8saqHfXkbSSLtSE0f2uzI-PjuTKvR1kuojEVNKlEcA4wsKfoiRpkua17sHkHU0q9zxCMDCr_1f8xbigRnRx0wscU3vy-8KhF3zQtpcWkk3r4C5YSXut9F3xjz5J9DUQn2vNMfZg4tOdcR-9Xv9fbY5iujiSlS58GEktSEa3SE9wrCw","expirationTimestamp":"2022-01-26T22:04:07Z"},"aud2":{"token":"eyJhbGciOiJSUzI1NiIsImtpZCI6InRhVDBxbzhQVEZ1ajB1S3BYUUxIclRsR01XakxjemJNOTlzWVMxSlNwbWcifQ.eyJhdWQiOlsiZ2NwIl0sImV4cCI6MTY0MzIzNDY0NywiaWF0IjoxNjQzMjMxMDQ3LCJpc3MiOiJodHRwczovL2t1YmVybmV0ZXMuZGVmYXVsdC5zdmMuY2x1c3Rlci5sb2NhbCIsImt1YmVybmV0ZXMuaW8iOnsibmFtZXNwYWNlIjoidGVzdC12MWFscGhhMSIsInBvZCI6eyJuYW1lIjoic2VjcmV0cy1zdG9yZS1pbmxpbmUtY3JkIiwidWlkIjoiYjBlYmZjMzUtZjEyNC00ZTEyLWI3N2UtYjM0MjM2N2IyMDNmIn0sInNlcnZpY2VhY2NvdW50Ijp7Im5hbWUiOiJkZWZhdWx0IiwidWlkIjoiMjViNGY1NzgtM2U4MC00NTczLWJlOGQtZTdmNDA5ZDI0MmI2In19LCJuYmYiOjE2NDMyMzEwNDcsInN1YiI6InN5c3RlbTpzZXJ2aWNlYWNjb3VudDp0ZXN0LXYxYWxwaGExOmRlZmF1bHQifQ.BT0YGI7bGdSNaIBqIEnVL0Ky5t-fynaemSGxjGdKOPl0E22UIVGDpAMUhaS19i20c-Dqs-Kn0N-R5QyDNpZg8vOL5KIFqu2kSYNbKxtQW7TPYIsV0d9wUZjLSr54DKrmyXNMGRoT2bwcF4yyfmO46eMmZSaXN8Y4lgapeabg6CBVVQYHD-GrgXf9jVLeJfCQkTuojK1iXOphyD6NqlGtVCaY1jWxbBMibN0q214vKvQboub8YMuvclGdzn_l_ZQSTjvhBj9I-W1t-JArVjqHoIb8_FlR9BSgzgL7V3Jki55vmiOdEYqMErJWrIZPP3s8qkU5hhO9rSVEd3LJHponvQ","expirationTimestamp":"2022-01-26T22:04:07Z"}}` //nolint
	expectedToken := `eyJhbGciOiJSUzI1NiIsImtpZCI6InRhVDBxbzhQVEZ1ajB1S3BYUUxIclRsR01XakxjemJNOTlzWVMxSlNwbWcifQ.eyJhdWQiOlsiYXBpOi8vQXp1cmVBRGlUb2tlbkV4Y2hhbmdlIl0sImV4cCI6MTY0MzIzNDY0NywiaWF0IjoxNjQzMjMxMDQ3LCJpc3MiOiJodHRwczovL2t1YmVybmV0ZXMuZGVmYXVsdC5zdmMuY2x1c3Rlci5sb2NhbCIsImt1YmVybmV0ZXMuaW8iOnsibmFtZXNwYWNlIjoidGVzdC12MWFscGhhMSIsInBvZCI6eyJuYW1lIjoic2VjcmV0cy1zdG9yZS1pbmxpbmUtY3JkIiwidWlkIjoiYjBlYmZjMzUtZjEyNC00ZTEyLWI3N2UtYjM0MjM2N2IyMDNmIn0sInNlcnZpY2VhY2NvdW50Ijp7Im5hbWUiOiJkZWZhdWx0IiwidWlkIjoiMjViNGY1NzgtM2U4MC00NTczLWJlOGQtZTdmNDA5ZDI0MmI2In19LCJuYmYiOjE2NDMyMzEwNDcsInN1YiI6InN5c3RlbTpzZXJ2aWNlYWNjb3VudDp0ZXN0LXYxYWxwaGExOmRlZmF1bHQifQ.ALE46aKmtTV7dsuFOwDZqvEjdHFUTNP-JVjMxexTemmPA78fmPTUZF0P6zANumA03fjX3L-MZNR3PxmEZgKA9qEGIDsljLsUWsVBEquowuBh8yoBYkGkMJmRfmbfS3y7_4Q7AU3D9Drw4iAHcn1GwedjOQC0i589y3dkNNqf8saqHfXkbSSLtSE0f2uzI-PjuTKvR1kuojEVNKlEcA4wsKfoiRpkua17sHkHU0q9zxCMDCr_1f8xbigRnRx0wscU3vy-8KhF3zQtpcWkk3r4C5YSXut9F3xjz5J9DUQn2vNMfZg4tOdcR-9Xv9fbY5iujiSlS58GEktSEa3SE9wrCw`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         //nolint

	token, err := parseServiceAccountToken(saTokens)
	if err != nil {
		t.Fatalf("ParseServiceAccountToken(%s) = %v, want nil", saTokens, err)
	}
	if token != expectedToken {
		t.Errorf("ParseServiceAccountToken(%s) = %s, want %s", saTokens, token, expectedToken)
	}
}

func TestIsSupportedPublicNetworkAccess(t *testing.T) {
	tests := []struct {
		publicNetworkAccess string
		expectedResult      bool
	}{
		{
			publicNetworkAccess: "",
			expectedResult:      true,
		},
		{
			publicNetworkAccess: "Enabled",
			expectedResult:      true,
		},
		{
			publicNetworkAccess: "Disabled",
			expectedResult:      true,
		},
		{
			publicNetworkAccess: "InvalidValue",
			expectedResult:      false,
		},
	}

	for _, test := range tests {
		result := isSupportedPublicNetworkAccess(test.publicNetworkAccess)
		if result != test.expectedResult {
			t.Errorf("isSupportedPublicNetworkAccess(%s) returned %v, expected %v", test.publicNetworkAccess, result, test.expectedResult)
		}
	}
}

func TestIsValidTokenFileName(t *testing.T) {
	testCases := []struct {
		name     string
		fileName string
		expected bool
	}{
		{
			name:     "valid lowercase",
			fileName: "token",
			expected: true,
		},
		{
			name:     "valid uppercase",
			fileName: "TOKEN",
			expected: true,
		},
		{
			name:     "valid mixed alphanumeric with hyphen",
			fileName: "Token-123",
			expected: true,
		},
		{
			name:     "valid mixed alphanumeric with hyphen#2",
			fileName: "0ab48765-efce-4799-8a9c-c3e1de2ee42eg",
			expected: true,
		},
		{
			name:     "empty string",
			fileName: "",
			expected: false,
		},
		{
			name:     "contains underscore",
			fileName: "token_file",
			expected: false,
		},
		{
			name:     "contains dot",
			fileName: "token.file",
			expected: false,
		},
		{
			name:     "contains space",
			fileName: "token file",
			expected: false,
		},
		{
			name:     "contains slash",
			fileName: "token/file",
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isValidTokenFileName(tc.fileName); got != tc.expected {
				t.Fatalf("isValidTokenFileName(%q) = %t, want %t", tc.fileName, got, tc.expected)
			}
		})
	}
}

func TestGetAuthEnvWithClientID(t *testing.T) {
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "clientID specified with valid service account token",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{}
				d.cloud.SubscriptionID = "test-sub"
				d.cloud.TenantID = "test-tenant"

				attrib := map[string]string{
					clientIDField:       "test-client-id",
					containerNameField:  "containername",
					storageAccountField: "accountname",
				}
				secret := make(map[string]string)
				volumeID := "rg#accountname#containername"

				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				mockStorageAccountsClient := mock_accountclient.NewMockInterface(ctrl)
				d.cloud.ComputeClientFactory = mock_azclient.NewMockClientFactory(ctrl)
				d.cloud.ComputeClientFactory.(*mock_azclient.MockClientFactory).EXPECT().GetAccountClient().Return(mockStorageAccountsClient).AnyTimes()

				// GetStorageAccesskeyFromServiceAccountToken will fail but we're testing the clientID path
				_, _, _, _, _, err := d.GetAuthEnv(context.TODO(), volumeID, "", attrib, secret)
				// Error expected because GetStorageAccesskeyFromServiceAccountToken is not mocked
				assert.Error(t, err)
			},
		},
		{
			name: "clientID specified uses cloud subscriptionID when subsID is empty",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{}
				d.cloud.SubscriptionID = "cloud-sub-id"
				d.cloud.TenantID = "test-tenant"

				attrib := map[string]string{
					clientIDField:       "test-client-id",
					containerNameField:  "containername",
					storageAccountField: "accountname",
				}
				secret := make(map[string]string)
				volumeID := "unique-volume-id"

				ctrl := gomock.NewController(t)
				defer ctrl.Finish()

				_, _, _, _, _, err := d.GetAuthEnv(context.TODO(), volumeID, "", attrib, secret)
				// Error expected because GetStorageAccesskeyFromServiceAccountToken is not mocked
				assert.Error(t, err)
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func TestGetAuthEnvWithMountWithWIToken(t *testing.T) {
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "mountWithWIToken true but clientID not specified and cloud has no identity",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{}
				d.cloud.Config.AzureAuthConfig = azclient.AzureAuthConfig{}

				attrib := map[string]string{
					mountWithWITokenField: "true",
					containerNameField:    "containername",
					storageAccountField:   "accountname",
				}
				secret := make(map[string]string)
				volumeID := "rg#accountname#containername"

				_, _, _, _, _, err := d.GetAuthEnv(context.TODO(), volumeID, "", attrib, secret)
				expectedErr := fmt.Errorf("mountWithWorkloadIdentityToken is true but clientID is not specified")
				assert.Equal(t, expectedErr, err)
			},
		},
		{
			name: "mountWithWIToken true with clientID from cloud config but invalid service account token",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{}
				d.cloud.Config.AzureAuthConfig = azclient.AzureAuthConfig{
					UserAssignedIdentityID: "test-identity-id",
				}

				attrib := map[string]string{
					mountWithWITokenField:    "true",
					containerNameField:       "containername",
					storageAccountField:      "accountname",
					serviceAccountTokenField: "invalid-token",
				}
				secret := make(map[string]string)
				volumeID := "rg#accountname#containername"

				_, _, _, _, _, err := d.GetAuthEnv(context.TODO(), volumeID, "", attrib, secret)
				assert.Error(t, err)
			},
		},
		{
			name: "mountWithWIToken true with explicit clientID but empty service account token",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{}

				attrib := map[string]string{
					mountWithWITokenField: "true",
					clientIDField:         "explicit-client-id",
					containerNameField:    "containername",
					storageAccountField:   "accountname",
				}
				secret := make(map[string]string)
				volumeID := "rg#accountname#containername"

				_, _, _, _, _, err := d.GetAuthEnv(context.TODO(), volumeID, "", attrib, secret)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "service account token")
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func TestGetAuthEnvSecretNamespaceHandling(t *testing.T) {
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "secretNamespace defaults to pvcNamespace when secretNamespace is empty",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{}
				d.KubeClient = fake.NewSimpleClientset()

				pvcNS := "pvc-namespace"
				attrib := map[string]string{
					pvcNamespaceKey:     pvcNS,
					containerNameField:  "containername",
					storageAccountField: "accountname",
				}
				secret := make(map[string]string)
				volumeID := "rg#accountname#containername"

				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				mockStorageAccountsClient := mock_accountclient.NewMockInterface(ctrl)
				d.cloud.ComputeClientFactory = mock_azclient.NewMockClientFactory(ctrl)
				d.cloud.ComputeClientFactory.(*mock_azclient.MockClientFactory).EXPECT().GetAccountClient().Return(mockStorageAccountsClient).AnyTimes()
				s := "test-key"
				accountkey := armstorage.AccountKey{Value: &s}
				list := []*armstorage.AccountKey{&accountkey}
				mockStorageAccountsClient.EXPECT().ListKeys(gomock.Any(), gomock.Any(), gomock.Any()).Return(list, nil).AnyTimes()
				d.cloud.ComputeClientFactory.(*mock_azclient.MockClientFactory).EXPECT().GetAccountClientForSub(gomock.Any()).Return(mockStorageAccountsClient, nil).AnyTimes()

				_, _, _, _, _, err := d.GetAuthEnv(context.TODO(), volumeID, "", attrib, secret)
				assert.NoError(t, err)
			},
		},
		{
			name: "secretNamespace defaults to 'default' when both secretNamespace and pvcNamespace are empty",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{}
				d.KubeClient = fake.NewSimpleClientset()

				attrib := map[string]string{
					containerNameField:  "containername",
					storageAccountField: "accountname",
				}
				secret := make(map[string]string)
				volumeID := "rg#accountname#containername"

				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				mockStorageAccountsClient := mock_accountclient.NewMockInterface(ctrl)
				d.cloud.ComputeClientFactory = mock_azclient.NewMockClientFactory(ctrl)
				d.cloud.ComputeClientFactory.(*mock_azclient.MockClientFactory).EXPECT().GetAccountClient().Return(mockStorageAccountsClient).AnyTimes()
				s := "test-key"
				accountkey := armstorage.AccountKey{Value: &s}
				list := []*armstorage.AccountKey{&accountkey}
				mockStorageAccountsClient.EXPECT().ListKeys(gomock.Any(), gomock.Any(), gomock.Any()).Return(list, nil).AnyTimes()
				d.cloud.ComputeClientFactory.(*mock_azclient.MockClientFactory).EXPECT().GetAccountClientForSub(gomock.Any()).Return(mockStorageAccountsClient, nil).AnyTimes()

				_, _, _, _, _, err := d.GetAuthEnv(context.TODO(), volumeID, "", attrib, secret)
				assert.NoError(t, err)
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func TestGetAuthEnvResourceGroupHandling(t *testing.T) {
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "rgName defaults to cloud.ResourceGroup when rgName is empty",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{}
				d.cloud.ResourceGroup = "cloud-rg"

				attrib := map[string]string{
					containerNameField:  "containername",
					storageAccountField: "accountname",
				}
				secret := map[string]string{
					accountNameField: "accountname",
					accountKeyField:  "testkey",
				}
				volumeID := "unique-volume-id"

				rg, _, _, _, _, err := d.GetAuthEnv(context.TODO(), volumeID, "", attrib, secret)
				assert.NoError(t, err)
				assert.Equal(t, "cloud-rg", rg)
			},
		},
		{
			name: "rgName from attrib takes precedence over cloud.ResourceGroup",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{}
				d.cloud.ResourceGroup = "cloud-rg"

				attrib := map[string]string{
					resourceGroupField:  "attrib-rg",
					containerNameField:  "containername",
					storageAccountField: "accountname",
				}
				secret := map[string]string{
					accountNameField: "accountname",
					accountKeyField:  "testkey",
				}
				volumeID := "unique-volume-id"

				rg, _, _, _, _, err := d.GetAuthEnv(context.TODO(), volumeID, "", attrib, secret)
				assert.NoError(t, err)
				assert.Equal(t, "attrib-rg", rg)
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func TestGetAuthEnvTenantIDHandling(t *testing.T) {
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "tenantID defaults to cloud.TenantID when tenantID is empty",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{}
				d.cloud.TenantID = "cloud-tenant-id"

				attrib := map[string]string{
					containerNameField:  "containername",
					storageAccountField: "accountname",
				}
				secret := map[string]string{
					accountNameField: "accountname",
					accountKeyField:  "testkey",
				}
				volumeID := "rg#accountname#containername"

				_, _, _, _, _, err := d.GetAuthEnv(context.TODO(), volumeID, "", attrib, secret)
				assert.NoError(t, err)
			},
		},
		{
			name: "tenantID from attrib takes precedence",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &storage.AccountRepo{}
				d.cloud.TenantID = "cloud-tenant-id"

				attrib := map[string]string{
					tenantIDField:       "attrib-tenant-id",
					containerNameField:  "containername",
					storageAccountField: "accountname",
				}
				secret := map[string]string{
					accountNameField: "accountname",
					accountKeyField:  "testkey",
				}
				volumeID := "rg#accountname#containername"

				_, _, _, _, _, err := d.GetAuthEnv(context.TODO(), volumeID, "", attrib, secret)
				assert.NoError(t, err)
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func TestGetAuthEnvEmptyContainerName(t *testing.T) {
	d := NewFakeDriver()
	d.cloud = &storage.AccountRepo{}

	attrib := map[string]string{
		storageAccountField: "accountname",
	}
	secret := map[string]string{
		accountNameField: "accountname",
		accountKeyField:  "testkey",
	}
	volumeID := "unique-volume-id"

	_, _, _, _, _, err := d.GetAuthEnv(context.TODO(), volumeID, "", attrib, secret) //nolint:dogsled
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not find containerName")
}

func TestGetAuthEnvWithSPNCredentials(t *testing.T) {
	d := NewFakeDriver()
	d.cloud = &storage.AccountRepo{}

	attrib := map[string]string{
		containerNameField:      "containername",
		storageAccountField:     "accountname",
		storageSPNClientIDField: "spn-client-id",
		storageSPNTenantIDField: "spn-tenant-id",
	}
	secret := map[string]string{
		accountNameField:            "accountname",
		accountKeyField:             "testkey",
		storageSPNClientSecretField: "spn-client-secret",
	}
	volumeID := "rg#accountname#containername"

	_, _, _, _, authEnv, err := d.GetAuthEnv(context.TODO(), volumeID, "", attrib, secret) //nolint:dogsled
	assert.NoError(t, err)

	foundClientID := false
	foundTenantID := false
	foundClientSecret := false
	for _, env := range authEnv {
		if env == "AZURE_STORAGE_SPN_CLIENT_ID=spn-client-id" {
			foundClientID = true
		}
		if env == "AZURE_STORAGE_SPN_TENANT_ID=spn-tenant-id" {
			foundTenantID = true
		}
		if env == "AZURE_STORAGE_SPN_CLIENT_SECRET=spn-client-secret" {
			foundClientSecret = true
		}
	}
	assert.True(t, foundClientID, "AZURE_STORAGE_SPN_CLIENT_ID should be in authEnv")
	assert.True(t, foundTenantID, "AZURE_STORAGE_SPN_TENANT_ID should be in authEnv")
	assert.True(t, foundClientSecret, "AZURE_STORAGE_SPN_CLIENT_SECRET should be in authEnv")
}

func TestGetAuthEnvWithSasToken(t *testing.T) {
	d := NewFakeDriver()
	d.cloud = &storage.AccountRepo{}

	attrib := map[string]string{
		containerNameField:  "containername",
		storageAccountField: "accountname",
	}
	secret := map[string]string{
		accountNameField:     "accountname",
		accountSasTokenField: "?sv=2017-03-28&ss=bfqt&srt=sco&sp=rwdlacup",
	}
	volumeID := "rg#accountname#containername"

	_, _, _, _, authEnv, err := d.GetAuthEnv(context.TODO(), volumeID, "", attrib, secret) //nolint:dogsled
	assert.NoError(t, err)

	foundSasToken := false
	for _, env := range authEnv {
		if strings.HasPrefix(env, "AZURE_STORAGE_SAS_TOKEN=") {
			foundSasToken = true
			break
		}
	}
	assert.True(t, foundSasToken, "AZURE_STORAGE_SAS_TOKEN should be in authEnv")
}

func TestGetAuthEnvWithMsiSecret(t *testing.T) {
	d := NewFakeDriver()
	d.cloud = &storage.AccountRepo{}

	attrib := map[string]string{
		containerNameField:  "containername",
		storageAccountField: "accountname",
	}
	secret := map[string]string{
		accountNameField: "accountname",
		msiSecretField:   "msi-secret-value",
	}
	volumeID := "rg#accountname#containername"

	_, _, _, _, authEnv, err := d.GetAuthEnv(context.TODO(), volumeID, "", attrib, secret) //nolint:dogsled
	assert.NoError(t, err)

	foundMsiSecret := false
	for _, env := range authEnv {
		if env == "MSI_SECRET=msi-secret-value" {
			foundMsiSecret = true
			break
		}
	}
	assert.True(t, foundMsiSecret, "MSI_SECRET should be in authEnv")
}

func TestGetAuthEnvMSIAuthTypeSkipsIdentityEnvIfAlreadySet(t *testing.T) {
	d := NewFakeDriver()
	d.cloud = &storage.AccountRepo{}
	d.cloud.Config.AzureAuthConfig = azclient.AzureAuthConfig{
		UserAssignedIdentityID: "cloud-identity-id",
	}

	attrib := map[string]string{
		containerNameField:           "containername",
		storageAccountField:          "accountname",
		storageAuthTypeField:         storageAuthTypeMSI,
		storageIdentityClientIDField: "explicit-identity-id",
	}
	secret := map[string]string{
		accountNameField: "accountname",
		accountKeyField:  "testkey",
	}
	volumeID := "rg#accountname#containername"

	_, _, _, _, authEnv, err := d.GetAuthEnv(context.TODO(), volumeID, "", attrib, secret) //nolint:dogsled
	assert.NoError(t, err)

	count := 0
	for _, env := range authEnv {
		if strings.HasPrefix(env, "AZURE_STORAGE_IDENTITY_CLIENT_ID=") {
			count++
		}
	}
	// Should only have the explicit one, not the cloud one
	assert.Equal(t, 1, count, "Should only have one AZURE_STORAGE_IDENTITY_CLIENT_ID in authEnv")

	found := false
	for _, env := range authEnv {
		if env == "AZURE_STORAGE_IDENTITY_CLIENT_ID=explicit-identity-id" {
			found = true
			break
		}
	}
	assert.True(t, found, "Explicit AZURE_STORAGE_IDENTITY_CLIENT_ID should be preserved")
}
