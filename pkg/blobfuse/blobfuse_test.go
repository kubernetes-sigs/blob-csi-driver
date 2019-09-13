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

package blobfuse

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
		result1, result2, result3, result4 := getContainerInfo(test.options)
		if !reflect.DeepEqual(result1, test.expected1) || !reflect.DeepEqual(result2, test.expected2) ||
			!reflect.DeepEqual(result3, test.expected3) || !reflect.DeepEqual(result4, test.expected4) {
			t.Errorf("input: %q, getContainerInfo result1: %q, expected1: %q, result2: %q, expected2: %q, result3: %q, expected3: %q, result4: %q, expected4: %q", test.options, result1, test.expected1, result2, test.expected2,
				result3, test.expected3, result4, test.expected4)
		}
	}
}

func TestGetStorageAccountFromSecretsMap(t *testing.T) {
	emptyAccountKeyMap := map[string]string{
		"accountname": "testaccount",
		"accountkey":  "",
	}

	emptyAccountNameMap := map[string]string{
		"accountname": "",
		"accountkey":  "testkey",
	}

	emptyAzureAccountKeyMap := map[string]string{
		"azurestorageaccountname": "testaccount",
		"azurestorageaccountkey":  "",
	}

	emptyAzureAccountNameMap := map[string]string{
		"azurestorageaccountname": "",
		"azurestorageaccountkey":  "testkey",
	}

	emptyAccountSasTokenMap := map[string]string{
		"azurestorageaccountname":     "testaccount",
		"azurestorageaccountsastoken": "",
	}

	accesskeyAndaccountSasTokenMap := map[string]string{
		"azurestorageaccountname":     "testaccount",
		"azurestorageaccountkey":      "testkey",
		"azurestorageaccountsastoken": "testkey",
	}

	tests := []struct {
		options   map[string]string
		expected1 string
		expected2 string
		expected3 string
		expected4 error
	}{
		{
			options: map[string]string{
				"accountname": "testaccount",
				"accountkey":  "testkey",
			},
			expected1: "testaccount",
			expected2: "testkey",
			expected3: "",
			expected4: nil,
		},
		{
			options: map[string]string{
				"azurestorageaccountname": "testaccount",
				"azurestorageaccountkey":  "testkey",
			},
			expected1: "testaccount",
			expected2: "testkey",
			expected3: "",
			expected4: nil,
		},
		{
			options: map[string]string{
				"accountname": "",
				"accountkey":  "",
			},
			expected1: "",
			expected2: "",
			expected3: "",
			expected4: fmt.Errorf("could not find accountname or azurestorageaccountname field secrets(map[accountname: accountkey:])"),
		},
		{
			options:   emptyAccountKeyMap,
			expected1: "",
			expected2: "",
			expected3: "",
			expected4: fmt.Errorf("could not find accountkey, azurestorageaccountkey or azurestorageaccountsastoken field in secrets(%v)", emptyAccountKeyMap),
		},
		{
			options:   emptyAccountNameMap,
			expected1: "",
			expected2: "",
			expected3: "",
			expected4: fmt.Errorf("could not find accountname or azurestorageaccountname field secrets(%v)", emptyAccountNameMap),
		},
		{
			options:   emptyAzureAccountKeyMap,
			expected1: "",
			expected2: "",
			expected3: "",
			expected4: fmt.Errorf("could not find accountkey, azurestorageaccountkey or azurestorageaccountsastoken field in secrets(%v)", emptyAzureAccountKeyMap),
		},
		{
			options:   emptyAzureAccountNameMap,
			expected1: "",
			expected2: "",
			expected3: "",
			expected4: fmt.Errorf("could not find accountname or azurestorageaccountname field secrets(%v)", emptyAzureAccountNameMap),
		},
		{
			options:   emptyAccountSasTokenMap,
			expected1: "",
			expected2: "",
			expected3: "",
			expected4: fmt.Errorf("could not find accountkey, azurestorageaccountkey or azurestorageaccountsastoken field in secrets(%v)", emptyAzureAccountKeyMap),
		},
		{
			options:   accesskeyAndaccountSasTokenMap,
			expected1: "",
			expected2: "",
			expected3: "",
			expected4: fmt.Errorf("could not specify Access Key and SAS Token together"),
		},
		{
			options:   nil,
			expected1: "",
			expected2: "",
			expected3: "",
			expected4: fmt.Errorf("unexpected: getStorageAccountFromSecretsMap secrets is nil"),
		},
	}

	for _, test := range tests {
		result1, result2, result3, result4 := getStorageAccountFromSecretsMap(test.options)
		if !reflect.DeepEqual(result1, test.expected1) || !reflect.DeepEqual(result2, test.expected2) || !reflect.DeepEqual(result3, test.expected3) {
			t.Errorf("input: %q, getStorageAccountFromSecretsMap result1: %q, expected1: %q, result2: %q, expected2: %q, result3: %q, expected3: %q, result4: %q, expected4: %q", test.options, result1, test.expected1, result2, test.expected2,
				result3, test.expected3, result4, test.expected4)
		} else {
			if result1 == "" || (result2 == "" && result3 == "") {
				assert.Error(t, result4)
			}
		}
	}
}

func TestGetValidContainerName(t *testing.T) {
	tests := []struct {
		volumeName string
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
		result := getValidContainerName(test.volumeName)
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
