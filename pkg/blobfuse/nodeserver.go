/*
Copyright 2017 The Kubernetes Authors.

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

package blobfuse

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"

	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/util/mount"
	"k8s.io/kubernetes/pkg/volume/util"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"golang.org/x/net/context"
)

// NodePublishVolume mount the volume from staging to target path
func (d *Driver) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	klog.V(2).Infof("NodePublishVolume: called with args %+v", *req)
	if req.GetVolumeCapability() == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume capability missing in request")
	}
	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}

	source := req.GetStagingTargetPath()
	if len(source) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Staging target not provided")
	}

	target := req.GetTargetPath()
	if len(target) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path not provided")
	}

	mountOptions := []string{"bind"}
	if req.GetReadonly() {
		mountOptions = append(mountOptions, "ro")
	}

	if err := d.ensureMountPoint(target); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not mount target %q: %v", target, err)
	}

	klog.V(2).Infof("NodePublishVolume: mounting %s at %s with mountOptions: %v", source, target, mountOptions)
	if err := d.mounter.Mount(source, target, "", mountOptions); err != nil {
		if removeErr := os.Remove(target); removeErr != nil {
			return nil, status.Errorf(codes.Internal, "Could not remove mount target %q: %v", target, removeErr)
		}
		return nil, status.Errorf(codes.Internal, "Could not mount %q at %q: %v", source, target, err)
	}
	klog.V(2).Infof("NodePublishVolume: mount %s at %s successfully", source, target)

	return &csi.NodePublishVolumeResponse{}, nil
}

// NodeUnpublishVolume unmount the volume from the target path
func (d *Driver) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	klog.V(2).Infof("NodeUnPublishVolume: called with args %+v", *req)
	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}
	if len(req.GetTargetPath()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path missing in request")
	}
	targetPath := req.GetTargetPath()
	volumeID := req.GetVolumeId()

	klog.V(2).Infof("NodeUnpublishVolume: unmounting volume %s on %s", volumeID, targetPath)
	err := mount.CleanupMountPoint(targetPath, d.mounter, false)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to unmount target %q: %v", targetPath, err)
	}
	klog.V(2).Infof("NodeUnpublishVolume: unmount volume %s on %s successfully", volumeID, targetPath)

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

// NodeStageVolume mount the volume to a staging path
func (d *Driver) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}
	targetPath := req.GetStagingTargetPath()
	if len(targetPath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Staging target not provided")
	}
	volumeCapability := req.GetVolumeCapability()
	if volumeCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume capability not provided")
	}

	if err := d.ensureMountPoint(targetPath); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not mount target %q: %v", targetPath, err)
	}

	volumeID := req.GetVolumeId()
	fsType := req.GetVolumeCapability().GetMount().GetFsType()
	mountFlags := req.GetVolumeCapability().GetMount().GetMountFlags()
	attrib := req.GetVolumeContext()
	secrets := req.GetSecrets()

	accountName, containerName, authEnv, err := d.GetAuthEnv(ctx, volumeID, attrib, secrets)
	if err != nil {
		return nil, err
	}

	// Get mountOptions that the volume will be formatted and mounted with
	options := []string{"--use-https=true"}
	mountOptions := util.JoinMountOptions(mountFlags, options)

	args := targetPath + " " + "--tmp-path=/mnt/" + volumeID + " " + "--container-name=" + containerName
	for _, opt := range mountOptions {
		args = args + " " + opt
	}

	var blobStorageEndPoint string
	for k, v := range attrib {
		switch strings.ToLower(k) {
		case serverNameField:
			blobStorageEndPoint = v
		}
	}
	if strings.TrimSpace(blobStorageEndPoint) == "" {
		// server address is "accountname.blob.core.windows.net" by default
		blobStorageEndPoint = fmt.Sprintf("%s.blob.%s", accountName, d.cloud.Environment.StorageEndpointSuffix)
	}

	klog.V(2).Infof("target %v\nfstype %v\n\nvolumeId %v\ncontext %v\nmountflags %v\nmountOptions %v\nargs %v\nblobStorageEndPoint %v",
		targetPath, fsType, volumeID, attrib, mountFlags, mountOptions, args, blobStorageEndPoint)
	cmd := exec.Command("blobfuse", strings.Split(args, " ")...)

	cmd.Env = append(os.Environ(), "AZURE_STORAGE_ACCOUNT="+accountName)
	cmd.Env = append(cmd.Env, "AZURE_STORAGE_BLOB_ENDPOINT="+blobStorageEndPoint)
	cmd.Env = append(cmd.Env, authEnv...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("Mount failed with error: %v, output: %v", err, string(output))
		klog.Errorf("%v", err)
		notMnt, mntErr := d.mounter.IsLikelyNotMountPoint(targetPath)
		if mntErr != nil {
			klog.Errorf("IsLikelyNotMountPoint check failed: %v", mntErr)
			return nil, err
		}
		if !notMnt {
			if mntErr = d.mounter.Unmount(targetPath); mntErr != nil {
				klog.Errorf("Failed to unmount: %v", mntErr)
				return nil, err
			}
			notMnt, mntErr := d.mounter.IsLikelyNotMountPoint(targetPath)
			if mntErr != nil {
				klog.Errorf("IsLikelyNotMountPoint check failed: %v", mntErr)
				return nil, err
			}
			if !notMnt {
				// This is very odd, we don't expect it.  We'll try again next sync loop.
				klog.Errorf("%s is still mounted, despite call to unmount().  Will try again next sync loop.", targetPath)
				return nil, err
			}
		}
		os.Remove(targetPath)
		return nil, err
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

// NodeUnstageVolume unmount the volume from the staging path
func (d *Driver) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	klog.V(2).Infof("NodeUnstageVolume: called with args %+v", *req)
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	stagingTargetPath := req.GetStagingTargetPath()
	if len(stagingTargetPath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Staging target not provided")
	}

	klog.V(2).Infof("NodeUnstageVolume: unmounting %s", stagingTargetPath)
	err := mount.CleanupMountPoint(stagingTargetPath, d.mounter, false)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to unmount staing target %q: %v", stagingTargetPath, err)
	}
	klog.V(2).Infof("NodeUnstageVolume: unmount %s successfully", stagingTargetPath)

	return &csi.NodeUnstageVolumeResponse{}, nil
}

// NodeGetCapabilities return the capabilities of the Node plugin
func (d *Driver) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	klog.V(2).Infof("NodeGetCapabilities: called with args %+v", *req)

	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: d.NSCap,
	}, nil
}

// NodeGetInfo return info of the node on which this plugin is running
func (d *Driver) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	klog.V(2).Infof("NodeGetInfo: called with args %+v", *req)

	return &csi.NodeGetInfoResponse{
		NodeId: d.NodeID,
	}, nil
}

// NodeGetVolumeStats get volume stats
func (d *Driver) NodeGetVolumeStats(ctx context.Context, in *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

// NodeExpandVolume node expand volume
func (d *Driver) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "NodeExpandVolume is not yet implemented")
}

// ensureMountPoint: create mount point if not exists
func (d *Driver) ensureMountPoint(target string) error {
	notMnt, err := d.mounter.IsLikelyNotMountPoint(target)
	if err != nil && !os.IsNotExist(err) {
		if IsCorruptedDir(target) {
			notMnt = false
			klog.Warningf("detected corrupted mount for targetPath [%s]", target)
		} else {
			return err
		}
	}

	if !notMnt {
		// testing original mount point, make sure the mount link is valid
		_, err := ioutil.ReadDir(target)
		if err == nil {
			klog.V(2).Infof("already mounted to target %s", target)
			return nil
		}
		// mount link is invalid, now unmount and remount later
		klog.Warningf("ReadDir %s failed with %v, unmount this directory", target, err)
		if err := d.mounter.Unmount(target); err != nil {
			klog.Errorf("Unmount directory %s failed with %v", target, err)
			return err
		}
		// notMnt = true
	}

	if err := d.mounter.MakeDir(target); err != nil {
		klog.Errorf("MakeDir failed on target: %s (%v)", target, err)
		return err
	}

	return nil
}
