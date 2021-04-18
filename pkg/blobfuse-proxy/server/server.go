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
	"os/exec"
	"strings"
	"sync"

	"google.golang.org/grpc"
	"k8s.io/klog/v2"
	mount_azure_blob "sigs.k8s.io/blob-csi-driver/pkg/blobfuse-proxy/pb"
)

var (
	mutex sync.Mutex
)

type MountServer struct {
	mount_azure_blob.UnimplementedMountServiceServer
}

// NewMountServer returns a new Mountserver
func NewMountServiceServer() *MountServer {
	return &MountServer{}
}

// MountAzureBlob mounts an azure blob container to given location
func (server *MountServer) MountAzureBlob(ctx context.Context,
	req *mount_azure_blob.MountAzureBlobRequest,
) (resp *mount_azure_blob.MountAzureBlobResponse, err error) {
	mutex.Lock()
	defer mutex.Unlock()

	args := req.GetMountArgs()
	authEnv := req.GetAuthEnv()
	klog.V(2).Infof("received mount request: Mounting with args %v \n", args)

	var result mount_azure_blob.MountAzureBlobResponse
	cmd := exec.Command("blobfuse", strings.Split(args, " ")...)

	cmd.Env = append(cmd.Env, authEnv...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		klog.Error("blobfuse mount failed: with error:", err.Error())
	} else {
		klog.V(2).Infof("successfully mounted")
	}
	result.Output = string(output)
	klog.V(2).Infof("blobfuse output: %s\n", result.Output)
	return &result, err
}

func RunGRPCServer(
	mountServer mount_azure_blob.MountServiceServer,
	enableTLS bool,
	listener net.Listener,
) error {
	serverOptions := []grpc.ServerOption{}
	grpcServer := grpc.NewServer(serverOptions...)

	mount_azure_blob.RegisterMountServiceServer(grpcServer, mountServer)

	klog.V(2).Infof("Start GRPC server at %s, TLS = %t", listener.Addr().String(), enableTLS)
	return grpcServer.Serve(listener)
}
