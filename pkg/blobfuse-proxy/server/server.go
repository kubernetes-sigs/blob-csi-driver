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
	"fmt"
	"net"
	"os/exec"
	"strings"
	"sync"

	"google.golang.org/grpc"
	"k8s.io/klog/v2"
	"sigs.k8s.io/blob-csi-driver/pkg/blob"
	mount_azure_blob "sigs.k8s.io/blob-csi-driver/pkg/blobfuse-proxy/pb"
	"sigs.k8s.io/blob-csi-driver/pkg/util"
)

var (
	mutex sync.Mutex
)

type BlobfuseVersion int

const (
	BlobfuseV1 BlobfuseVersion = iota
	BlobfuseV2
)

type MountServer struct {
	blobfuseVersion BlobfuseVersion
	mount_azure_blob.UnimplementedMountServiceServer
}

// NewMountServer returns a new Mountserver
func NewMountServiceServer() *MountServer {
	mountServer := &MountServer{}
	mountServer.blobfuseVersion = getBlobfuseVersion()
	return mountServer
}

// MountAzureBlob mounts an azure blob container to given location
func (server *MountServer) MountAzureBlob(ctx context.Context,
	req *mount_azure_blob.MountAzureBlobRequest,
) (resp *mount_azure_blob.MountAzureBlobResponse, err error) {
	mutex.Lock()
	defer mutex.Unlock()

	args := req.GetMountArgs()
	authEnv := req.GetAuthEnv()
	protocol := req.GetProtocol()
	klog.V(2).Infof("received mount request: protocol: %s, server default blobfuseVersion: %v, mount args %v \n", protocol, server.blobfuseVersion, args)

	var cmd *exec.Cmd
	var result mount_azure_blob.MountAzureBlobResponse
	if protocol == blob.Fuse2 || server.blobfuseVersion == BlobfuseV2 {
		args = "mount " + args
		// add this arg for blobfuse2 to solve the issue:
		// https://github.com/Azure/azure-storage-fuse/issues/1015
		if !strings.Contains(args, "--ignore-open-flags") {
			klog.V(2).Infof("append --ignore-open-flags=true to mount args")
			args = args + " " + "--ignore-open-flags=true"
		}
		klog.V(2).Infof("mount with v2, protocol: %s, args: %s", protocol, args)
		cmd = exec.Command("blobfuse2", strings.Split(args, " ")...)
	} else {
		klog.V(2).Infof("mount with v1, protocol: %s, args: %s", protocol, args)
		cmd = exec.Command("blobfuse", strings.Split(args, " ")...)
	}

	cmd.Env = append(cmd.Env, authEnv...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		klog.Error("blobfuse mount failed: with error:", err.Error())
	} else {
		klog.V(2).Infof("successfully mounted")
	}
	result.Output = string(output)
	klog.V(2).Infof("blobfuse output: %s\n", result.Output)
	if err != nil {
		return &result, fmt.Errorf("%w %s", err, result.Output)
	}
	return &result, nil
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

func getBlobfuseVersion() BlobfuseVersion {
	osinfo, err := util.GetOSInfo("/etc/lsb-release")
	if err != nil {
		klog.Warningf("failed to get OS info: %v, default using blobfuse v1", err)
		return BlobfuseV1
	}

	if osinfo.Distro == "Ubuntu" && osinfo.Version >= "22.04" {
		klog.V(2).Info("proxy default using blobfuse V2 for mounting")
		return BlobfuseV2
	}

	klog.V(2).Info("proxy default using blobfuse V1 for mounting")
	return BlobfuseV1
}
