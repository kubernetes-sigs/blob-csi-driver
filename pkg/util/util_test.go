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
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	"k8s.io/kubernetes/pkg/volume"
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

func (lm *LockMap) lockAndCallback(_ *testing.T, entry string, callbackChan chan<- interface{}) {
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
	// Successfully create directory
	targetTest := filepath.Join(os.TempDir(), "TestMakeDir")
	err := MakeDir(targetTest, 0777)
	defer func() {
		err := os.RemoveAll(targetTest)
		assert.NoError(t, err)
	}()
	assert.NoError(t, err)

	// create an existing directory
	if err = MakeDir(targetTest, 0755); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestConvertTagsToMap(t *testing.T) {
	tests := []struct {
		desc          string
		tags          string
		tagsDelimiter string
		expectedOut   map[string]string
		expectedErr   error
	}{
		{
			desc:          "Improper KeyValuePair",
			tags:          "foo,lorem=ipsum",
			tagsDelimiter: ",",
			expectedOut:   nil,
			expectedErr:   fmt.Errorf("Tags '%s' are invalid, the format should like: 'key1=value1,key2=value2'", "foo,lorem=ipsum"),
		},
		{
			desc:          "Missing Key",
			tags:          "=bar,lorem=ipsum",
			tagsDelimiter: ",",
			expectedOut:   nil,
			expectedErr:   fmt.Errorf("Tags '%s' are invalid, the format should like: 'key1=value1,key2=value2'", "=bar,lorem=ipsum"),
		},
		{
			desc:          "Successful Input/Output",
			tags:          "foo=bar,lorem=ipsum",
			tagsDelimiter: ",",
			expectedOut:   map[string]string{"foo": "bar", "lorem": "ipsum"},
			expectedErr:   nil,
		},
		{
			desc:          "should return success for empty tagsDelimiter",
			tags:          "key1=value1,key2=value2",
			tagsDelimiter: "",
			expectedOut: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			expectedErr: nil,
		},
		{
			desc:          "should return success for special tagsDelimiter and tag values containing commas and equal sign",
			tags:          "key1=aGVsbG8=;key2=value-2, value-3",
			tagsDelimiter: ";",
			expectedOut: map[string]string{
				"key1": "aGVsbG8=",
				"key2": "value-2, value-3",
			},
			expectedErr: nil,
		},
	}

	for _, test := range tests {
		output, err := ConvertTagsToMap(test.tags, test.tagsDelimiter)
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
		result, err := ConvertTagsToMap(test.tags, "")
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

func TestCleanJobs(t *testing.T) {
	tests := []struct {
		desc        string
		execStr     string
		execErr     error
		expectedErr error
	}{
		{
			desc:        "run exec get error",
			execStr:     "",
			execErr:     fmt.Errorf("error"),
			expectedErr: fmt.Errorf("error"),
		},
		{
			desc:        "run exec succeed",
			execStr:     "cleaned",
			execErr:     nil,
			expectedErr: nil,
		},
	}
	for _, test := range tests {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		m := NewMockEXEC(ctrl)
		m.EXPECT().RunCommand(gomock.Eq("azcopy jobs clean"), nil).Return(test.execStr, test.execErr)

		azcopyFunc := &Azcopy{ExecCmd: m}
		_, err := azcopyFunc.CleanJobs()
		if !reflect.DeepEqual(err, test.expectedErr) {
			t.Errorf("test[%s]: unexpected err: %v, expected err: %v", test.desc, err, test.expectedErr)
		}
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
			desc:             "run exec parse azcopy job CompletedWithErrors",
			listStr:          "JobId: ed1c3833-eaff-fe42-71d7-513fb065a9d9\nStart Time: Monday, 07-Aug-23 03:29:54 UTC\nStatus: CompletedWithErrors\nCommand: copy https://{accountName}.file.core.windows.net/{srcFileshare}{SAStoken} https://{accountName}.file.core.windows.net/{dstFileshare}{SAStoken} --recursive --check-length=false",
			listErr:          nil,
			enableShow:       false,
			showStr:          "",
			showErr:          nil,
			expectedJobState: AzcopyJobCompletedWithErrors,
			expectedPercent:  "100.0",
			expectedErr:      nil,
		},
		{
			desc:             "run exec parse azcopy job CompletedWithSkipped",
			listStr:          "JobId: ed1c3833-eaff-fe42-71d7-513fb065a9d9\nStart Time: Monday, 07-Aug-23 03:29:54 UTC\nStatus: CompletedWithSkipped\nCommand: copy https://{accountName}.file.core.windows.net/{srcFileshare}{SAStoken} https://{accountName}.file.core.windows.net/{dstFileshare}{SAStoken} --recursive --check-length=false",
			listErr:          nil,
			enableShow:       false,
			showStr:          "",
			showErr:          nil,
			expectedJobState: AzcopyJobCompletedWithSkipped,
			expectedPercent:  "100.0",
			expectedErr:      nil,
		},
		{
			desc:             "run exec parse azcopy job CompletedWithErrorsAndSkipped",
			listStr:          "JobId: ed1c3833-eaff-fe42-71d7-513fb065a9d9\nStart Time: Monday, 07-Aug-23 03:29:54 UTC\nStatus: CompletedWithErrorsAndSkipped\nCommand: copy https://{accountName}.file.core.windows.net/{srcFileshare}{SAStoken} https://{accountName}.file.core.windows.net/{dstFileshare}{SAStoken} --recursive --check-length=false",
			listErr:          nil,
			enableShow:       false,
			showStr:          "",
			showErr:          nil,
			expectedJobState: AzcopyJobCompletedWithErrorsAndSkipped,
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
		m.EXPECT().RunCommand(gomock.Eq("azcopy jobs list | grep dstBlobContainer -B 3"), []string{}).Return(test.listStr, test.listErr)
		if test.enableShow {
			m.EXPECT().RunCommand(gomock.Not("azcopy jobs list | grep dstBlobContainer -B 3"), []string{}).Return(test.showStr, test.showErr)
		}

		azcopyFunc := &Azcopy{}
		azcopyFunc.ExecCmd = m
		jobState, percent, err := azcopyFunc.GetAzcopyJob(dstBlobContainer, []string{})
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
			desc:             "parse azcopy job CompletedWithErrors",
			str:              "JobId: ed1c3833-eaff-fe42-71d7-513fb065a9d9\nStart Time: Monday, 07-Aug-23 03:29:54 UTC\nStatus: CompletedWithErrors\nCommand: copy https://{accountName}.file.core.windows.net/{srcFileshare}{SAStoken} https://{accountName}.file.core.windows.net/{dstFileshare}{SAStoken} --recursive --check-length=false",
			expectedJobid:    "",
			expectedJobState: AzcopyJobCompletedWithErrors,
			expectedErr:      nil,
		},
		{
			desc:             "parse azcopy job CompletedWithSkipped",
			str:              "JobId: ed1c3833-eaff-fe42-71d7-513fb065a9d9\nStart Time: Monday, 07-Aug-23 03:29:54 UTC\nStatus: CompletedWithSkipped\nCommand: copy https://{accountName}.file.core.windows.net/{srcFileshare}{SAStoken} https://{accountName}.file.core.windows.net/{dstFileshare}{SAStoken} --recursive --check-length=false",
			expectedJobid:    "",
			expectedJobState: AzcopyJobCompletedWithSkipped,
			expectedErr:      nil,
		},
		{
			desc:             "parse azcopy job CompletedWithErrorsAndSkipped",
			str:              "JobId: ed1c3833-eaff-fe42-71d7-513fb065a9d9\nStart Time: Monday, 07-Aug-23 03:29:54 UTC\nStatus: CompletedWithErrorsAndSkipped\nCommand: copy https://{accountName}.file.core.windows.net/{srcFileshare}{SAStoken} https://{accountName}.file.core.windows.net/{dstFileshare}{SAStoken} --recursive --check-length=false",
			expectedJobid:    "",
			expectedJobState: AzcopyJobCompletedWithErrorsAndSkipped,
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
		jobid, jobState, err := parseAzcopyJobList(test.str)
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

func TestGetKubeConfig(t *testing.T) {
	emptyKubeConfig := "empty-Kube-Config"
	validKubeConfig := "valid-Kube-Config"
	nonexistingConfig := "nonexisting-config"
	fakeContent := `
apiVersion: v1
clusters:
- cluster:
    server: https://localhost:8080
  name: foo-cluster
contexts:
- context:
    cluster: foo-cluster
    user: foo-user
    namespace: bar
  name: foo-context
current-context: foo-context
kind: Config
users:
- name: foo-user
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1beta1
      args:
      - arg-1
      - arg-2
      command: foo-command
`
	if err := os.WriteFile(validKubeConfig, []byte(""), 0666); err != nil {
		t.Error(err)
	}
	defer os.Remove(emptyKubeConfig)
	if err := os.WriteFile(validKubeConfig, []byte(fakeContent), 0666); err != nil {
		t.Error(err)
	}
	defer os.Remove(validKubeConfig)

	tests := []struct {
		desc                     string
		kubeconfig               string
		expectError              bool
		envVariableHasConfig     bool
		envVariableConfigIsValid bool
	}{
		{
			desc:                     "[success] valid kube config passed",
			kubeconfig:               validKubeConfig,
			expectError:              false,
			envVariableHasConfig:     false,
			envVariableConfigIsValid: false,
		},
		{
			desc:                     "[failure] invalid kube config passed",
			kubeconfig:               emptyKubeConfig,
			expectError:              true,
			envVariableHasConfig:     false,
			envVariableConfigIsValid: false,
		},
		{
			desc:                     "[failure] invalid kube config passed",
			kubeconfig:               nonexistingConfig,
			expectError:              true,
			envVariableHasConfig:     false,
			envVariableConfigIsValid: false,
		},
		{
			desc:        "no-need-kubeconfig",
			kubeconfig:  "no-need-kubeconfig",
			expectError: false,
		},
	}

	for _, test := range tests {
		_, err := GetKubeClient(test.kubeconfig, 25.0, 50, "")
		receiveError := (err != nil)
		if test.expectError != receiveError {
			t.Errorf("desc: %s,\n input: %q, GetCloudProvider err: %v, expectErr: %v", test.desc, test.kubeconfig, err, test.expectError)
		}
	}
}

func TestVolumeMounter(t *testing.T) {
	path := "/mnt/data"
	attributes := volume.Attributes{}

	mounter := &VolumeMounter{
		path:       path,
		attributes: attributes,
	}

	if mounter.GetPath() != path {
		t.Errorf("Expected path %s, but got %s", path, mounter.GetPath())
	}

	if mounter.GetAttributes() != attributes {
		t.Errorf("Expected attributes %v, but got %v", attributes, mounter.GetAttributes())
	}

	if err := mounter.CanMount(); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if err := mounter.SetUp(volume.MounterArgs{}); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if err := mounter.SetUpAt("", volume.MounterArgs{}); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	metrics, err := mounter.GetMetrics()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if metrics != nil {
		t.Errorf("Expected nil metrics, but got %v", metrics)
	}
}

func TestSetVolumeOwnership(t *testing.T) {
	tmpVDir, err := os.MkdirTemp(os.TempDir(), "SetVolumeOwnership")
	if err != nil {
		t.Fatalf("can't make a temp dir: %v", err)
	}
	//deferred clean up
	defer os.RemoveAll(tmpVDir)

	tests := []struct {
		path                string
		gid                 string
		fsGroupChangePolicy string
		expectedError       error
	}{
		{
			path:          "path",
			gid:           "",
			expectedError: fmt.Errorf("convert %s to int failed with %v", "", `strconv.Atoi: parsing "": invalid syntax`),
		},
		{
			path:          "path",
			gid:           "alpha",
			expectedError: fmt.Errorf("convert %s to int failed with %v", "alpha", `strconv.Atoi: parsing "alpha": invalid syntax`),
		},
		{
			path:          "not-exists",
			gid:           "1000",
			expectedError: fmt.Errorf("lstat not-exists: no such file or directory"),
		},
		{
			path:          tmpVDir,
			gid:           "1000",
			expectedError: nil,
		},
		{
			path:                tmpVDir,
			gid:                 "1000",
			fsGroupChangePolicy: "Always",
			expectedError:       nil,
		},
		{
			path:                tmpVDir,
			gid:                 "1000",
			fsGroupChangePolicy: "OnRootMismatch",
			expectedError:       nil,
		},
	}

	for _, test := range tests {
		err := SetVolumeOwnership(test.path, test.gid, test.fsGroupChangePolicy)
		if err != nil && (err.Error() != test.expectedError.Error()) {
			t.Errorf("unexpected error: %v, expected error: %v", err, test.expectedError)
		}
	}
}

func TestWaitUntilTimeout(t *testing.T) {
	defer goleak.VerifyNone(t)
	tests := []struct {
		desc        string
		timeout     time.Duration
		execFunc    ExecFunc
		timeoutFunc TimeoutFunc
		expectedErr error
	}{
		{
			desc:    "execFunc returns error",
			timeout: 1 * time.Second,
			execFunc: func() error {
				return fmt.Errorf("execFunc error")
			},
			timeoutFunc: func() error {
				return fmt.Errorf("timeout error")
			},
			expectedErr: fmt.Errorf("execFunc error"),
		},
		{
			desc:    "execFunc timeout",
			timeout: 1 * time.Second,
			execFunc: func() error {
				time.Sleep(2 * time.Second)
				return nil
			},
			timeoutFunc: func() error {
				time.Sleep(2 * time.Second)
				return fmt.Errorf("timeout error")
			},
			expectedErr: fmt.Errorf("timeout error"),
		},
		{
			desc:    "execFunc completed successfully",
			timeout: 1 * time.Second,
			execFunc: func() error {
				return nil
			},
			timeoutFunc: func() error {
				return fmt.Errorf("timeout error")
			},
			expectedErr: nil,
		},
	}

	for _, test := range tests {
		err := WaitUntilTimeout(test.timeout, test.execFunc, test.timeoutFunc)
		if err != nil && (err.Error() != test.expectedErr.Error()) {
			t.Errorf("unexpected error: %v, expected error: %v", err, test.expectedErr)
		}
	}
}

func TestMakeDirAdditional(t *testing.T) {
	tests := []struct {
		name     string
		pathname string
		perm     os.FileMode
		wantErr  bool
	}{
		{
			name:     "create new directory with specific permissions",
			pathname: "/tmp/test-make-dir-additional",
			perm:     0700,
			wantErr:  false,
		},
		{
			name:     "fail to create directory with invalid path",
			pathname: "/invalid-root-path/that/does/not/exist",
			perm:     0755,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := MakeDir(tt.pathname, tt.perm)
			if (err != nil) != tt.wantErr {
				t.Errorf("MakeDir() error = %v, wantErr %v", err, tt.wantErr)
			}
			
			// Cleanup only if successful
			if err == nil {
				os.RemoveAll(tt.pathname)
			}
		})
	}
}

func TestExecCommand_RunCommand(t *testing.T) {
	ec := &ExecCommand{}
	
	tests := []struct {
		name     string
		cmdStr   string
		authEnv  []string
		wantErr  bool
	}{
		{
			name:     "simple echo command",
			cmdStr:   "echo hello",
			authEnv:  []string{},
			wantErr:  false,
		},
		{
			name:     "command with environment variable",
			cmdStr:   "echo $TEST_VAR",
			authEnv:  []string{"TEST_VAR=test_value"},
			wantErr:  false,
		},
		{
			name:     "invalid command",
			cmdStr:   "nonexistent_command_12345",
			authEnv:  []string{},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := ec.RunCommand(tt.cmdStr, tt.authEnv)
			if (err != nil) != tt.wantErr {
				t.Errorf("RunCommand() error = %v, wantErr %v", err, tt.wantErr)
			}
			
			if !tt.wantErr {
				assert.NotEmpty(t, output)
			}
		})
	}
}
