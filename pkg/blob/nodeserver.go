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

package blob

import (
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	volumehelper "sigs.k8s.io/blob-csi-driver/pkg/util"

	"github.com/Azure/azure-sdk-for-go/storage"
	"github.com/container-storage-interface/spec/lib/go/csi"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/volume"
	"k8s.io/kubernetes/pkg/volume/util"
	mount "k8s.io/mount-utils"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	mount_azure_blob "sigs.k8s.io/blob-csi-driver/pkg/blobfuse-proxy/pb"
)

const (
	waitForMountInterval = 20 * time.Millisecond
	waitForMountTimeout  = 3 * time.Second
)

type MountClient struct {
	service mount_azure_blob.MountServiceClient
}

// NewMountClient returns a new mount client
func NewMountClient(cc *grpc.ClientConn) *MountClient {
	service := mount_azure_blob.NewMountServiceClient(cc)
	return &MountClient{service}
}

// NodePublishVolume mount the volume from staging to target path
func (d *Driver) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	volCap := req.GetVolumeCapability()
	if volCap == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume capability missing in request")
	}
	volumeID := req.GetVolumeId()
	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}

	target := req.GetTargetPath()
	if len(target) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path not provided")
	}

	mountPermissions := d.mountPermissions
	context := req.GetVolumeContext()
	if context != nil {
		if strings.EqualFold(context[ephemeralField], trueValue) {
			setKeyValueInMap(context, secretNamespaceField, context[podNamespaceField])
			if !d.allowInlineVolumeKeyAccessWithIdentity {
				// only get storage account from secret
				setKeyValueInMap(context, getAccountKeyFromSecretField, trueValue)
				setKeyValueInMap(context, storageAccountField, "")
			}
			klog.V(2).Infof("NodePublishVolume: ephemeral volume(%s) mount on %s, VolumeContext: %v", volumeID, target, context)
			_, err := d.NodeStageVolume(ctx, &csi.NodeStageVolumeRequest{
				StagingTargetPath: target,
				VolumeContext:     context,
				VolumeCapability:  volCap,
				VolumeId:          volumeID,
			})
			return &csi.NodePublishVolumeResponse{}, err
		}

		if perm := context[mountPermissionsField]; perm != "" {
			var err error
			if mountPermissions, err = strconv.ParseUint(perm, 8, 32); err != nil {
				return nil, status.Errorf(codes.InvalidArgument, fmt.Sprintf("invalid mountPermissions %s", perm))
			}
		}
	}

	source := req.GetStagingTargetPath()
	if len(source) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Staging target not provided")
	}

	mountOptions := []string{"bind"}
	if req.GetReadonly() {
		mountOptions = append(mountOptions, "ro")
	}

	mnt, err := d.ensureMountPoint(target, fs.FileMode(mountPermissions))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not mount target %q: %v", target, err)
	}
	if mnt {
		klog.V(2).Infof("NodePublishVolume: volume %s is already mounted on %s", volumeID, target)
		return &csi.NodePublishVolumeResponse{}, nil
	}

	klog.V(2).Infof("NodePublishVolume: volume %s mounting %s at %s with mountOptions: %v", volumeID, source, target, mountOptions)
	if d.enableBlobMockMount {
		klog.Warningf("NodePublishVolume: mock mount on volumeID(%s), this is only for TESTING!!!", volumeID)
		if err := volumehelper.MakeDir(target, os.FileMode(mountPermissions)); err != nil {
			klog.Errorf("MakeDir failed on target: %s (%v)", target, err)
			return nil, status.Errorf(codes.Internal, err.Error())
		}
		return &csi.NodePublishVolumeResponse{}, nil
	}

	if err := d.mounter.Mount(source, target, "", mountOptions); err != nil {
		if removeErr := os.Remove(target); removeErr != nil {
			return nil, status.Errorf(codes.Internal, "Could not remove mount target %q: %v", target, removeErr)
		}
		return nil, status.Errorf(codes.Internal, "Could not mount %q at %q: %v", source, target, err)
	}
	klog.V(2).Infof("NodePublishVolume: volume %s mount %s at %s successfully", volumeID, source, target)

	return &csi.NodePublishVolumeResponse{}, nil
}

func (d *Driver) mountBlobfuseWithProxy(args, protocol string, authEnv []string) (string, error) {
	var resp *mount_azure_blob.MountAzureBlobResponse
	var output string
	connectionTimout := time.Duration(d.blobfuseProxyConnTimout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimout)
	defer cancel()
	klog.V(2).Infof("start connecting to blobfuse proxy, protocol: %s, args: %s", protocol, args)
	conn, err := grpc.DialContext(ctx, d.blobfuseProxyEndpoint, grpc.WithInsecure(), grpc.WithBlock())
	if err == nil {
		mountClient := NewMountClient(conn)
		mountreq := mount_azure_blob.MountAzureBlobRequest{
			MountArgs: args,
			Protocol:  protocol,
			AuthEnv:   authEnv,
		}
		klog.V(2).Infof("begin to mount with blobfuse proxy, protocol: %s, args: %s", protocol, args)
		resp, err = mountClient.service.MountAzureBlob(context.TODO(), &mountreq)
		if err != nil {
			klog.Error("GRPC call returned with an error:", err)
		}
		output = resp.GetOutput()
	}
	return output, err
}

func (d *Driver) mountBlobfuseInsideDriver(args string, protocol string, authEnv []string) (string, error) {
	var cmd *exec.Cmd

	mountLog := "mount inside driver with"
	if protocol == Fuse2 {
		mountLog += " v2"
		args = "mount " + args
		cmd = exec.Command("blobfuse2", strings.Split(args, " ")...)
	} else {
		mountLog += " v1"
		cmd = exec.Command("blobfuse", strings.Split(args, " ")...)
	}
	klog.V(2).Infof("%s, protocol: %s, args: %s", mountLog, protocol, args)

	cmd.Env = append(os.Environ(), authEnv...)
	output, err := cmd.CombinedOutput()
	klog.V(2).Infof("mount output: %s\n", string(output))

	return string(output), err
}

// NodeUnpublishVolume unmount the volume from the target path
func (d *Driver) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}
	targetPath := req.GetTargetPath()
	if len(targetPath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path missing in request")
	}

	klog.V(2).Infof("NodeUnpublishVolume: unmounting volume %s on %s", volumeID, targetPath)
	err := mount.CleanupMountPoint(targetPath, d.mounter, true /*extensiveMountPointCheck*/)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to unmount target %q: %v", targetPath, err)
	}
	klog.V(2).Infof("NodeUnpublishVolume: unmount volume %s on %s successfully", volumeID, targetPath)

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

// NodeStageVolume mount the volume to a staging path
func (d *Driver) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
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

	if acquired := d.volumeLocks.TryAcquire(volumeID); !acquired {
		return nil, status.Errorf(codes.Aborted, volumeOperationAlreadyExistsFmt, volumeID)
	}
	defer d.volumeLocks.Release(volumeID)

	mountFlags := req.GetVolumeCapability().GetMount().GetMountFlags()
	attrib := req.GetVolumeContext()
	secrets := req.GetSecrets()

	var serverAddress, storageEndpointSuffix, protocol, ephemeralVolMountOptions string
	var ephemeralVol, isHnsEnabled bool

	containerNameReplaceMap := map[string]string{}

	mountPermissions := d.mountPermissions
	performChmodOp := (mountPermissions > 0)
	for k, v := range attrib {
		switch strings.ToLower(k) {
		case serverNameField:
			serverAddress = v
		case protocolField:
			protocol = v
		case storageEndpointSuffixField:
			storageEndpointSuffix = v
		case ephemeralField:
			ephemeralVol = strings.EqualFold(v, trueValue)
		case mountOptionsField:
			ephemeralVolMountOptions = v
		case isHnsEnabledField:
			isHnsEnabled = strings.EqualFold(v, trueValue)
		case pvcNamespaceKey:
			containerNameReplaceMap[pvcNamespaceMetadata] = v
		case pvcNameKey:
			containerNameReplaceMap[pvcNameMetadata] = v
		case pvNameKey:
			containerNameReplaceMap[pvNameMetadata] = v
		case mountPermissionsField:
			if v != "" {
				var err error
				var perm uint64
				if perm, err = strconv.ParseUint(v, 8, 32); err != nil {
					return nil, status.Errorf(codes.InvalidArgument, fmt.Sprintf("invalid mountPermissions %s", v))
				}
				if perm == 0 {
					performChmodOp = false
				} else {
					mountPermissions = perm
				}
			}
		}
	}

	mnt, err := d.ensureMountPoint(targetPath, fs.FileMode(mountPermissions))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not mount target %q: %v", targetPath, err)
	}
	if mnt {
		klog.V(2).Infof("NodeStageVolume: volume %s is already mounted on %s", volumeID, targetPath)
		return &csi.NodeStageVolumeResponse{}, nil
	}

	_, accountName, _, containerName, authEnv, err := d.GetAuthEnv(ctx, volumeID, protocol, attrib, secrets)
	if err != nil {
		return nil, status.Errorf(codes.Internal, err.Error())
	}

	// replace pv/pvc name namespace metadata in subDir
	containerName = replaceWithMap(containerName, containerNameReplaceMap)

	if strings.TrimSpace(storageEndpointSuffix) == "" {
		if d.cloud.Environment.StorageEndpointSuffix != "" {
			storageEndpointSuffix = d.cloud.Environment.StorageEndpointSuffix
		} else {
			storageEndpointSuffix = storage.DefaultBaseURL
		}
	}

	if strings.TrimSpace(serverAddress) == "" {
		// server address is "accountname.blob.core.windows.net" by default
		serverAddress = fmt.Sprintf("%s.blob.%s", accountName, storageEndpointSuffix)
	}

	if protocol == NFS {
		klog.V(2).Infof("target %v\nprotocol %v\n\nvolumeId %v\ncontext %v\nmountflags %v\nserverAddress %v",
			targetPath, protocol, volumeID, attrib, mountFlags, serverAddress)

		source := fmt.Sprintf("%s:/%s/%s", serverAddress, accountName, containerName)
		mountOptions := util.JoinMountOptions(mountFlags, []string{"sec=sys,vers=3,nolock"})
		if err := wait.PollImmediate(1*time.Second, 2*time.Minute, func() (bool, error) {
			return true, d.mounter.MountSensitive(source, targetPath, NFS, mountOptions, []string{})
		}); err != nil {
			var helpLinkMsg string
			if d.appendMountErrorHelpLink {
				helpLinkMsg = "\nPlease refer to http://aka.ms/blobmounterror for possible causes and solutions for mount errors."
			}
			return nil, status.Error(codes.Internal, fmt.Sprintf("volume(%s) mount %q on %q failed with %v%s", volumeID, source, targetPath, err, helpLinkMsg))
		}

		if performChmodOp {
			if err := chmodIfPermissionMismatch(targetPath, os.FileMode(mountPermissions)); err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
		} else {
			klog.V(2).Infof("skip chmod on targetPath(%s) since mountPermissions is set as 0", targetPath)
		}

		klog.V(2).Infof("volume(%s) mount %s on %s succeeded", volumeID, source, targetPath)
		return &csi.NodeStageVolumeResponse{}, nil
	}

	// Get mountOptions that the volume will be formatted and mounted with
	mountOptions := mountFlags
	if ephemeralVol {
		mountOptions = util.JoinMountOptions(mountOptions, strings.Split(ephemeralVolMountOptions, ","))
	}
	if isHnsEnabled {
		mountOptions = util.JoinMountOptions(mountOptions, []string{"--use-adls=true"})
	}
	tmpPath := fmt.Sprintf("%s/%s", "/mnt", volumeID)
	if d.appendTimeStampInCacheDir {
		tmpPath += fmt.Sprintf("#%d", time.Now().Unix())
	}
	mountOptions = appendDefaultMountOptions(mountOptions, tmpPath, containerName)

	args := targetPath
	for _, opt := range mountOptions {
		args = args + " " + opt
	}

	klog.V(2).Infof("target %v\nprotocol %v\n\nvolumeId %v\ncontext %v\nmountflags %v\nmountOptions %v\nargs %v\nserverAddress %v",
		targetPath, protocol, volumeID, attrib, mountFlags, mountOptions, args, serverAddress)

	authEnv = append(authEnv, "AZURE_STORAGE_ACCOUNT="+accountName, "AZURE_STORAGE_BLOB_ENDPOINT="+serverAddress)
	if d.enableBlobMockMount {
		klog.Warningf("NodeStageVolume: mock mount on volumeID(%s), this is only for TESTING!!!", volumeID)
		if err := volumehelper.MakeDir(targetPath, os.FileMode(mountPermissions)); err != nil {
			klog.Errorf("MakeDir failed on target: %s (%v)", targetPath, err)
			return nil, status.Errorf(codes.Internal, err.Error())
		}
		return &csi.NodeStageVolumeResponse{}, nil
	}

	var output string
	if d.enableBlobfuseProxy {
		output, err = d.mountBlobfuseWithProxy(args, protocol, authEnv)
	} else {
		output, err = d.mountBlobfuseInsideDriver(args, protocol, authEnv)
	}

	if err != nil {
		var helpLinkMsg string
		if d.appendMountErrorHelpLink {
			helpLinkMsg = "\nPlease refer to http://aka.ms/blobmounterror for possible causes and solutions for mount errors."
		}
		err = status.Errorf(codes.Internal, "Mount failed with error: %v, output: %v%s", err, output, helpLinkMsg)
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

	// wait a few seconds to make sure blobfuse mount is successful
	// please refer to https://github.com/Azure/azure-storage-fuse/pull/1088 for more details
	if err := waitForMount(targetPath, waitForMountInterval, waitForMountTimeout); err != nil {
		return nil, fmt.Errorf("failed to wait for mount: %w", err)
	}

	klog.V(2).Infof("volume(%s) mount on %q succeeded", volumeID, targetPath)
	return &csi.NodeStageVolumeResponse{}, nil
}

// NodeUnstageVolume unmount the volume from the staging path
func (d *Driver) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	stagingTargetPath := req.GetStagingTargetPath()
	if len(stagingTargetPath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Staging target not provided")
	}

	if acquired := d.volumeLocks.TryAcquire(volumeID); !acquired {
		return nil, status.Errorf(codes.Aborted, volumeOperationAlreadyExistsFmt, volumeID)
	}
	defer d.volumeLocks.Release(volumeID)

	klog.V(2).Infof("NodeUnstageVolume: volume %s unmounting on %s", volumeID, stagingTargetPath)
	err := mount.CleanupMountPoint(stagingTargetPath, d.mounter, true /*extensiveMountPointCheck*/)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to unmount staging target %q: %v", stagingTargetPath, err)
	}
	klog.V(2).Infof("NodeUnstageVolume: volume %s unmount on %s successfully", volumeID, stagingTargetPath)

	return &csi.NodeUnstageVolumeResponse{}, nil
}

// NodeGetCapabilities return the capabilities of the Node plugin
func (d *Driver) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: d.NSCap,
	}, nil
}

// NodeGetInfo return info of the node on which this plugin is running
func (d *Driver) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{
		NodeId: d.NodeID,
	}, nil
}

// NodeExpandVolume node expand volume
func (d *Driver) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "NodeExpandVolume is not yet implemented")
}

// NodeGetVolumeStats get volume stats
func (d *Driver) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	if len(req.VolumeId) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodeGetVolumeStats volume ID was empty")
	}
	if len(req.VolumePath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodeGetVolumeStats volume path was empty")
	}

	if _, err := os.Lstat(req.VolumePath); err != nil {
		if os.IsNotExist(err) {
			return nil, status.Errorf(codes.NotFound, "path %s does not exist", req.VolumePath)
		}
		return nil, status.Errorf(codes.Internal, "failed to stat file %s: %v", req.VolumePath, err)
	}

	volumeMetrics, err := volume.NewMetricsStatFS(req.VolumePath).GetMetrics()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get metrics: %v", err)
	}

	available, ok := volumeMetrics.Available.AsInt64()
	if !ok {
		return nil, status.Errorf(codes.Internal, "failed to transform volume available size(%v)", volumeMetrics.Available)
	}
	capacity, ok := volumeMetrics.Capacity.AsInt64()
	if !ok {
		return nil, status.Errorf(codes.Internal, "failed to transform volume capacity size(%v)", volumeMetrics.Capacity)
	}
	used, ok := volumeMetrics.Used.AsInt64()
	if !ok {
		return nil, status.Errorf(codes.Internal, "failed to transform volume used size(%v)", volumeMetrics.Used)
	}

	inodesFree, ok := volumeMetrics.InodesFree.AsInt64()
	if !ok {
		return nil, status.Errorf(codes.Internal, "failed to transform disk inodes free(%v)", volumeMetrics.InodesFree)
	}
	inodes, ok := volumeMetrics.Inodes.AsInt64()
	if !ok {
		return nil, status.Errorf(codes.Internal, "failed to transform disk inodes(%v)", volumeMetrics.Inodes)
	}
	inodesUsed, ok := volumeMetrics.InodesUsed.AsInt64()
	if !ok {
		return nil, status.Errorf(codes.Internal, "failed to transform disk inodes used(%v)", volumeMetrics.InodesUsed)
	}

	return &csi.NodeGetVolumeStatsResponse{
		Usage: []*csi.VolumeUsage{
			{
				Unit:      csi.VolumeUsage_BYTES,
				Available: available,
				Total:     capacity,
				Used:      used,
			},
			{
				Unit:      csi.VolumeUsage_INODES,
				Available: inodesFree,
				Total:     inodes,
				Used:      inodesUsed,
			},
		},
	}, nil
}

// ensureMountPoint: create mount point if not exists
// return <true, nil> if it's already a mounted point otherwise return <false, nil>
func (d *Driver) ensureMountPoint(target string, perm os.FileMode) (bool, error) {
	notMnt, err := d.mounter.IsLikelyNotMountPoint(target)
	if err != nil && !os.IsNotExist(err) {
		if IsCorruptedDir(target) {
			notMnt = false
			klog.Warningf("detected corrupted mount for targetPath [%s]", target)
		} else {
			return !notMnt, err
		}
	}

	// Check all the mountpoints in case IsLikelyNotMountPoint
	// cannot handle --bind mount
	mountList, err := d.mounter.List()
	if err != nil {
		return !notMnt, err
	}

	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return !notMnt, err
	}

	for _, mountPoint := range mountList {
		if mountPoint.Path == targetAbs {
			notMnt = false
			break
		}
	}

	if !notMnt {
		// testing original mount point, make sure the mount link is valid
		_, err := ioutil.ReadDir(target)
		if err == nil {
			klog.V(2).Infof("already mounted to target %s", target)
			return !notMnt, nil
		}
		// mount link is invalid, now unmount and remount later
		klog.Warningf("ReadDir %s failed with %v, unmount this directory", target, err)
		if err := d.mounter.Unmount(target); err != nil {
			klog.Errorf("Unmount directory %s failed with %v", target, err)
			return !notMnt, err
		}
		notMnt = true
		return !notMnt, err
	}
	if err := volumehelper.MakeDir(target, perm); err != nil {
		klog.Errorf("MakeDir failed on target: %s (%v)", target, err)
		return !notMnt, err
	}
	return !notMnt, nil
}

func waitForMount(path string, intervel, timeout time.Duration) error {
	timeAfter := time.After(timeout)
	timeTick := time.Tick(intervel)

	for {
		select {
		case <-timeTick:
			notMount, err := mount.New("").IsLikelyNotMountPoint(path)
			if err != nil {
				return err
			}
			if !notMount {
				klog.V(2).Infof("blobfuse mount at %s success", path)
				return nil
			}
		case <-timeAfter:
			return fmt.Errorf("timeout waiting for mount %s", path)
		}
	}
}
