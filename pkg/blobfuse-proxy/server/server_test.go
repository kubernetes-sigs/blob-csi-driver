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
	"testing"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc/codes"
	"sigs.k8s.io/blob-csi-driver/pkg/blob"
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

func fakeExecCommandWithDistributedCacheHelp(_ string, _ ...string) *exec.Cmd {
	return exec.Command("echo", distributedCacheDiscoveryFlag)
}

func fakeExecCommandWithoutDistributedCacheHelp(_ string, _ ...string) *exec.Cmd {
	return exec.Command("echo", "--block-cache")
}

func fakeExecCommandFailure(_ string, _ ...string) *exec.Cmd {
	return exec.Command("sh", "-c", "echo probe-failed >&2; exit 1")
}

func TestGetBlobfuseCapabilities(t *testing.T) {
	tests := []struct {
		name      string
		exec      func(string, ...string) *exec.Cmd
		supported bool
		wantError string
	}{
		{
			name:      "distributed cache supported",
			exec:      fakeExecCommandWithDistributedCacheHelp,
			supported: true,
		},
		{
			name: "distributed cache unsupported",
			exec: fakeExecCommandWithoutDistributedCacheHelp,
		},
		{
			name:      "help probe fails",
			exec:      fakeExecCommandFailure,
			wantError: "probe-failed",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := &MountServer{exec: test.exec}
			response, err := server.GetBlobfuseCapabilities(context.Background(), &mount_azure_blob.BlobfuseCapabilitiesRequest{
				Protocol: blob.Fuse2,
			})
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				require.Nil(t, response)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.supported, response.GetDistributedCacheSupported())
		})
	}
}
