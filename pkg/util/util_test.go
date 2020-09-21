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
	"os"
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
	err := MakeDir(targetTest)
	assert.NoError(t, err)

	// Remove the directory created
	err = os.RemoveAll(targetTest)
	assert.NoError(t, err)
}
