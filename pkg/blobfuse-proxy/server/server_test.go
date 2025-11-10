/*
Copyright 2021 The Kubernetes Authors.

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

package server

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc/codes"
	mount_azure_blob "sigs.k8s.io/blob-csi-driver/pkg/blobfuse-proxy/pb"
)

func TestServerMountAzureBlob(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		args    string
		authEnv []string
		code    codes.Code
	}{
		{
			name:    "failed_mount",
			args:    "--hello",
			authEnv: []string{"hello"},
			code:    codes.InvalidArgument,
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			mountServer := NewMountServiceServer()
			req := mount_azure_blob.MountAzureBlobRequest{
				MountArgs: tc.args,
				AuthEnv:   tc.authEnv,
			}
			res, err := mountServer.MountAzureBlob(context.Background(), &req)
			if tc.code == codes.OK {
				require.NoError(t, err)
				require.NotNil(t, res)
			} else {
				require.Error(t, err)
				require.NotNil(t, res)
			}
		})
	}
}

// fakeExecCommand is used to mock exec.Command for testing, it returns list of args
func fakeExecCommandEchoArgs(_ string, args ...string) *exec.Cmd {
	return exec.Command("echo", append([]string{"-n"}, args...)...)
}

func TestAddTelemetryTagToArgs(t *testing.T) {
	driverVersion = "fake-version-ut"
	t.Parallel()
	testCases := []struct {
		name         string
		args         string
		expectedArgs string
	}{
		{
			name:         "args_without_telemetry_option",
			args:         "--account-name=testaccount --container-name=testcontainer --tmp-path=/tmp/blobfuse-tmp",
			expectedArgs: "--account-name=testaccount --container-name=testcontainer --tmp-path=/tmp/blobfuse-tmp --telemetry=" + telemetryTagPrefix + driverVersion,
		},
		{
			name:         "args_with_some_telemetry_option",
			args:         "--account-name=testaccount --container-name=testcontainer --telemetry=app1-volume1 --tmp-path=/tmp/blobfuse-tmp",
			expectedArgs: "--account-name=testaccount --container-name=testcontainer --telemetry=" + telemetryTagPrefix + driverVersion + ",app1-volume1 --tmp-path=/tmp/blobfuse-tmp",
		},
		{
			name:         "args_with_csi_driver_telemetry_option",
			args:         "--account-name=testaccount --container-name=testcontainer --telemetry=" + telemetryTagPrefix + driverVersion + ",app1-volume1 --tmp-path=/tmp/blobfuse-tmp",
			expectedArgs: "--account-name=testaccount --container-name=testcontainer --telemetry=" + telemetryTagPrefix + driverVersion + ",app1-volume1 --tmp-path=/tmp/blobfuse-tmp",
		},
		{
			name:         "args_with_some_telemetry_option_only",
			args:         "--telemetry=app1-volume1",
			expectedArgs: "--telemetry=" + telemetryTagPrefix + driverVersion + ",app1-volume1",
		},
		{
			name:         "args_with_multiple_telemetry_options",
			args:         "--account-name=testaccount --container-name=testcontainer --telemetry=app1 --tmp-path=/tmp/blobfuse-tmp --telemetry=app2",
			expectedArgs: "--account-name=testaccount --container-name=testcontainer --telemetry=" + telemetryTagPrefix + driverVersion + ",app1 --tmp-path=/tmp/blobfuse-tmp --telemetry=app2",
		},
	}
	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			actualArgs := addTelemetryTagToArgs(tc.args)
			require.Equal(t, tc.expectedArgs, actualArgs)
		})
	}
}

func TestServerMountAzureBlob_Telemetry(t *testing.T) {
	driverVersion = "fake-version"
	t.Parallel()
	testCases := []struct {
		name                  string
		args                  string
		code                  codes.Code
		mountServer           MountServer
		areValidTelemetryArgs func(cmdArgs string) bool
	}{
		{
			name: "mount_with_telemetry_tag_blobfusev2",
			args: "--account-name=testaccount --container-name=testcontainer --telemetry=volume1-app1 --tmp-path=/tmp/blobfuse-tmp",
			mountServer: MountServer{
				blobfuseVersion: BlobfuseV2,
				exec:            fakeExecCommandEchoArgs,
			},
			code: codes.OK,
			areValidTelemetryArgs: func(cmdArgs string) bool {
				expectedTelemetryArg := "--telemetry=" + telemetryTagPrefix + driverVersion + ",volume1-app1"
				return strings.Contains(cmdArgs, expectedTelemetryArg)
			},
		},
		{
			name: "mount_without_telemetry_tag_blobfusev2",
			args: "--account-name=testaccount --container-name=testcontainer --tmp-path=/tmp/blobfuse-tmp",
			mountServer: MountServer{
				blobfuseVersion: BlobfuseV2,
				exec:            fakeExecCommandEchoArgs,
			},
			code: codes.OK,
			areValidTelemetryArgs: func(cmdArgs string) bool {
				expectedTelemetryArg := "--telemetry=" + telemetryTagPrefix + driverVersion
				return strings.Contains(cmdArgs, expectedTelemetryArg)
			},
		},
		{
			name: "mount_with_blobfusev1",
			args: "--account-name=testaccount --container-name=testcontainer --tmp-path=/tmp/blobfuse-tmp",
			mountServer: MountServer{
				blobfuseVersion: BlobfuseV1,
				exec:            fakeExecCommandEchoArgs,
			},
			code: codes.OK,
			areValidTelemetryArgs: func(cmdArgs string) bool {
				// No telemetry arg should be added for blobfuse v1
				return !strings.Contains(cmdArgs, "--telemetry=") && cmdArgs == "--account-name=testaccount --container-name=testcontainer --tmp-path=/tmp/blobfuse-tmp"
			},
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := mount_azure_blob.MountAzureBlobRequest{
				MountArgs: tc.args,
				AuthEnv:   []string{},
			}
			res, err := tc.mountServer.MountAzureBlob(context.Background(), &req)
			if tc.code == codes.OK {
				require.NoError(t, err)
				require.NotNil(t, res)
				require.True(t, tc.areValidTelemetryArgs(res.Output), "telemetry args are mismatching in command args: %s", res.Output)
			} else {
				require.Error(t, err)
				require.NotNil(t, res)
			}
		})
	}
}
