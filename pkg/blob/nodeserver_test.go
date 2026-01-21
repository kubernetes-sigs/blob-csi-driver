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
	"net"
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
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/mock_azclient"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	mount "k8s.io/mount-utils"
	utilexec "k8s.io/utils/exec"
	testingexec "k8s.io/utils/exec/testing"
	mount_azure_blob "sigs.k8s.io/blob-csi-driver/pkg/blobfuse-proxy/pb"
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
		req         *csi.NodePublishVolumeRequest
		expectedErr error
		cleanup     func(*Driver)
	}{
		{
			desc:        "Volume capabilities missing",
			req:         &csi.NodePublishVolumeRequest{},
			expectedErr: status.Error(codes.InvalidArgument, "Volume capability missing in request"),
		},
		{
			desc:        "Volume ID missing",
			req:         &csi.NodePublishVolumeRequest{VolumeCapability: &csi.VolumeCapability{AccessMode: &volumeCap}},
			expectedErr: status.Error(codes.InvalidArgument, "Volume ID missing in request"),
		},
		{
			desc: "Stage path missing",
			req: &csi.NodePublishVolumeRequest{VolumeCapability: &csi.VolumeCapability{AccessMode: &volumeCap},
				VolumeId:   "vol_1",
				TargetPath: sourceTest},
			expectedErr: status.Error(codes.InvalidArgument, "Staging target not provided"),
		},
		{
			desc: "Stage target path missing",
			req: &csi.NodePublishVolumeRequest{VolumeCapability: &csi.VolumeCapability{AccessMode: &volumeCap},
				VolumeId:          "vol_1",
				StagingTargetPath: sourceTest},
			expectedErr: status.Error(codes.InvalidArgument, "Target path not provided"),
		},
		{
			desc: "Valid request read only",
			req: &csi.NodePublishVolumeRequest{
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
			req: &csi.NodePublishVolumeRequest{VolumeCapability: &csi.VolumeCapability{AccessMode: &volumeCap},
				VolumeId:          "vol_1",
				TargetPath:        "./azure.go",
				StagingTargetPath: sourceTest,
				Readonly:          true},
			expectedErr: createDirError,
		},
		{
			desc: "Error mounting resource busy",
			req: &csi.NodePublishVolumeRequest{VolumeCapability: &csi.VolumeCapability{AccessMode: &volumeCap},
				VolumeId:          "vol_1",
				TargetPath:        targetTest,
				StagingTargetPath: sourceTest,
				Readonly:          true},
			expectedErr: nil,
		},
		{
			desc: "[Error] invalid mountPermissions",
			req: &csi.NodePublishVolumeRequest{
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
		{
			desc: "Valid request with service account token and clientID",
			setup: func(d *Driver) {
				// Mock NodeStageVolume to return success
				d.cloud.ResourceGroup = "rg"
				d.enableBlobMockMount = true
				// Create the directory for token file
				defaultAzureOAuthTokenDir = "./blob.csi.azure.com/"
				_ = makeDir(defaultAzureOAuthTokenDir)
			},
			cleanup: func(_ *Driver) {
				_ = os.RemoveAll(defaultAzureOAuthTokenDir)
			},
			req: &csi.NodePublishVolumeRequest{
				VolumeCapability:  &csi.VolumeCapability{AccessMode: &volumeCap},
				VolumeId:          "vol_1",
				TargetPath:        targetTest,
				StagingTargetPath: sourceTest,
				VolumeContext: map[string]string{
					serviceAccountTokenField: `{"api://AzureADTokenExchange":{"token":"test-token","expirationTimestamp":"2023-01-01T00:00:00Z"}}`,
					mountWithWITokenField:    "true",
					clientIDField:            "client-id-value",
					storageAccountNameField:  "test-account",
				},
			},
			expectedErr: nil,
		},
		{
			desc: "Valid request with ephemeral volume",
			setup: func(d *Driver) {
				// Mock NodeStageVolume to return success
				d.cloud.ResourceGroup = "rg"
				d.enableBlobMockMount = true
			},
			req: &csi.NodePublishVolumeRequest{
				VolumeCapability:  &csi.VolumeCapability{AccessMode: &volumeCap},
				VolumeId:          "vol_1",
				TargetPath:        targetTest,
				StagingTargetPath: sourceTest,
				VolumeContext: map[string]string{
					"csi.storage.k8s.io/ephemeral":     "true",
					"csi.storage.k8s.io/pod.namespace": "test-namespace",
					"containername":                    "test-container", // Add container name to avoid error
				},
			},
			expectedErr: nil,
		},
		{
			desc: "Volume already mounted",
			setup: func(_ *Driver) {
				// Create the directory and ensure it's seen as already mounted
				_ = makeDir("./false_is_likely")
			},
			req: &csi.NodePublishVolumeRequest{
				VolumeCapability:  &csi.VolumeCapability{AccessMode: &volumeCap},
				VolumeId:          "vol_1",
				TargetPath:        "./false_is_likely", // This path will make IsLikelyNotMountPoint return false
				StagingTargetPath: sourceTest,
			},
			expectedErr: nil,
			cleanup: func(_ *Driver) {
				// Clean up the directory
				_ = os.RemoveAll("./false_is_likely")
			},
		},
		{
			desc: "enableBlobMockMount enabled",
			setup: func(d *Driver) {
				// Enable mock mount
				d.enableBlobMockMount = true

				// Create a temporary directory for the test
				_ = makeDir("./mock_mount_test")
			},
			req: &csi.NodePublishVolumeRequest{
				VolumeCapability:  &csi.VolumeCapability{AccessMode: &volumeCap},
				VolumeId:          "vol_1",
				TargetPath:        "./mock_mount_test",
				StagingTargetPath: sourceTest,
				VolumeContext: map[string]string{
					mountPermissionsField: "0755",
				},
			},
			expectedErr: nil,
			cleanup: func(d *Driver) {
				// Disable mock mount
				d.enableBlobMockMount = false

				// Clean up the directory
				_ = os.RemoveAll("./mock_mount_test")
			},
		},
		{
			desc: "enableBlobMockMount enabled - MakeDir fails",
			setup: func(d *Driver) {
				// Enable mock mount
				d.enableBlobMockMount = true
			},
			req: &csi.NodePublishVolumeRequest{
				VolumeCapability:  &csi.VolumeCapability{AccessMode: &volumeCap},
				VolumeId:          "vol_1",
				TargetPath:        "./azure.go", // This will fail because it's a file, not a directory
				StagingTargetPath: sourceTest,
				VolumeContext: map[string]string{
					mountPermissionsField: "0755",
				},
			},
			expectedErr: func() error {
				if runtime.GOOS == "windows" {
					t.Skip("Skipping test on ", runtime.GOOS)
					return nil
				}
				return status.Errorf(codes.Internal, "Could not mount target \"./azure.go\": mkdir ./azure.go: not a directory")
			}(),
		},
	}

	// Setup
	_ = makeDir(sourceTest)
	_ = makeDir(targetTest)
	d := NewFakeDriver()
	//d.cloud = provider.GetTestCloud(gomock.NewController(t))
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
		_, err := d.NodePublishVolume(context.Background(), test.req)

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

func TestNodePublishVolumeMountError(t *testing.T) {
	d := NewFakeDriver()
	fakeMounter := &fakeMounter{}
	fakeExec := &testingexec.FakeExec{ExactOrder: true}
	d.mounter = &mount.SafeFormatAndMount{
		Interface: fakeMounter,
		Exec:      fakeExec,
	}

	volumeCap := csi.VolumeCapability_AccessMode{
		Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
	}

	// Test case 1: Mount fails with source error
	targetPath := "./mount_test_target"
	_ = makeDir(targetPath)

	req := &csi.NodePublishVolumeRequest{
		VolumeId:          "vol_1",
		TargetPath:        targetPath,
		StagingTargetPath: "error_mount", // This will trigger an error in Mount
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &volumeCap,
		},
	}

	// Run the test
	_, err := d.NodePublishVolume(context.Background(), req)
	expectedErr := status.Errorf(codes.Internal, "Could not mount \"error_mount\" at \"./mount_test_target\": fake Mount: source error")
	if err == nil || err.Error() != expectedErr.Error() {
		t.Errorf("Expected error: %v, got: %v", expectedErr, err)
	}

	// Clean up
	_ = os.RemoveAll(targetPath)

	// Let's try a different approach
	// Let's create a custom fakeMounter that returns a specific error for Mount

	// We'll continue to use the same fakeMounter
	// It's already configured to return an error for paths containing "error_mount"

	// Create a test directory that doesn't exist
	nonExistentPath := "/tmp/non-existent-path-" + uuid.NewString()

	req = &csi.NodePublishVolumeRequest{
		VolumeId:          "vol_1",
		TargetPath:        nonExistentPath,
		StagingTargetPath: "error_mount", // This will trigger an error in Mount
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &volumeCap,
		},
	}

	// Run the test
	_, err = d.NodePublishVolume(context.Background(), req)

	// The error should be about failing to mount, not about removing the target
	// because the target doesn't exist
	expectedErr = status.Errorf(codes.Internal, "Could not mount \"error_mount\" at \"%s\": fake Mount: source error", nonExistentPath)
	if err == nil || err.Error() != expectedErr.Error() {
		t.Errorf("Expected error: %v, got: %v", expectedErr, err)
	}
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
		req         *csi.NodeUnpublishVolumeRequest
		expectedErr error
		cleanup     func(*Driver)
	}{
		{
			desc:        "Volume ID missing",
			req:         &csi.NodeUnpublishVolumeRequest{TargetPath: targetTest},
			expectedErr: status.Error(codes.InvalidArgument, "Volume ID missing in request"),
		},
		{
			desc:        "Target missing",
			req:         &csi.NodeUnpublishVolumeRequest{VolumeId: "vol_1"},
			expectedErr: status.Error(codes.InvalidArgument, "Target path missing in request"),
		},
		{
			desc:        "Valid request",
			req:         &csi.NodeUnpublishVolumeRequest{TargetPath: "./abc.go", VolumeId: "vol_1"},
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
		_, err := d.NodeUnpublishVolume(context.Background(), test.req)

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
				d.volumeLocks.TryAcquire(fmt.Sprintf("%s-%s", "unit-test", "unit-test"))
				defer d.volumeLocks.Release(fmt.Sprintf("%s-%s", "unit-test", "unit-test"))
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
			name: "[Error] Invalid fsGroupChangePolicy",
			testFunc: func(t *testing.T) {
				req := &csi.NodeStageVolumeRequest{
					VolumeId:          "unit-test",
					StagingTargetPath: "unit-test",
					VolumeCapability:  &csi.VolumeCapability{AccessMode: &volumeCap},
					VolumeContext: map[string]string{
						fsGroupChangePolicyField: "test_fsGroupChangePolicy",
					},
				}
				d := NewFakeDriver()
				_, err := d.NodeStageVolume(context.TODO(), req)
				expectedErr := status.Error(codes.InvalidArgument, "fsGroupChangePolicy(test_fsGroupChangePolicy) is not supported, supported fsGroupChangePolicy list: [None Always OnRootMismatch]")
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
				//d.cloud = provider.GetTestCloud(gomock.NewController(t))
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
				//d.cloud = provider.GetTestCloud(gomock.NewController(t))
				d.cloud.ResourceGroup = "rg"
				d.enableBlobMockMount = true
				fakeMounter := &fakeMounter{}
				fakeExec := &testingexec.FakeExec{}
				d.mounter = &mount.SafeFormatAndMount{
					Interface: fakeMounter,
					Exec:      fakeExec,
				}

				keyList := make([]*armstorage.AccountKey, 1)
				fakeKey := "fakeKey"
				fakeValue := "fakeValue"
				keyList[0] = (&armstorage.AccountKey{
					KeyName: &fakeKey,
					Value:   &fakeValue,
				})
				mockStorageAccountsClient := NewMockSAClient(context.Background(), gomock.NewController(t), "subID", "unit-test", "unit-test", keyList)
				d.cloud.ComputeClientFactory = mock_azclient.NewMockClientFactory(gomock.NewController(t))
				d.cloud.ComputeClientFactory.(*mock_azclient.MockClientFactory).EXPECT().GetAccountClient().Return(mockStorageAccountsClient).AnyTimes()
				d.cloud.ComputeClientFactory.(*mock_azclient.MockClientFactory).EXPECT().GetAccountClientForSub(gomock.Any()).Return(mockStorageAccountsClient, nil).AnyTimes()

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
				d.volumeLocks.TryAcquire(fmt.Sprintf("%s-%s", "unit-test", "unit-test"))
				defer d.volumeLocks.Release(fmt.Sprintf("%s-%s", "unit-test", "unit-test"))
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
		req         *csi.NodeGetVolumeStatsRequest
		expectedErr error
	}{
		{
			desc:        "[Error] Volume ID missing",
			req:         &csi.NodeGetVolumeStatsRequest{VolumePath: targetTest},
			expectedErr: status.Error(codes.InvalidArgument, "NodeGetVolumeStats volume ID was empty"),
		},
		{
			desc:        "[Error] VolumePath missing",
			req:         &csi.NodeGetVolumeStatsRequest{VolumeId: "vol_1"},
			expectedErr: status.Error(codes.InvalidArgument, "NodeGetVolumeStats volume path was empty"),
		},
		{
			desc:        "[Error] Incorrect volume path",
			req:         &csi.NodeGetVolumeStatsRequest{VolumePath: nonexistedPath, VolumeId: "vol_1"},
			expectedErr: status.Errorf(codes.NotFound, "path /not/a/real/directory does not exist"),
		},
		{
			desc:        "[Success] Standard success",
			req:         &csi.NodeGetVolumeStatsRequest{VolumePath: fakePath, VolumeId: "vol_1"},
			expectedErr: nil,
		},
	}

	// Setup
	_ = makeDir(fakePath)
	d := NewFakeDriver()

	for _, test := range tests {
		_, err := d.NodeGetVolumeStats(context.Background(), test.req)
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

// fakeMountServiceServer implements mount_azure_blob.MountServiceServer for testing
type fakeMountServiceServer struct {
	mount_azure_blob.MountServiceServer
	mockOutput string
	mockError  error
}

// MountAzureBlob implements the mount service method
func (s *fakeMountServiceServer) MountAzureBlob(_ context.Context, _ *mount_azure_blob.MountAzureBlobRequest) (*mount_azure_blob.MountAzureBlobResponse, error) {
	return &mount_azure_blob.MountAzureBlobResponse{
		Output: s.mockOutput,
	}, s.mockError
}

func TestMountBlobfuseWithProxy(t *testing.T) {
	tests := []struct {
		desc         string
		setup        func(*testing.T) (*Driver, *grpc.Server, net.Listener)
		args         string
		protocol     string
		authEnv      []string
		expectedOut  string
		expectedErr  error
		cleanupCheck func(*testing.T, string, error)
	}{
		{
			desc: "Success case with mock server",
			setup: func(t *testing.T) (*Driver, *grpc.Server, net.Listener) {
				// Create a mock server for the success test
				mockServer := &fakeMountServiceServer{
					mockOutput: "mock mount successful",
					mockError:  nil,
				}

				// Start a local gRPC server for testing
				lis, err := net.Listen("tcp", "127.0.0.1:0") // Use random available port
				if err != nil {
					t.Fatalf("Failed to listen: %v", err)
				}
				s := grpc.NewServer()
				mount_azure_blob.RegisterMountServiceServer(s, mockServer)

				// Start the server in a goroutine
				go func() {
					if err := s.Serve(lis); err != nil && !strings.Contains(err.Error(), "use of closed network connection") {
						t.Logf("Server exited with error: %v", err)
					}
				}()

				// Create the driver with the configured proxy
				d := NewFakeDriver()
				d.blobfuseProxyEndpoint = lis.Addr().String()
				d.blobfuseProxyConnTimeout = 5 // 5 second timeout

				return d, s, lis
			},
			args:        "--tmp-path /tmp",
			protocol:    "fuse",
			authEnv:     []string{"username=blob", "authkey=blob"},
			expectedOut: "mock mount successful",
			expectedErr: nil,
			cleanupCheck: func(t *testing.T, output string, err error) {
				assert.NoError(t, err)
				assert.Equal(t, "mock mount successful", output)
			},
		},
		{
			desc: "Error from MountAzureBlob",
			setup: func(t *testing.T) (*Driver, *grpc.Server, net.Listener) {
				// Create a mock server for the error test
				mockServer := &fakeMountServiceServer{
					mockOutput: "",
					mockError:  fmt.Errorf("mock mount error"),
				}

				// Start a local gRPC server for testing
				lis, err := net.Listen("tcp", "127.0.0.1:0") // Use random available port
				if err != nil {
					t.Fatalf("Failed to listen: %v", err)
				}
				s := grpc.NewServer()
				mount_azure_blob.RegisterMountServiceServer(s, mockServer)

				// Start the server in a goroutine
				go func() {
					if err := s.Serve(lis); err != nil && !strings.Contains(err.Error(), "use of closed network connection") {
						t.Logf("Server exited with error: %v", err)
					}
				}()

				// Create the driver with the configured proxy
				d := NewFakeDriver()
				d.blobfuseProxyEndpoint = lis.Addr().String()
				d.blobfuseProxyConnTimeout = 5 // 5 second timeout

				return d, s, lis
			},
			args:        "--tmp-path /tmp",
			protocol:    "fuse",
			authEnv:     []string{"username=blob", "authkey=blob"},
			expectedOut: "",
			expectedErr: fmt.Errorf("mock mount error"),
			cleanupCheck: func(t *testing.T, _ string, err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "mock mount error")
			},
		},
		{
			desc: "Connection error case",
			setup: func(_ *testing.T) (*Driver, *grpc.Server, net.Listener) {
				// Create the driver with a non-existent endpoint
				d := NewFakeDriver()
				d.blobfuseProxyEndpoint = "unix://non-existent-socket.sock"
				d.blobfuseProxyConnTimeout = 1 // 1 second timeout for quick failure

				// No server or listener for this test
				return d, nil, nil
			},
			args:        "--tmp-path /tmp",
			protocol:    "fuse",
			authEnv:     []string{"username=blob", "authkey=blob"},
			expectedOut: "",
			expectedErr: fmt.Errorf("failed to exit idle mode: invalid (non-empty) authority: non-existent-socket.sock"),
			cleanupCheck: func(t *testing.T, output string, err error) {
				assert.Error(t, err)
				assert.Empty(t, output)
				assert.Contains(t, err.Error(), "invalid (non-empty) authority: non-existent-socket.sock")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			// Setup the test
			d, s, lis := test.setup(t)

			// Cleanup resources if needed
			if s != nil && lis != nil {
				defer s.Stop()
				defer lis.Close()
			}

			// Run the test - call mountBlobfuseWithProxy directly
			output, err := d.mountBlobfuseWithProxy(test.args, test.protocol, test.authEnv)

			// Verify results
			if test.cleanupCheck != nil {
				test.cleanupCheck(t, output, err)
			}
		})
	}
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

func TestCheckGidPresentInMountFlags(t *testing.T) {
	tests := []struct {
		desc       string
		MountFlags []string
		result     bool
	}{
		{
			desc:       "[Success] Gid present in mount flags",
			MountFlags: []string{"gid=3000"},
			result:     true,
		},
		{
			desc:       "[Success] Gid present in mount flags",
			MountFlags: []string{"-o gid=3000"},
			result:     true,
		},
		{
			desc:       "[Success] Gid not present in mount flags",
			MountFlags: []string{},
			result:     false,
		},
	}

	for _, test := range tests {
		gIDPresent := checkGidPresentInMountFlags(test.MountFlags)
		if gIDPresent != test.result {
			t.Errorf("[%s]: Expected result : %t, Actual result: %t", test.desc, test.result, gIDPresent)
		}
	}
}

func TestUseWorkloadIdentity(t *testing.T) {
	tests := []struct {
		name   string
		attrib map[string]string
		want   bool
	}{
		{
			name: "clientID present",
			attrib: map[string]string{
				clientIDField: "client-id",
			},
			want: true,
		},
		{
			name: "mountWithWIToken true",
			attrib: map[string]string{
				mountWithWITokenField: trueValue,
			},
			want: true,
		},
		{
			name: "mountWithWIToken false",
			attrib: map[string]string{
				mountWithWITokenField: "false",
			},
			want: false,
		},
		{
			name:   "no workload identity fields",
			attrib: map[string]string{},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := useWorkloadIdentity(tt.attrib); got != tt.want {
				t.Errorf("useWorkloadIdentity() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetServiceAccountTokens(t *testing.T) {
	tests := []struct {
		name          string
		secrets       map[string]string
		volumeContext map[string]string
		expected      string
	}{
		{
			name: "token from secrets field (new behavior)",
			secrets: map[string]string{
				serviceAccountTokenField: "token-from-secrets",
			},
			volumeContext: map[string]string{
				serviceAccountTokenField: "token-from-context",
			},
			expected: "token-from-secrets",
		},
		{
			name:    "token from volume context (backward compatible)",
			secrets: map[string]string{},
			volumeContext: map[string]string{
				serviceAccountTokenField: "token-from-context",
			},
			expected: "token-from-context",
		},
		{
			name:          "no token available",
			secrets:       map[string]string{},
			volumeContext: map[string]string{},
			expected:      "",
		},
		{
			name:    "nil secrets falls back to volume context",
			secrets: nil,
			volumeContext: map[string]string{
				serviceAccountTokenField: "token-from-context",
			},
			expected: "token-from-context",
		},
		{
			name: "nil volume context with secrets",
			secrets: map[string]string{
				serviceAccountTokenField: "token-from-secrets",
			},
			volumeContext: nil,
			expected:      "token-from-secrets",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := getServiceAccountTokens(test.secrets, test.volumeContext)
			assert.Equal(t, test.expected, result)
		})
	}
}
