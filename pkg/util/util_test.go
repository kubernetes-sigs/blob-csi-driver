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

	"github.com/golang/mock/gomock"
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
				f: []byte("ID=ubuntu\nVERSION_ID=\"22.04\""),
			},
			want: &OsInfo{
				Distro:  "ubuntu",
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

func TestTrimDuplicatedSpace(t *testing.T) {
	type args struct {
		s string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			// ignore spell lint error
			name: "trim duplicated space",
			args: args{
				s: "  12 3   456  ",
			},
			want: " 12 3 456 ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TrimDuplicatedSpace(tt.args.s); got != tt.want {
				t.Errorf("TrimDuplicatedSpace() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetAzcopyJob(t *testing.T) {
	tests := []struct {
		desc             string
		listStr          string
		listErr          error
		enableShow       bool
		showStr          string
		showErr          error
		expectedJobState AzcopyJobState
		expectedPercent  string
		expectedErr      error
	}{
		{
			desc:             "run exec get error",
			listStr:          "",
			listErr:          fmt.Errorf("error"),
			enableShow:       false,
			showStr:          "",
			showErr:          nil,
			expectedJobState: AzcopyJobError,
			expectedPercent:  "",
			expectedErr:      fmt.Errorf("couldn't list jobs in azcopy error"),
		},
		{
			desc:             "run exec parse azcopy job list get error",
			listStr:          "JobId: ed1c3833-eaff-fe42-71d7-513fb065a9d9\nStart Time: Monday, 07-Aug-23 03:29:54 UTC",
			listErr:          nil,
			enableShow:       false,
			showStr:          "",
			showErr:          nil,
			expectedJobState: AzcopyJobError,
			expectedPercent:  "",
			expectedErr:      fmt.Errorf("couldn't parse azcopy job list in azcopy error parsing jobs list: ed1c3833-eaff-fe42-71d7-513fb065a9d9\nStart Time: Monday, 07-Aug-23 03:29:54 UTC"),
		},
		{
			desc:             "run exec parse azcopy job not found jobid when Status is Canceled",
			listStr:          "JobId: ed1c3833-eaff-fe42-71d7-513fb065a9d9\nStart Time: Monday, 07-Aug-23 03:29:54 UTC\nStatus: Cancelled\nCommand: copy https://{accountName}.file.core.windows.net/{srcFileshare}{SAStoken} https://{accountName}.file.core.windows.net/{dstFileshare}{SAStoken} --recursive --check-length=false",
			listErr:          nil,
			enableShow:       false,
			showStr:          "",
			showErr:          nil,
			expectedJobState: AzcopyJobNotFound,
			expectedPercent:  "",
			expectedErr:      nil,
		},
		{
			desc:             "run exec parse azcopy job Completed",
			listStr:          "JobId: ed1c3833-eaff-fe42-71d7-513fb065a9d9\nStart Time: Monday, 07-Aug-23 03:29:54 UTC\nStatus: Completed\nCommand: copy https://{accountName}.file.core.windows.net/{srcFileshare}{SAStoken} https://{accountName}.file.core.windows.net/{dstFileshare}{SAStoken} --recursive --check-length=false",
			listErr:          nil,
			enableShow:       false,
			showStr:          "",
			showErr:          nil,
			expectedJobState: AzcopyJobCompleted,
			expectedPercent:  "100.0",
			expectedErr:      nil,
		},
		{
			desc:             "run exec get error in azcopy jobs show",
			listStr:          "JobId: ed1c3833-eaff-fe42-71d7-513fb065a9d9\nStart Time: Monday, 07-Aug-23 03:29:54 UTC\nStatus: InProgress\nCommand: copy https://{accountName}.file.core.windows.net/{srcFileshare}{SAStoken} https://{accountName}.file.core.windows.net/{dstFileshare}{SAStoken} --recursive --check-length=false",
			listErr:          nil,
			enableShow:       true,
			showStr:          "",
			showErr:          fmt.Errorf("error"),
			expectedJobState: AzcopyJobError,
			expectedPercent:  "",
			expectedErr:      fmt.Errorf("couldn't show jobs summary in azcopy error"),
		},
		{
			desc:             "run exec parse azcopy job show error",
			listStr:          "JobId: ed1c3833-eaff-fe42-71d7-513fb065a9d9\nStart Time: Monday, 07-Aug-23 03:29:54 UTC\nStatus: InProgress\nCommand: copy https://{accountName}.file.core.windows.net/{srcFileshare}{SAStoken} https://{accountName}.file.core.windows.net/{dstFileshare}{SAStoken} --recursive --check-length=false",
			listErr:          nil,
			enableShow:       true,
			showStr:          "",
			showErr:          nil,
			expectedJobState: AzcopyJobError,
			expectedPercent:  "",
			expectedErr:      fmt.Errorf("couldn't parse azcopy job show in azcopy error parsing jobs summary:  in Percent Complete (approx)"),
		},
		{
			desc:             "run exec parse azcopy job show succeed",
			listStr:          "JobId: ed1c3833-eaff-fe42-71d7-513fb065a9d9\nStart Time: Monday, 07-Aug-23 03:29:54 UTC\nStatus: InProgress\nCommand: copy https://{accountName}.file.core.windows.net/{srcFileshare}{SAStoken} https://{accountName}.file.core.windows.net/{dstFileshare}{SAStoken} --recursive --check-length=false",
			listErr:          nil,
			enableShow:       true,
			showStr:          "Percent Complete (approx): 50.0",
			showErr:          nil,
			expectedJobState: AzcopyJobRunning,
			expectedPercent:  "50.0",
			expectedErr:      nil,
		},
	}
	for _, test := range tests {
		dstBlobContainer := "dstBlobContainer"

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		m := NewMockEXEC(ctrl)
		m.EXPECT().RunCommand(gomock.Eq("azcopy jobs list | grep dstBlobContainer -B 3")).Return(test.listStr, test.listErr)
		if test.enableShow {
			m.EXPECT().RunCommand(gomock.Not("azcopy jobs list | grep dstBlobContainer -B 3")).Return(test.showStr, test.showErr)
		}

		azcopyFunc := &Azcopy{}
		azcopyFunc.ExecCmd = m
		jobState, percent, err := azcopyFunc.GetAzcopyJob(dstBlobContainer)
		if jobState != test.expectedJobState || percent != test.expectedPercent || !reflect.DeepEqual(err, test.expectedErr) {
			t.Errorf("test[%s]: unexpected jobState: %v, percent: %v, err: %v, expected jobState: %v, percent: %v, err: %v", test.desc, jobState, percent, err, test.expectedJobState, test.expectedPercent, test.expectedErr)
		}
	}
}

func TestParseAzcopyJobList(t *testing.T) {
	tests := []struct {
		desc             string
		str              string
		expectedJobid    string
		expectedJobState AzcopyJobState
		expectedErr      error
	}{
		{
			desc:             "azcopy job not found",
			str:              "",
			expectedJobid:    "",
			expectedJobState: AzcopyJobNotFound,
			expectedErr:      nil,
		},
		{
			desc:             "parse azcopy job list error",
			str:              "JobId: ed1c3833-eaff-fe42-71d7-513fb065a9d9\nStart Time: Monday, 07-Aug-23 03:29:54 UTC",
			expectedJobid:    "",
			expectedJobState: AzcopyJobError,
			expectedErr:      fmt.Errorf("error parsing jobs list: ed1c3833-eaff-fe42-71d7-513fb065a9d9\nStart Time: Monday, 07-Aug-23 03:29:54 UTC"),
		},
		{
			desc:             "parse azcopy job list status error",
			str:              "JobId: ed1c3833-eaff-fe42-71d7-513fb065a9d9\nStart Time: Monday, 07-Aug-23 03:29:54 UTC\nStatus Cancelled\nCommand: copy https://{accountName}.file.core.windows.net/{srcFileshare}{SAStoken} https://{accountName}.file.core.windows.net/{dstFileshare}{SAStoken} --recursive --check-length=false",
			expectedJobid:    "",
			expectedJobState: AzcopyJobError,
			expectedErr:      fmt.Errorf("error parsing jobs list status: Status Cancelled"),
		},
		{
			desc:             "parse azcopy job not found jobid when Status is Canceled",
			str:              "JobId: ed1c3833-eaff-fe42-71d7-513fb065a9d9\nStart Time: Monday, 07-Aug-23 03:29:54 UTC\nStatus: Cancelled\nCommand: copy https://{accountName}.file.core.windows.net/{srcFileshare}{SAStoken} https://{accountName}.file.core.windows.net/{dstFileshare}{SAStoken} --recursive --check-length=false",
			expectedJobid:    "",
			expectedJobState: AzcopyJobNotFound,
			expectedErr:      nil,
		},
		{
			desc:             "parse azcopy job Completed",
			str:              "JobId: ed1c3833-eaff-fe42-71d7-513fb065a9d9\nStart Time: Monday, 07-Aug-23 03:29:54 UTC\nStatus: Completed\nCommand: copy https://{accountName}.file.core.windows.net/{srcFileshare}{SAStoken} https://{accountName}.file.core.windows.net/{dstFileshare}{SAStoken} --recursive --check-length=false",
			expectedJobid:    "",
			expectedJobState: AzcopyJobCompleted,
			expectedErr:      nil,
		},
		{
			desc:             "parse azcopy job InProgress",
			str:              "JobId: ed1c3833-eaff-fe42-71d7-513fb065a9d9\nStart Time: Monday, 07-Aug-23 03:29:54 UTC\nStatus: InProgress\nCommand: copy https://{accountName}.file.core.windows.net/{srcFileshare}{SAStoken} https://{accountName}.file.core.windows.net/{dstFileshare}{SAStoken} --recursive --check-length=false",
			expectedJobid:    "ed1c3833-eaff-fe42-71d7-513fb065a9d9",
			expectedJobState: AzcopyJobRunning,
			expectedErr:      nil,
		},
	}

	for _, test := range tests {
		dstBlobContainer := "dstBlobContainer"
		jobid, jobState, err := parseAzcopyJobList(test.str, dstBlobContainer)
		if jobid != test.expectedJobid || jobState != test.expectedJobState || !reflect.DeepEqual(err, test.expectedErr) {
			t.Errorf("test[%s]: unexpected jobid: %v, jobState: %v, err: %v, expected jobid: %v, jobState: %v, err: %v", test.desc, jobid, jobState, err, test.expectedJobid, test.expectedJobState, test.expectedErr)
		}
	}
}

func TestParseAzcopyJobShow(t *testing.T) {
	tests := []struct {
		desc             string
		str              string
		expectedJobState AzcopyJobState
		expectedPercent  string
		expectedErr      error
	}{
		{
			desc:             "error parse azcopy job show",
			str:              "",
			expectedJobState: AzcopyJobError,
			expectedPercent:  "",
			expectedErr:      fmt.Errorf("error parsing jobs summary:  in Percent Complete (approx)"),
		},
		{
			desc:             "parse azcopy job show succeed",
			str:              "Percent Complete (approx): 50.0",
			expectedJobState: AzcopyJobRunning,
			expectedPercent:  "50.0",
			expectedErr:      nil,
		},
	}

	for _, test := range tests {
		jobState, percent, err := parseAzcopyJobShow(test.str)
		if jobState != test.expectedJobState || percent != test.expectedPercent || !reflect.DeepEqual(err, test.expectedErr) {
			t.Errorf("test[%s]: unexpected jobState: %v, percent: %v, err: %v, expected jobState: %v, percent: %v, err: %v", test.desc, jobState, percent, err, test.expectedJobState, test.expectedPercent, test.expectedErr)
		}
	}
}
