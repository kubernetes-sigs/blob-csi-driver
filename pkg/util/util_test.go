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

package util

import (
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRoundUpBytes(t *testing.T) {
	var sizeInBytes int64 = 1024
	actual := RoundUpBytes(sizeInBytes)
	if actual != 1*GiB {
		t.Fatalf("Wrong result for RoundUpBytes. Got: %d", actual)
	}
}

func TestRoundUpGiB(t *testing.T) {
	var sizeInBytes int64 = 1
	actual := RoundUpGiB(sizeInBytes)
	if actual != 1 {
		t.Fatalf("Wrong result for RoundUpGiB. Got: %d", actual)
	}
}

func TestSimpleLockEntry(t *testing.T) {
	testLockMap := NewLockMap()

	callbackChan1 := make(chan interface{})
	go testLockMap.lockAndCallback(t, "entry1", callbackChan1)
	ensureCallbackHappens(t, callbackChan1)
}

func TestSimpleLockUnlockEntry(t *testing.T) {
	testLockMap := NewLockMap()

	callbackChan1 := make(chan interface{})
	go testLockMap.lockAndCallback(t, "entry1", callbackChan1)
	ensureCallbackHappens(t, callbackChan1)
	testLockMap.UnlockEntry("entry1")
}

func TestConcurrentLockEntry(t *testing.T) {
	testLockMap := NewLockMap()

	callbackChan1 := make(chan interface{})
	callbackChan2 := make(chan interface{})

	go testLockMap.lockAndCallback(t, "entry1", callbackChan1)
	ensureCallbackHappens(t, callbackChan1)

	go testLockMap.lockAndCallback(t, "entry1", callbackChan2)
	ensureNoCallback(t, callbackChan2)

	testLockMap.UnlockEntry("entry1")
	ensureCallbackHappens(t, callbackChan2)
	testLockMap.UnlockEntry("entry1")
}

func (lm *LockMap) lockAndCallback(t *testing.T, entry string, callbackChan chan<- interface{}) {
	lm.LockEntry(entry)
	callbackChan <- true
}

var callbackTimeout = 2 * time.Second

func ensureCallbackHappens(t *testing.T, callbackChan <-chan interface{}) bool {
	select {
	case <-callbackChan:
		return true
	case <-time.After(callbackTimeout):
		t.Fatalf("timed out waiting for callback")
		return false
	}
}

func ensureNoCallback(t *testing.T, callbackChan <-chan interface{}) bool {
	select {
	case <-callbackChan:
		t.Fatalf("unexpected callback")
		return false
	case <-time.After(callbackTimeout):
		return true
	}
}

func TestUnlockEntryNotExists(t *testing.T) {
	testLockMap := NewLockMap()

	callbackChan1 := make(chan interface{})
	go testLockMap.lockAndCallback(t, "entry1", callbackChan1)
	ensureCallbackHappens(t, callbackChan1)
	// entry2 does not exist
	testLockMap.UnlockEntry("entry2")
	testLockMap.UnlockEntry("entry1")
}
func TestBytesToGiB(t *testing.T) {
	var sizeInBytes int64 = 5 * GiB

	actual := BytesToGiB(sizeInBytes)
	if actual != 5 {
		t.Fatalf("Wrong result for BytesToGiB. Got: %d", actual)
	}
}

func TestGiBToBytes(t *testing.T) {
	var sizeInGiB int64 = 3

	actual := GiBToBytes(sizeInGiB)
	if actual != 3*GiB {
		t.Fatalf("Wrong result for GiBToBytes. Got: %d", actual)
	}
}

func TestGetMountOptions(t *testing.T) {
	tests := []struct {
		options  []string
		expected string
	}{
		{
			options:  []string{"-o allow_other", "-o ro", "--use-https=true"},
			expected: "-o allow_other -o ro --use-https=true",
		},
		{
			options:  []string{"-o allow_other"},
			expected: "-o allow_other",
		},
		{
			options:  []string{""},
			expected: "",
		},
		{
			options:  make([]string, 0),
			expected: "",
		},
	}

	for _, test := range tests {
		result := GetMountOptions(test.options)
		if result != test.expected {
			t.Errorf("getMountOptions(%v) result: %s, expected: %s", test.options, result, test.expected)
		}
	}
}

func TestMakeDir(t *testing.T) {
	//Successfully create directory
	targetTest := "./target_test"
	err := MakeDir(targetTest, 0777)
	assert.NoError(t, err)

	// Remove the directory created
	err = os.RemoveAll(targetTest)
	assert.NoError(t, err)
}

func TestConvertTagsToMap(t *testing.T) {
	tests := []struct {
		desc        string
		tags        string
		expectedOut map[string]string
		expectedErr error
	}{
		{
			desc:        "Improper KeyValuePair",
			tags:        "foo=bar=gar,lorem=ipsum",
			expectedOut: nil,
			expectedErr: fmt.Errorf("Tags '%s' are invalid, the format should like: 'key1=value1,key2=value2'", "foo=bar=gar,lorem=ipsum"),
		},
		{
			desc:        "Missing Key",
			tags:        "=bar,lorem=ipsum",
			expectedOut: nil,
			expectedErr: fmt.Errorf("Tags '%s' are invalid, the format should like: 'key1=value1,key2=value2'", "=bar,lorem=ipsum"),
		},
		{
			desc:        "Successful Input/Output",
			tags:        "foo=bar,lorem=ipsum",
			expectedOut: map[string]string{"foo": "bar", "lorem": "ipsum"},
			expectedErr: nil,
		},
	}

	for _, test := range tests {
		output, err := ConvertTagsToMap(test.tags)
		assert.Equal(t, test.expectedOut, output, test.desc)
		assert.Equal(t, test.expectedErr, err, test.desc)
	}
}
func TestConvertTagsToMap2(t *testing.T) {
	type StringMap map[string]string
	tests := []struct {
		tags     string
		expected map[string]string
		err      bool
	}{
		{
			tags:     "",
			expected: StringMap{},
			err:      false,
		},
		{
			tags:     "key1=value1, key2=value2,key3= value3",
			expected: StringMap{"key1": "value1", "key2": "value2", "key3": "value3"},
			err:      false,
		},
		{
			tags:     " key = value ",
			expected: StringMap{"key": "value"},
			err:      false,
		},
		{
			tags:     "keyvalue",
			expected: nil,
			err:      true,
		},
		{
			tags:     " = value,=",
			expected: nil,
			err:      true,
		},
	}
	for _, test := range tests {
		result, err := ConvertTagsToMap(test.tags)
		if test.err {
			assert.NotNil(t, err)
		} else {
			assert.Nil(t, err)
		}
		assert.Equal(t, result, test.expected)
	}
}

func TestGetOSInfo(t *testing.T) {
	type args struct {
		f interface{}
	}
	tests := []struct {
		name    string
		args    args
		want    *OsInfo
		wantErr bool
	}{
		{
			name: "file not exist",
			args: args{
				f: "/not-exist/not-exist",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "parse os info correctly",
			args: args{
				f: []byte("DISTRIB_ID=Ubuntu\nDISTRIB_RELEASE=22.04"),
			},
			want: &OsInfo{
				Distro:  "Ubuntu",
				Version: "22.04",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetOSInfo(tt.args.f)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetOSInfo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetOSInfo() = %v, want %v", got, tt.want)
			}
		})
	}
}
