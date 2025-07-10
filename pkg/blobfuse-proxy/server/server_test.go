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
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"google.golang.org/grpc/codes"
	mount_azure_blob "sigs.k8s.io/blob-csi-driver/pkg/blobfuse-proxy/pb"
	"sigs.k8s.io/blob-csi-driver/pkg/blob"
)

func TestServerMountAzureBlob(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		args     string
		authEnv  []string
		protocol string
		code     codes.Code
	}{
		{
			name:     "failed_mount",
			args:     "--hello",
			authEnv:  []string{"hello"},
			protocol: "",
			code:     codes.InvalidArgument,
		},
		{
			name:     "failed_mount_with_fuse2",
			args:     "--hello",
			authEnv:  []string{"hello"},
			protocol: blob.Fuse2,
			code:     codes.InvalidArgument,
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
				Protocol:  tc.protocol,
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

func TestNewMountServiceServer(t *testing.T) {
	server := NewMountServiceServer()
	assert.NotNil(t, server)
	// blobfuseVersion should be set based on the OS
	assert.True(t, server.blobfuseVersion == BlobfuseV1 || server.blobfuseVersion == BlobfuseV2)
}

func TestGetBlobfuseVersion(t *testing.T) {
	// This will test the function based on the actual OS
	version := getBlobfuseVersion()
	assert.True(t, version == BlobfuseV1 || version == BlobfuseV2)
}

func TestRunGRPCServer(t *testing.T) {
	// Create a test listener
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	// Create a mock mount server
	mountServer := NewMountServiceServer()

	// Test with TLS disabled
	errCh := make(chan error, 1)
	go func() {
		errCh <- RunGRPCServer(mountServer, false, listener)
	}()

	// Close the listener to stop the server
	listener.Close()

	// The server should stop when the listener is closed
	select {
	case err := <-errCh:
		// We expect an error when the listener is closed
		assert.Error(t, err)
	}
}
