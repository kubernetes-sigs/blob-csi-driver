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
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"testing"

	"github.com/Azure/azure-sdk-for-go/services/storage/mgmt/2019-06-01/storage"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"sigs.k8s.io/cloud-provider-azure/pkg/azureclients/storageaccountclient/mockstorageaccountclient"
	azure "sigs.k8s.io/cloud-provider-azure/pkg/provider"
	"sigs.k8s.io/cloud-provider-azure/pkg/retry"
)

const (
	fakeNodeID     = "fakeNodeID"
	fakeDriverName = "fake"
	vendorVersion  = "0.3.0"
)

func NewFakeDriver() *Driver {
	driver := NewDriver(fakeNodeID)
	driver.Name = fakeDriverName
	driver.Version = vendorVersion
	return driver
}

func TestNewFakeDriver(t *testing.T) {
	d := NewDriver(fakeNodeID)
	assert.NotNil(t, d)
}

func TestNewDriver(t *testing.T) {
	driver := NewDriver(fakeNodeID)
	fakedriver := NewFakeDriver()
	fakedriver.Name = DriverName
	fakedriver.Version = driverVersion
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
				if err := ioutil.WriteFile(fakeCredFile, []byte(fakeCredContent), 0666); err != nil {
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
				d.Run("tcp://127.0.0.1:0", "", true)
			},
		},
		{
			name: "Successful run with node ID missing",
			testFunc: func(t *testing.T) {
				if err := ioutil.WriteFile(fakeCredFile, []byte(fakeCredContent), 0666); err != nil {
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
				d.cloud = &azure.Cloud{}
				d.NodeID = ""
				d.Run("tcp://127.0.0.1:0", "", true)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func TestAppendDefaultMountOptions(t *testing.T) {
	tests := []struct {
		options  []string
		expected []string
	}{
		{
			options: []string{"dir_mode=0777"},
			expected: []string{"dir_mode=0777",
				fmt.Sprintf("%s=%s", fileMode, defaultFileMode),
				fmt.Sprintf("%s=%s", vers, defaultVers)},
		},
		{
			options: []string{"file_mode=0777"},
			expected: []string{"file_mode=0777",
				fmt.Sprintf("%s=%s", dirMode, defaultDirMode),
				fmt.Sprintf("%s=%s", vers, defaultVers)},
		},
		{
			options: []string{"vers=2.1"},
			expected: []string{"vers=2.1",
				fmt.Sprintf("%s=%s", fileMode, defaultFileMode),
				fmt.Sprintf("%s=%s", dirMode, defaultDirMode)},
		},
		{
			options: []string{""},
			expected: []string{"", fmt.Sprintf("%s=%s",
				fileMode, defaultFileMode),
				fmt.Sprintf("%s=%s", dirMode, defaultDirMode),
				fmt.Sprintf("%s=%s", vers, defaultVers)},
		},
		{
			options:  []string{"file_mode=0777", "dir_mode=0777"},
			expected: []string{"file_mode=0777", "dir_mode=0777", fmt.Sprintf("%s=%s", vers, defaultVers)},
		},
	}

	for _, test := range tests {
		result := appendDefaultMountOptions(test.options)
		if !reflect.DeepEqual(result, test.expected) {
			t.Errorf("input: %q, appendDefaultMountOptions result: %q, expected: %q", test.options, result, test.expected)
		}
	}
}

func TestGetContainerInfo(t *testing.T) {
	tests := []struct {
		options   string
		expected1 string
		expected2 string
		expected3 string
		expected4 error
	}{
		{
			options:   "rg#f5713de20cde511e8ba4900#pvc-file-dynamic-17e43f84-f474-11e8-acd0-000d3a00df41",
			expected1: "rg",
			expected2: "f5713de20cde511e8ba4900",
			expected3: "pvc-file-dynamic-17e43f84-f474-11e8-acd0-000d3a00df41",
			expected4: nil,
		},
		{
			options:   "rg#f5713de20cde511e8ba4900",
			expected1: "",
			expected2: "",
			expected3: "",
			expected4: fmt.Errorf("error parsing volume id: \"rg#f5713de20cde511e8ba4900\", should at least contain two #"),
		},
		{
			options:   "rg",
			expected1: "",
			expected2: "",
			expected3: "",
			expected4: fmt.Errorf("error parsing volume id: \"rg\", should at least contain two #"),
		},
		{
			options:   "",
			expected1: "",
			expected2: "",
			expected3: "",
			expected4: fmt.Errorf("error parsing volume id: \"\", should at least contain two #"),
		},
	}

	for _, test := range tests {
		result1, result2, result3, result4 := GetContainerInfo(test.options)
		if !reflect.DeepEqual(result1, test.expected1) || !reflect.DeepEqual(result2, test.expected2) ||
			!reflect.DeepEqual(result3, test.expected3) || !reflect.DeepEqual(result4, test.expected4) {
			t.Errorf("input: %q, GetContainerInfo result1: %q, expected1: %q, result2: %q, expected2: %q, result3: %q, expected3: %q, result4: %q, expected4: %q", test.options, result1, test.expected1, result2, test.expected2,
				result3, test.expected3, result4, test.expected4)
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
			desc:         "shareNotFound",
			rpcErr:       errors.New("storage.FileSharesClient#Get: Failure responding to request: StatusCode=404 -- Original Error: autorest/azure: Service returned an error. Status=404 Code=\"ShareNotFound\" Message=\"The specified share does not exist\""),
			expectedBool: true,
		},
		{
			desc:         "shareBeingDeleted",
			rpcErr:       errors.New("storage.FileSharesClient#Create: Failure sending request: StatusCode=409 -- Original Error: autorest/azure: Service returned an error. Status=<nil> Code=\"ShareBeingDeleted\" Message=\"The specified share is being deleted. Try operation later.\""),
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
	existingMountPath, err := ioutil.TempDir(os.TempDir(), "blob-csi-mount-test")
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
				attrib["storageaccountname"] = "unit-test"
				attrib["azurestorageidentityclientid"] = "unit-test"
				attrib["azurestorageidentityobjectid"] = "unit-test"
				attrib["azurestorageidentityresourceid"] = "unit-test"
				attrib["msiendpoint"] = "unit-test"
				attrib["azurestoragespnclientid"] = "unit-test"
				attrib["azurestoragespntenantid"] = "unit-test"
				attrib["azurestorageaadendpoint"] = "unit-test"
				volumeID := "rg#f5713de20cde511e8ba4900#pvc-fuse-dynamic-17e43f84-f474-11e8-acd0-000d3a00df41"
				d.cloud = &azure.Cloud{}
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				mockStorageAccountsClient := mockstorageaccountclient.NewMockInterface(ctrl)
				d.cloud.StorageAccountClient = mockStorageAccountsClient
				accountListKeysResult := storage.AccountListKeysResult{}
				rerr := &retry.Error{
					RawError: fmt.Errorf("test"),
				}
				mockStorageAccountsClient.EXPECT().ListKeys(gomock.Any(), gomock.Any(), gomock.Any()).Return(accountListKeysResult, rerr).AnyTimes()
				_, _, _, err := d.GetAuthEnv(context.TODO(), volumeID, "", attrib, secret)
				expectedErr := fmt.Errorf("no key for storage account(f5713de20cde511e8ba4900) under resource group(rg), err Retriable: false, RetryAfter: 0s, HTTPStatusCode: 0, RawError: test")
				if !reflect.DeepEqual(err, expectedErr) {
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
				_, _, _, err := d.GetAuthEnv(context.TODO(), volumeID, "", attrib, secret)
				expectedErr := error(nil)
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
				volumeID := "rg#f5713de20cde511e8ba4900#pvc-fuse-dynamic-17e43f84-f474-11e8-acd0-000d3a00df41"
				secret["accountname"] = "unit-test"
				secret["azurestorageaccountname"] = "unit-test"
				secret["accountkey"] = "unit-test"
				secret["azurestorageaccountkey"] = "unit-test"
				secret["azurestorageaccountsastoken"] = "unit-test"
				secret["msisecret"] = "unit-test"
				secret["azurestoragespnclientsecret"] = "unit-test"
				_, _, _, err := d.GetAuthEnv(context.TODO(), volumeID, "", attrib, secret)
				expectedErr := fmt.Errorf("could not find containerName from attributes(map[]) or volumeID(rg#f5713de20cde511e8ba4900#pvc-fuse-dynamic-17e43f84-f474-11e8-acd0-000d3a00df41)")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
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
				accountName, containerName, authEnv, err := d.GetAuthEnv(context.TODO(), volumeID, nfs, attrib, secret)
				if err != nil {
					t.Errorf("actualErr: (%v), expect no error", err)
				}

				assert.Equal(t, accountName, "accountname")
				assert.Equal(t, containerName, "containername")
				assert.Equal(t, len(authEnv), 0)
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
				d.cloud = &azure.Cloud{}
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				mockStorageAccountsClient := mockstorageaccountclient.NewMockInterface(ctrl)
				d.cloud.StorageAccountClient = mockStorageAccountsClient
				accountListKeysResult := storage.AccountListKeysResult{}
				rerr := &retry.Error{
					RawError: fmt.Errorf("test"),
				}
				mockStorageAccountsClient.EXPECT().ListKeys(gomock.Any(), gomock.Any(), gomock.Any()).Return(accountListKeysResult, rerr).AnyTimes()
				_, _, _, _, err := d.GetStorageAccountAndContainer(context.TODO(), volumeID, attrib, secret)
				expectedErr := fmt.Errorf("no key for storage account(f5713de20cde511e8ba4900) under resource group(rg), err Retriable: false, RetryAfter: 0s, HTTPStatusCode: 0, RawError: test")
				if !reflect.DeepEqual(err, expectedErr) {
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
				_, _, _, _, err := d.GetStorageAccountAndContainer(context.TODO(), volumeID, attrib, secret)
				expectedErr := error(nil)
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
			expectedError:       fmt.Errorf("could not find accountname or azurestorageaccountname field secrets(map[accountname: accountkey:])"),
		},
		{
			options:             emptyAccountKeyMap,
			expectedAccountName: "testaccount",
			expectedAccountKey:  "",
			expectedError:       fmt.Errorf("could not find accountkey or azurestorageaccountkey field in secrets(%v)", emptyAccountKeyMap),
		},
		{
			options:             emptyAccountNameMap,
			expectedAccountName: "",
			expectedAccountKey:  "testkey",
			expectedError:       fmt.Errorf("could not find accountname or azurestorageaccountname field secrets(%v)", emptyAccountNameMap),
		},
		{
			options:             emptyAzureAccountKeyMap,
			expectedAccountName: "testaccount",
			expectedAccountKey:  "",
			expectedError:       fmt.Errorf("could not find accountkey or azurestorageaccountkey field in secrets(%v)", emptyAzureAccountKeyMap),
		},
		{
			options:             emptyAzureAccountNameMap,
			expectedAccountName: "",
			expectedAccountKey:  "testkey",
			expectedError:       fmt.Errorf("could not find accountname or azurestorageaccountname field secrets(%v)", emptyAzureAccountNameMap),
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
		result, err := setAzureCredentials(test.kubeClient, test.accountName, test.accountKey, test.secretNamespace)
		if result != test.expectedName || !reflect.DeepEqual(err, test.expectedErr) {
			t.Errorf("desc: %s,\n input: kubeClient(%v), accountName(%v), accountKey(%v),\n setAzureCredentials result: %v, expectedName: %v err: %v, expectedErr: %v",
				test.desc, test.kubeClient, test.accountName, test.accountKey, result, test.expectedName, err, test.expectedErr)
		}
	}
}
