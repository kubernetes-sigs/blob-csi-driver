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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"sigs.k8s.io/cloud-provider-azure/pkg/provider"

	"github.com/Azure/azure-sdk-for-go/services/storage/mgmt/2021-09-01/storage"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	mount "k8s.io/mount-utils"
	utilexec "k8s.io/utils/exec"
	testingexec "k8s.io/utils/exec/testing"
)

const (
	sourceTest = "./source_test"
	targetTest = "./target_test"
)

func TestNodeGetInfo(t *testing.T) {
	d := NewFakeDriver()

	// Test valid request
	req := csi.NodeGetInfoRequest{}
	resp, err := d.NodeGetInfo(context.Background(), &req)
	assert.NoError(t, err)
	assert.Equal(t, resp.GetNodeId(), fakeNodeID)
}

func TestNodeGetCapabilities(t *testing.T) {
	d := NewFakeDriver()
	capType := &csi.NodeServiceCapability_Rpc{
		Rpc: &csi.NodeServiceCapability_RPC{
			Type: csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
		},
	}
	capList := []*csi.NodeServiceCapability{{
		Type: capType,
	}}
	d.NSCap = capList
	// Test valid request
	req := csi.NodeGetCapabilitiesRequest{}
	resp, err := d.NodeGetCapabilities(context.Background(), &req)
	assert.NotNil(t, resp)
	assert.Equal(t, resp.Capabilities[0].GetType(), capType)
	assert.NoError(t, err)
}

func TestEnsureMountPoint(t *testing.T) {
	errorTarget := "./error_is_likely_target"
	alreadyExistTarget := "./false_is_likely_exist_target"
	falseTarget := "./false_is_likely_target"
	azureFile := "./azure.go"

	tests := []struct {
		desc        string
		target      string
		expectedErr error
	}{
		{
			desc:        "[Error] Mocked by IsLikelyNotMountPoint",
			target:      errorTarget,
			expectedErr: fmt.Errorf("fake IsLikelyNotMountPoint: fake error"),
		},
		{
			desc:        "[Error] Error opening file",
			target:      falseTarget,
			expectedErr: &os.PathError{Op: "open", Path: "./false_is_likely_target", Err: syscall.ENOENT},
		},
		{
			desc:        "[Error] Not a directory",
			target:      azureFile,
			expectedErr: &os.PathError{Op: "mkdir", Path: "./azure.go", Err: syscall.ENOTDIR},
		},
		{
			desc:        "[Success] Successful run",
			target:      targetTest,
			expectedErr: nil,
		},
		{
			desc:        "[Success] Already existing mount",
			target:      alreadyExistTarget,
			expectedErr: nil,
		},
	}

	// Setup
	_ = makeDir(alreadyExistTarget)
	d := NewFakeDriver()
	fakeMounter := &fakeMounter{}
	fakeExec := &testingexec.FakeExec{ExactOrder: true}
	d.mounter = &mount.SafeFormatAndMount{
		Interface: fakeMounter,
		Exec:      fakeExec,
	}

	for _, test := range tests {
		_, err := d.ensureMountPoint(test.target, 0777)
		if !reflect.DeepEqual(err, test.expectedErr) {
			t.Errorf("[%s]: Unexpected Error: %v, expected error: %v", test.desc, err, test.expectedErr)
		}
	}

	// Clean up
	err := os.RemoveAll(alreadyExistTarget)
	assert.NoError(t, err)
	err = os.RemoveAll(targetTest)
	assert.NoError(t, err)
}

func TestNewMountClient(t *testing.T) {
	conn, _ := grpc.DialContext(context.TODO(), "unix://tmp/proxy.sock", grpc.WithInsecure())
	// No need to check for error because the socket is not available and
	// will result in following error: failed to build resolver: invalid (non-empty) authority: tmp
	client := NewMountClient(conn)
	valueType := reflect.TypeOf(client).String()
	assert.Equal(t, valueType, "*blob.MountClient")
}

func TestNodePublishVolume(t *testing.T) {
	volumeCap := csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER}
	createDirError := status.Errorf(codes.Internal,
		"Could not mount target \"./azure.go\": mkdir ./azure.go: not a directory")
	if runtime.GOOS == "windows" {
		createDirError = status.Errorf(codes.Internal,
			"Could not mount target \"./azure.go\": mkdir ./azure.go: "+
				"The system cannot find the path specified.")
	}
	tests := []struct {
		desc        string
		setup       func(*Driver)
		req         csi.NodePublishVolumeRequest
		expectedErr error
		cleanup     func(*Driver)
	}{
		{
			desc:        "Volume capabilities missing",
			req:         csi.NodePublishVolumeRequest{},
			expectedErr: status.Error(codes.InvalidArgument, "Volume capability missing in request"),
		},
		{
			desc:        "Volume ID missing",
			req:         csi.NodePublishVolumeRequest{VolumeCapability: &csi.VolumeCapability{AccessMode: &volumeCap}},
			expectedErr: status.Error(codes.InvalidArgument, "Volume ID missing in request"),
		},
		{
			desc: "Stage path missing",
			req: csi.NodePublishVolumeRequest{VolumeCapability: &csi.VolumeCapability{AccessMode: &volumeCap},
				VolumeId:   "vol_1",
				TargetPath: sourceTest},
			expectedErr: status.Error(codes.InvalidArgument, "Staging target not provided"),
		},
		{
			desc: "Stage target path missing",
			req: csi.NodePublishVolumeRequest{VolumeCapability: &csi.VolumeCapability{AccessMode: &volumeCap},
				VolumeId:          "vol_1",
				StagingTargetPath: sourceTest},
			expectedErr: status.Error(codes.InvalidArgument, "Target path not provided"),
		},
		{
			desc: "Valid request read only",
			req: csi.NodePublishVolumeRequest{
				VolumeCapability:  &csi.VolumeCapability{AccessMode: &volumeCap},
				VolumeId:          "vol_1",
				TargetPath:        targetTest,
				StagingTargetPath: sourceTest,
				VolumeContext: map[string]string{
					mountPermissionsField: "0755",
				},
				Readonly: true,
			},
			expectedErr: nil,
		},
		{
			desc: "Error creating directory",
			req: csi.NodePublishVolumeRequest{VolumeCapability: &csi.VolumeCapability{AccessMode: &volumeCap},
				VolumeId:          "vol_1",
				TargetPath:        "./azure.go",
				StagingTargetPath: sourceTest,
				Readonly:          true},
			expectedErr: createDirError,
		},
		{
			desc: "Error mounting resource busy",
			req: csi.NodePublishVolumeRequest{VolumeCapability: &csi.VolumeCapability{AccessMode: &volumeCap},
				VolumeId:          "vol_1",
				TargetPath:        targetTest,
				StagingTargetPath: sourceTest,
				Readonly:          true},
			expectedErr: nil,
		},
		{
			desc: "[Error] invalid mountPermissions",
			req: csi.NodePublishVolumeRequest{
				VolumeCapability:  &csi.VolumeCapability{AccessMode: &volumeCap},
				VolumeId:          "vol_1",
				TargetPath:        targetTest,
				StagingTargetPath: sourceTest,
				VolumeContext: map[string]string{
					mountPermissionsField: "07ab",
				},
			},
			expectedErr: status.Error(codes.InvalidArgument, fmt.Sprintf("invalid mountPermissions %s", "07ab")),
		},
	}

	// Setup
	_ = makeDir(sourceTest)
	_ = makeDir(targetTest)
	d := NewFakeDriver()
	d.cloud = provider.GetTestCloud(gomock.NewController(t))
	fakeMounter := &fakeMounter{}
	fakeExec := &testingexec.FakeExec{ExactOrder: true}
	d.mounter = &mount.SafeFormatAndMount{
		Interface: fakeMounter,
		Exec:      fakeExec,
	}

	for _, test := range tests {
		d.cloud.ResourceGroup = "rg"
		if test.setup != nil {
			test.setup(d)
		}
		_, err := d.NodePublishVolume(context.Background(), &test.req)

		if !reflect.DeepEqual(err, test.expectedErr) {
			t.Errorf("Desc: %s - Unexpected error: %v - Expected: %v", test.desc, err, test.expectedErr)
		}
		if test.cleanup != nil {
			test.cleanup(d)
		}
	}

	// Clean up
	_ = d.mounter.Unmount(sourceTest)
	err := d.mounter.Unmount(targetTest)
	assert.NoError(t, err)
	err = os.RemoveAll(sourceTest)
	assert.NoError(t, err)
	err = os.RemoveAll(targetTest)
	assert.NoError(t, err)
}

func TestNodePublishVolumeIdempotentMount(t *testing.T) {
	if runtime.GOOS != "linux" || os.Getuid() != 0 {
		return
	}
	_ = makeDir(sourceTest)
	_ = makeDir(targetTest)
	d := NewFakeDriver()
	d.mounter = &mount.SafeFormatAndMount{
		Interface: mount.New(""),
		Exec:      utilexec.New(),
	}

	volumeCap := csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER}
	req := csi.NodePublishVolumeRequest{VolumeCapability: &csi.VolumeCapability{AccessMode: &volumeCap},
		VolumeId:          "vol_1",
		TargetPath:        targetTest,
		StagingTargetPath: sourceTest,
		Readonly:          true}

	_, err := d.NodePublishVolume(context.Background(), &req)
	assert.NoError(t, err)
	_, err = d.NodePublishVolume(context.Background(), &req)
	assert.NoError(t, err)

	// ensure the target not be mounted twice
	targetAbs, err := filepath.Abs(targetTest)
	assert.NoError(t, err)

	mountList, err := d.mounter.List()
	assert.NoError(t, err)
	mountPointNum := 0
	for _, mountPoint := range mountList {
		if mountPoint.Path == targetAbs {
			mountPointNum++
		}
	}
	assert.Equal(t, 1, mountPointNum)
	err = d.mounter.Unmount(targetTest)
	assert.NoError(t, err)
	_ = d.mounter.Unmount(targetTest)
	err = os.RemoveAll(sourceTest)
	assert.NoError(t, err)
	err = os.RemoveAll(targetTest)
	assert.NoError(t, err)
}

func TestNodeUnpublishVolume(t *testing.T) {
	tests := []struct {
		desc        string
		setup       func(*Driver)
		req         csi.NodeUnpublishVolumeRequest
		expectedErr error
		cleanup     func(*Driver)
	}{
		{
			desc:        "Volume ID missing",
			req:         csi.NodeUnpublishVolumeRequest{TargetPath: targetTest},
			expectedErr: status.Error(codes.InvalidArgument, "Volume ID missing in request"),
		},
		{
			desc:        "Target missing",
			req:         csi.NodeUnpublishVolumeRequest{VolumeId: "vol_1"},
			expectedErr: status.Error(codes.InvalidArgument, "Target path missing in request"),
		},
		{
			desc:        "Valid request",
			req:         csi.NodeUnpublishVolumeRequest{TargetPath: "./abc.go", VolumeId: "vol_1"},
			expectedErr: nil,
		},
	}

	// Setup
	_ = makeDir(sourceTest)
	_ = makeDir(targetTest)
	d := NewFakeDriver()

	fakeMounter := &fakeMounter{}
	fakeExec := &testingexec.FakeExec{ExactOrder: true}
	d.mounter = &mount.SafeFormatAndMount{
		Interface: fakeMounter,
		Exec:      fakeExec,
	}

	for _, test := range tests {
		if test.setup != nil {
			test.setup(d)
		}
		_, err := d.NodeUnpublishVolume(context.Background(), &test.req)

		if !reflect.DeepEqual(err, test.expectedErr) {
			t.Errorf("Unexpected error: %v", err)
		}
		if test.cleanup != nil {
			test.cleanup(d)
		}
	}

	//Clean up
	err := d.mounter.Unmount(targetTest)
	assert.NoError(t, err)
	err = os.RemoveAll(sourceTest)
	assert.NoError(t, err)
	err = os.RemoveAll(targetTest)
	assert.NoError(t, err)
}

func TestNodeStageVolume(t *testing.T) {
	volumeCap := csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER}
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "Volume ID missing",
			testFunc: func(t *testing.T) {
				req := &csi.NodeStageVolumeRequest{}
				d := NewFakeDriver()
				_, err := d.NodeStageVolume(context.TODO(), req)
				expectedErr := status.Error(codes.InvalidArgument, "Volume ID missing in request")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "Staging target not provided",
			testFunc: func(t *testing.T) {
				req := &csi.NodeStageVolumeRequest{
					VolumeId: "unit-test",
				}
				d := NewFakeDriver()
				_, err := d.NodeStageVolume(context.TODO(), req)
				expectedErr := status.Error(codes.InvalidArgument, "Staging target not provided")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "Volume capability missing",
			testFunc: func(t *testing.T) {
				req := &csi.NodeStageVolumeRequest{
					VolumeId:          "unit-test",
					StagingTargetPath: "unit-test",
				}
				d := NewFakeDriver()
				_, err := d.NodeStageVolume(context.TODO(), req)
				expectedErr := status.Error(codes.InvalidArgument, "Volume capability not provided")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "Volume operation in progress",
			testFunc: func(t *testing.T) {
				req := &csi.NodeStageVolumeRequest{
					VolumeId:          "unit-test",
					StagingTargetPath: "unit-test",
					VolumeCapability:  &csi.VolumeCapability{AccessMode: &volumeCap},
				}
				d := NewFakeDriver()
				d.volumeLocks.TryAcquire("unit-test")
				defer d.volumeLocks.Release("unit-test")
				_, err := d.NodeStageVolume(context.TODO(), req)
				expectedErr := status.Error(codes.Aborted, fmt.Sprintf(volumeOperationAlreadyExistsFmt, "unit-test"))
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "[Error] invalid mountPermissions",
			testFunc: func(t *testing.T) {
				req := &csi.NodeStageVolumeRequest{
					VolumeId:          "unit-test",
					StagingTargetPath: "unit-test",
					VolumeCapability:  &csi.VolumeCapability{AccessMode: &volumeCap},
					VolumeContext: map[string]string{
						mountPermissionsField: "07ab",
					},
				}
				d := NewFakeDriver()
				_, err := d.NodeStageVolume(context.TODO(), req)
				expectedErr := status.Error(codes.InvalidArgument, fmt.Sprintf("invalid mountPermissions %s", "07ab"))
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "[Error] Could not mount to target",
			testFunc: func(t *testing.T) {
				req := &csi.NodeStageVolumeRequest{
					VolumeId:          "unit-test",
					StagingTargetPath: "error_is_likely",
					VolumeCapability:  &csi.VolumeCapability{AccessMode: &volumeCap},
					VolumeContext: map[string]string{
						mountPermissionsField: "0755",
					},
				}
				d := NewFakeDriver()
				fakeMounter := &fakeMounter{}
				fakeExec := &testingexec.FakeExec{}
				d.mounter = &mount.SafeFormatAndMount{
					Interface: fakeMounter,
					Exec:      fakeExec,
				}
				_, err := d.NodeStageVolume(context.TODO(), req)
				expectedErr := status.Error(codes.Internal, fmt.Sprintf("Could not mount target %q: %v", req.StagingTargetPath, fmt.Errorf("fake IsLikelyNotMountPoint: fake error")))
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "protocol = nfs",
			testFunc: func(t *testing.T) {
				req := &csi.NodeStageVolumeRequest{
					VolumeId:          "rg#acc#cont#ns",
					StagingTargetPath: targetTest,
					VolumeCapability:  &csi.VolumeCapability{AccessMode: &volumeCap},
					VolumeContext: map[string]string{
						mountPermissionsField: "0755",
						protocolField:         "nfs",
					},
					Secrets: map[string]string{},
				}
				d := NewFakeDriver()
				d.cloud = provider.GetTestCloud(gomock.NewController(t))
				d.cloud.ResourceGroup = "rg"
				d.enableBlobMockMount = true
				fakeMounter := &fakeMounter{}
				fakeExec := &testingexec.FakeExec{}
				d.mounter = &mount.SafeFormatAndMount{
					Interface: fakeMounter,
					Exec:      fakeExec,
				}

				_, err := d.NodeStageVolume(context.TODO(), req)
				//expectedErr := nil
				if !reflect.DeepEqual(err, nil) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, nil)
				}
			},
		},
		{
			name: "BlobMockMount Enabled",
			testFunc: func(t *testing.T) {
				req := &csi.NodeStageVolumeRequest{
					VolumeId:          "rg#acc#cont#ns",
					StagingTargetPath: targetTest,
					VolumeCapability:  &csi.VolumeCapability{AccessMode: &volumeCap},
					VolumeContext: map[string]string{
						mountPermissionsField: "0755",
						protocolField:         "protocol",
					},
					Secrets: map[string]string{},
				}
				d := NewFakeDriver()
				d.cloud = provider.GetTestCloud(gomock.NewController(t))
				d.cloud.ResourceGroup = "rg"
				d.enableBlobMockMount = true
				fakeMounter := &fakeMounter{}
				fakeExec := &testingexec.FakeExec{}
				d.mounter = &mount.SafeFormatAndMount{
					Interface: fakeMounter,
					Exec:      fakeExec,
				}

				keyList := make([]storage.AccountKey, 1)
				fakeKey := "fakeKey"
				fakeValue := "fakeValue"
				keyList[0] = (storage.AccountKey{
					KeyName: &fakeKey,
					Value:   &fakeValue,
				})
				d.cloud.StorageAccountClient = NewMockSAClient(context.Background(), gomock.NewController(t), "subID", "unit-test", "unit-test", &keyList)

				_, err := d.NodeStageVolume(context.TODO(), req)
				//expectedErr := nil
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

func TestNodeUnstageVolume(t *testing.T) {
	volumeCap := csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER}
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "Volume ID missing",
			testFunc: func(t *testing.T) {
				req := &csi.NodeUnstageVolumeRequest{}
				d := NewFakeDriver()
				_, err := d.NodeUnstageVolume(context.TODO(), req)
				expectedErr := status.Error(codes.InvalidArgument, "Volume ID not provided")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "staging target missing",
			testFunc: func(t *testing.T) {
				req := &csi.NodeUnstageVolumeRequest{
					VolumeId: "unit-test",
				}
				d := NewFakeDriver()
				_, err := d.NodeUnstageVolume(context.TODO(), req)
				expectedErr := status.Error(codes.InvalidArgument, "Staging target not provided")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "Volume operation in progress",
			testFunc: func(t *testing.T) {
				req := &csi.NodeStageVolumeRequest{
					VolumeId:          "unit-test",
					StagingTargetPath: "unit-test",
					VolumeCapability:  &csi.VolumeCapability{AccessMode: &volumeCap},
				}
				d := NewFakeDriver()
				d.volumeLocks.TryAcquire("unit-test")
				defer d.volumeLocks.Release("unit-test")
				_, err := d.NodeStageVolume(context.TODO(), req)
				expectedErr := status.Error(codes.Aborted, fmt.Sprintf(volumeOperationAlreadyExistsFmt, "unit-test"))
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "mount point not exist ",
			testFunc: func(t *testing.T) {
				req := &csi.NodeUnstageVolumeRequest{
					VolumeId:          "unit-test",
					StagingTargetPath: "./unit-test",
				}
				d := NewFakeDriver()
				fakeMounter := &fakeMounter{}
				fakeExec := &testingexec.FakeExec{}
				d.mounter = &mount.SafeFormatAndMount{
					Interface: fakeMounter,
					Exec:      fakeExec,
				}
				_, err := d.NodeUnstageVolume(context.TODO(), req)
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

func makeDir(pathname string) error {
	err := os.MkdirAll(pathname, os.FileMode(0755))
	if err != nil {
		if !os.IsExist(err) {
			return err
		}
	}
	return nil
}

func TestMakeDir(t *testing.T) {
	//Successfully create directory
	err := makeDir(targetTest)
	assert.NoError(t, err)

	//Failed case
	err = makeDir("./azure.go")
	var e *os.PathError
	if !errors.As(err, &e) {
		t.Errorf("Unexpected Error: %v", err)
	}

	// Remove the directory created
	err = os.RemoveAll(targetTest)
	assert.NoError(t, err)
}

func NewSafeMounter() (*mount.SafeFormatAndMount, error) {
	return &mount.SafeFormatAndMount{
		Interface: mount.New(""),
	}, nil
}

func TestNewSafeMounter(t *testing.T) {
	resp, err := NewSafeMounter()
	assert.NotNil(t, resp)
	assert.Nil(t, err)
}

func TestNodeGetVolumeStats(t *testing.T) {
	nonexistedPath := "/not/a/real/directory"
	fakePath := "/tmp/fake-volume-path"

	tests := []struct {
		desc        string
		req         csi.NodeGetVolumeStatsRequest
		expectedErr error
	}{
		{
			desc:        "[Error] Volume ID missing",
			req:         csi.NodeGetVolumeStatsRequest{VolumePath: targetTest},
			expectedErr: status.Error(codes.InvalidArgument, "NodeGetVolumeStats volume ID was empty"),
		},
		{
			desc:        "[Error] VolumePath missing",
			req:         csi.NodeGetVolumeStatsRequest{VolumeId: "vol_1"},
			expectedErr: status.Error(codes.InvalidArgument, "NodeGetVolumeStats volume path was empty"),
		},
		{
			desc:        "[Error] Incorrect volume path",
			req:         csi.NodeGetVolumeStatsRequest{VolumePath: nonexistedPath, VolumeId: "vol_1"},
			expectedErr: status.Errorf(codes.NotFound, "path /not/a/real/directory does not exist"),
		},
		{
			desc:        "[Success] Standard success",
			req:         csi.NodeGetVolumeStatsRequest{VolumePath: fakePath, VolumeId: "vol_1"},
			expectedErr: nil,
		},
	}

	// Setup
	_ = makeDir(fakePath)
	d := NewFakeDriver()

	for _, test := range tests {
		_, err := d.NodeGetVolumeStats(context.Background(), &test.req)
		//t.Errorf("[debug] error: %v\n metrics: %v", err, metrics)
		if !reflect.DeepEqual(err, test.expectedErr) {
			t.Errorf("desc: %v, expected error: %v, actual error: %v", test.desc, test.expectedErr, err)
		}
	}

	// Clean up
	err := os.RemoveAll(fakePath)
	assert.NoError(t, err)
}

func TestNodeExpandVolume(t *testing.T) {
	d := NewFakeDriver()
	req := csi.NodeExpandVolumeRequest{}
	resp, err := d.NodeExpandVolume(context.Background(), &req)
	assert.Nil(t, resp)
	if !reflect.DeepEqual(err, status.Error(codes.Unimplemented, "NodeExpandVolume is not yet implemented")) {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestMountBlobfuseWithProxy(t *testing.T) {
	args := "--tmp-path /tmp"
	authEnv := []string{"username=blob", "authkey=blob"}
	d := NewFakeDriver()
	_, err := d.mountBlobfuseWithProxy(args, "fuse", authEnv)
	// should be context.deadlineExceededError{} error
	assert.NotNil(t, err)
}

func TestMountBlobfuseInsideDriver(t *testing.T) {
	args := "--tmp-path /tmp"
	authEnv := []string{"username=blob", "authkey=blob"}
	d := NewFakeDriver()
	_, err := d.mountBlobfuseInsideDriver(args, Fuse, authEnv)
	// the error should be of type exec.ExitError
	assert.NotNil(t, err)
}

func Test_waitForMount(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		t.Skip("Skipping test on ", runtime.GOOS)
	}

	tmpDir, err := os.MkdirTemp("", "")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	type args struct {
		path     string
		intervel time.Duration
		timeout  time.Duration
	}

	tests := []struct {
		name      string
		args      args
		wantErr   bool
		subErrMsg string
	}{
		{
			name: "test error timeout",
			args: args{
				path:     tmpDir,
				intervel: 1 * time.Millisecond,
				timeout:  10 * time.Millisecond,
			},
			wantErr:   true,
			subErrMsg: "timeout",
		},
		{
			name: "test error no such file or directory",
			args: args{
				path:     "/no/such/file/or/directory",
				intervel: 1 * time.Millisecond,
				timeout:  10 * time.Millisecond,
			},
			wantErr:   true,
			subErrMsg: "no such file or directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := waitForMount(tt.args.path, tt.args.intervel, tt.args.timeout)
			if (err != nil) != tt.wantErr {
				t.Errorf("waitForMount() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), tt.subErrMsg) {
				t.Errorf("waitForMount() error = %v, wantErr %v", err, tt.subErrMsg)
			}
		})
	}
}
