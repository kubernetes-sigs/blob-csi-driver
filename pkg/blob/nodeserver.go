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
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	volumehelper "sigs.k8s.io/blob-csi-driver/pkg/util"
	azcache "sigs.k8s.io/cloud-provider-azure/pkg/cache"

	"github.com/container-storage-interface/spec/lib/go/csi"

	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/volume"
	"k8s.io/kubernetes/pkg/volume/util"
	mount "k8s.io/mount-utils"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	mount_azure_blob "sigs.k8s.io/blob-csi-driver/pkg/blobfuse-proxy/pb"
	csiMetrics "sigs.k8s.io/blob-csi-driver/pkg/metrics"
)

const (
	waitForMountInterval = 20 * time.Millisecond
	waitForMountTimeout  = 110 * time.Second
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
		// token request
		if context[serviceAccountTokenField] != "" && useWorkloadIdentity(context) {
			klog.V(2).Infof("NodePublishVolume: volume(%s) mount on %s with service account token, clientID: %s", volumeID, target, getValueInMap(context, clientIDField))
			_, err := d.NodeStageVolume(ctx, &csi.NodeStageVolumeRequest{
				StagingTargetPath: target,
				VolumeContext:     context,
				VolumeCapability:  volCap,
				VolumeId:          volumeID,
			})
			return &csi.NodePublishVolumeResponse{}, err
		}

		// ephemeral volume
		if strings.EqualFold(context[ephemeralField], trueValue) {
			setKeyValueInMap(context, secretNamespaceField, context[podNamespaceField])
			if !d.allowInlineVolumeKeyAccessWithIdentity {
				// only get storage account from secret
				setKeyValueInMap(context, getAccountKeyFromSecretField, trueValue)
				setKeyValueInMap(context, storageAccountField, "")
			}
			klog.V(2).Infof("NodePublishVolume: ephemeral volume(%s) mount on %s", volumeID, target)
			_, err := d.NodeStageVolume(ctx, &csi.NodeStageVolumeRequest{
				StagingTargetPath: target,
				VolumeContext:     context,
				VolumeCapability:  volCap,
				VolumeId:          volumeID,
			})
			return &csi.NodePublishVolumeResponse{}, err
		}

		if perm := getValueInMap(context, mountPermissionsField); perm != "" {
			var err error
			if mountPermissions, err = strconv.ParseUint(perm, 8, 32); err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "invalid mountPermissions %s", perm)
			}
		}
	}

	source := req.GetStagingTargetPath()
	if len(source) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Staging target not provided")
	}

	mc := csiMetrics.NewCSIMetricContext("node_publish_volume")
	isOperationSucceeded := false
	defer func() {
		mc.Observe(isOperationSucceeded)
	}()

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
		isOperationSucceeded = true
		return &csi.NodePublishVolumeResponse{}, nil
	}

	klog.V(2).Infof("NodePublishVolume: volume %s mounting %s at %s with mountOptions: %v", volumeID, source, target, mountOptions)
	if d.enableBlobMockMount {
		klog.Warningf("NodePublishVolume: mock mount on volumeID(%s), this is only for TESTING!!!", volumeID)
		if err := volumehelper.MakeDir(target, os.FileMode(mountPermissions)); err != nil {
			klog.Errorf("MakeDir failed on target: %s (%v)", target, err)
			return nil, status.Errorf(codes.Internal, "%v", err)
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

	isOperationSucceeded = true
	return &csi.NodePublishVolumeResponse{}, nil
}

func (d *Driver) mountBlobfuseWithProxy(args, protocol string, authEnv []string) (string, error) {
	mc := csiMetrics.NewCSIMetricContext("node_blobfuse_proxy_mount")
	isOperationSucceeded := false
	defer func() {
		mc.ObserveWithLabels(isOperationSucceeded, Protocol, protocol)
	}()

	connectionTimeout := time.Duration(d.blobfuseProxyConnTimeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()
	klog.V(2).Infof("start connecting to blobfuse proxy, protocol: %s, args: %s", protocol, args)
	conn, err := grpc.DialContext(ctx, d.blobfuseProxyEndpoint, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		klog.Errorf("failed to connect to blobfuse proxy: %v", err)
		return "", err
	}
	defer func() {
		if err := conn.Close(); err != nil {
			klog.Errorf("failed to close connection to blobfuse proxy: %v", err)
		}
	}()

	mountClient := NewMountClient(conn)
	mountreq := mount_azure_blob.MountAzureBlobRequest{
		MountArgs: args,
		Protocol:  protocol,
		AuthEnv:   authEnv,
	}
	klog.V(2).Infof("begin to mount with blobfuse proxy, protocol: %s, args: %s", protocol, args)
	resp, err := mountClient.service.MountAzureBlob(context.TODO(), &mountreq)
	if err != nil {
		klog.Error("GRPC call returned with an error:", err)
	}
	var output string
	if resp != nil {
		output = resp.GetOutput()
	}
	klog.V(2).Infof("mount with blobfuse proxy completed, protocol: %s, args: %s, output: %s, error: %v", protocol, args, output, err)

	isOperationSucceeded = err == nil
	return output, err
}

func (d *Driver) mountBlobfuseInsideDriver(args string, protocol string, authEnv []string) (string, error) {
	mc := csiMetrics.NewCSIMetricContext("node_blobfuse_mount")
	isOperationSucceeded := false
	defer func() {
		mc.ObserveWithLabels(isOperationSucceeded, Protocol, protocol)
	}()

	var cmd *exec.Cmd

	args = volumehelper.TrimDuplicatedSpace(args)

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

	isOperationSucceeded = err == nil
	return string(output), err
}

// NodeUnpublishVolume unmount the volume from the target path
func (d *Driver) NodeUnpublishVolume(_ context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}
	targetPath := req.GetTargetPath()
	if len(targetPath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path missing in request")
	}

	mc := csiMetrics.NewCSIMetricContext("node_unpublish_volume")
	isOperationSucceeded := false
	defer func() {
		mc.Observe(isOperationSucceeded)
	}()

	klog.V(2).Infof("NodeUnpublishVolume: unmounting volume %s on %s", volumeID, targetPath)
	err := mount.CleanupMountPoint(targetPath, d.mounter, true /*extensiveMountPointCheck*/)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to unmount target %q: %v", targetPath, err)
	}
	klog.V(2).Infof("NodeUnpublishVolume: unmount volume %s on %s successfully", volumeID, targetPath)

	isOperationSucceeded = true
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

	lockKey := fmt.Sprintf("%s-%s", volumeID, targetPath)
	if acquired := d.volumeLocks.TryAcquire(lockKey); !acquired {
		return nil, status.Errorf(codes.Aborted, volumeOperationAlreadyExistsFmt, volumeID)
	}
	defer d.volumeLocks.Release(lockKey)

	mountFlags := req.GetVolumeCapability().GetMount().GetMountFlags()
	volumeMountGroup := req.GetVolumeCapability().GetMount().GetVolumeMountGroup()
	attrib := req.GetVolumeContext()
	secrets := req.GetSecrets()

	if useWorkloadIdentity(attrib) && attrib[serviceAccountTokenField] == "" {
		klog.V(2).Infof("Skip NodeStageVolume for volume(%s) since clientID %s is provided but service account token is empty", volumeID, getValueInMap(attrib, clientIDField))
		return &csi.NodeStageVolumeResponse{}, nil
	}

	var serverAddress, storageEndpointSuffix, protocol, ephemeralVolMountOptions, blobStorageAccountType string
	var ephemeralVol, isHnsEnabled bool

	containerNameReplaceMap := map[string]string{}

	fsGroupChangePolicy := d.fsGroupChangePolicy

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
					return nil, status.Errorf(codes.InvalidArgument, "invalid mountPermissions %s", v)
				}
				if perm == 0 {
					performChmodOp = false
				} else {
					mountPermissions = perm
				}
			}
		case fsGroupChangePolicyField:
			fsGroupChangePolicy = v
		case blobStorageAccountTypeField:
			blobStorageAccountType = v
		}
	}

	if !isSupportedFSGroupChangePolicy(fsGroupChangePolicy) {
		return nil, status.Errorf(codes.InvalidArgument, "fsGroupChangePolicy(%s) is not supported, supported fsGroupChangePolicy list: %v", fsGroupChangePolicy, supportedFSGroupChangePolicyList)
	}

	mc := csiMetrics.NewCSIMetricContext("node_stage_volume").WithBasicVolumeInfo(d.cloud.ResourceGroup, "", d.Name)
	isOperationSucceeded := false
	defer func() {
		mc.WithAdditionalVolumeInfo(VolumeID, volumeID)
		mc.ObserveWithLabels(isOperationSucceeded,
			"protocol", protocol,
			"storage_account_type", blobStorageAccountType,
			"is_hns_enabled", strconv.FormatBool(isHnsEnabled))
	}()
	mnt, err := d.ensureMountPoint(targetPath, fs.FileMode(mountPermissions))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not mount target %q: %v", targetPath, err)
	}

	_, accountName, _, containerName, authEnv, err := d.GetAuthEnv(ctx, volumeID, protocol, attrib, secrets)
	if err != nil && !mnt {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}

	if mnt {
		klog.V(2).Infof("NodeStageVolume: volume %s is already mounted on %s", volumeID, targetPath)
		isOperationSucceeded = true
		return &csi.NodeStageVolumeResponse{}, nil
	}

	// replace pv/pvc name namespace metadata in subDir
	containerName = replaceWithMap(containerName, containerNameReplaceMap)

	if strings.TrimSpace(storageEndpointSuffix) == "" {
		storageEndpointSuffix = d.getStorageEndPointSuffix()
	}

	if strings.TrimSpace(serverAddress) == "" {
		// server address is "accountname.blob.core.windows.net" by default
		serverAddress = fmt.Sprintf("%s.blob.%s", accountName, storageEndpointSuffix)
	}

	if isReadOnlyFromCapability(volumeCapability) {
		if isNFSProtocol(protocol) {
			mountFlags = util.JoinMountOptions(mountFlags, []string{"ro"})
		} else {
			mountFlags = util.JoinMountOptions(mountFlags, []string{"-o ro"})
		}
		klog.V(2).Infof("CSI volume is read-only, mounting with extra option ro")
	}

	if isNFSProtocol(protocol) {
		klog.V(2).Infof("target %v\nprotocol %v\n\nvolumeId %v\nmountflags %v\nserverAddress %v",
			targetPath, protocol, volumeID, mountFlags, serverAddress)

		mountType := AZNFS
		if !d.enableAznfsMount || protocol == NFSv3 {
			mountType = NFS
		}

		if storageEndpointSuffix != "" && mountType == AZNFS {
			aznfsEndpoint := strings.Replace(storageEndpointSuffix, "core.", "", 1)
			klog.V(2).Infof("set AZURE_ENDPOINT_OVERRIDE to %s", aznfsEndpoint)
			os.Setenv("AZURE_ENDPOINT_OVERRIDE", aznfsEndpoint)
		}

		source := fmt.Sprintf("%s:/%s/%s", serverAddress, accountName, containerName)
		mountOptions := util.JoinMountOptions(mountFlags, []string{"sec=sys,vers=3,nolock"})
		execFunc := func() error { return d.mounter.MountSensitive(source, targetPath, mountType, mountOptions, []string{}) }
		timeoutFunc := func() error { return fmt.Errorf("time out") }
		if err := volumehelper.WaitUntilTimeout(90*time.Second, execFunc, timeoutFunc); err != nil {
			var helpLinkMsg string
			if d.appendMountErrorHelpLink {
				helpLinkMsg = "\nPlease refer to http://aka.ms/blobmounterror for possible causes and solutions for mount errors."
			}
			return nil, status.Error(codes.Internal, fmt.Sprintf("volume(%s) mount %q on %q failed with %v%s", volumeID, source, targetPath, err, helpLinkMsg))
		}

		if performChmodOp {
			if !isReadOnlyFromCapability(volumeCapability) {
				if err := chmodIfPermissionMismatch(targetPath, os.FileMode(mountPermissions)); err != nil {
					return nil, status.Error(codes.Internal, err.Error())
				}
			} else {
				klog.V(2).Infof("skip chmod on targetPath(%s) since it's a read-only mount", targetPath)
			}
		} else {
			klog.V(2).Infof("skip chmod on targetPath(%s) since mountPermissions is set as 0", targetPath)
		}

		if volumeMountGroup != "" && fsGroupChangePolicy != FSGroupChangeNone {
			klog.V(2).Infof("set gid of volume(%s) as %s using fsGroupChangePolicy(%s)", volumeID, volumeMountGroup, fsGroupChangePolicy)
			if err := volumehelper.SetVolumeOwnership(targetPath, volumeMountGroup, fsGroupChangePolicy); err != nil {
				return nil, status.Error(codes.Internal, fmt.Sprintf("SetVolumeOwnership with volume(%s) on %s failed with %v", volumeID, targetPath, err))
			}
		}

		isOperationSucceeded = true
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

	if !checkGidPresentInMountFlags(mountFlags) && volumeMountGroup != "" {
		klog.V(2).Infof("append volumeMountGroup %s", volumeMountGroup)
		mountOptions = append(mountOptions, fmt.Sprintf("-o gid=%s", volumeMountGroup))
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

	klog.V(2).Infof("target %v protocol %v volumeId %v\nmountflags %v\nmountOptions %v volumeMountGroup %s\nargs %v\nserverAddress %v",
		targetPath, protocol, volumeID, mountFlags, mountOptions, volumeMountGroup, args, serverAddress)

	authEnv = append(authEnv, "AZURE_STORAGE_ACCOUNT="+accountName, "AZURE_STORAGE_BLOB_ENDPOINT="+serverAddress)
	if blobStorageAccountType != "" {
		klog.V(2).Infof("set AZURE_STORAGE_ACCOUNT_TYPE to %s", blobStorageAccountType)
		authEnv = append(authEnv, "AZURE_STORAGE_ACCOUNT_TYPE="+blobStorageAccountType)
	}
	if d.enableBlobMockMount {
		klog.Warningf("NodeStageVolume: mock mount on volumeID(%s), this is only for TESTING!!!", volumeID)
		if err := volumehelper.MakeDir(targetPath, os.FileMode(mountPermissions)); err != nil {
			klog.Errorf("MakeDir failed on target: %s (%v)", targetPath, err)
			return nil, status.Errorf(codes.Internal, "%v", err)
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
	isOperationSucceeded = true
	return &csi.NodeStageVolumeResponse{}, nil
}

// NodeUnstageVolume unmount the volume from the staging path
func (d *Driver) NodeUnstageVolume(_ context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	stagingTargetPath := req.GetStagingTargetPath()
	if len(stagingTargetPath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Staging target not provided")
	}

	lockKey := fmt.Sprintf("%s-%s", volumeID, stagingTargetPath)
	if acquired := d.volumeLocks.TryAcquire(lockKey); !acquired {
		return nil, status.Errorf(codes.Aborted, volumeOperationAlreadyExistsFmt, volumeID)
	}
	defer d.volumeLocks.Release(lockKey)

	mc := csiMetrics.NewCSIMetricContext("node_unstage_volume").WithBasicVolumeInfo(d.cloud.ResourceGroup, "", d.Name)
	isOperationSucceeded := false
	defer func() {
		mc.WithAdditionalVolumeInfo(VolumeID, volumeID)
		mc.Observe(isOperationSucceeded)
	}()

	klog.V(2).Infof("NodeUnstageVolume: volume %s unmounting on %s", volumeID, stagingTargetPath)
	err := mount.CleanupMountPoint(stagingTargetPath, d.mounter, true /*extensiveMountPointCheck*/)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to unmount staging target %q: %v", stagingTargetPath, err)
	}
	klog.V(2).Infof("NodeUnstageVolume: volume %s unmount on %s successfully", volumeID, stagingTargetPath)

	isOperationSucceeded = true
	return &csi.NodeUnstageVolumeResponse{}, nil
}

// NodeGetCapabilities return the capabilities of the Node plugin
func (d *Driver) NodeGetCapabilities(_ context.Context, _ *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: d.NSCap,
	}, nil
}

// NodeGetInfo return info of the node on which this plugin is running
func (d *Driver) NodeGetInfo(_ context.Context, _ *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	mc := csiMetrics.NewCSIMetricContext("node_get_info")
	defer mc.Observe(true)

	return &csi.NodeGetInfoResponse{
		NodeId: d.NodeID,
	}, nil
}

// NodeExpandVolume node expand volume
func (d *Driver) NodeExpandVolume(_ context.Context, _ *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
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

	// check if the volume stats is cached
	cache, err := d.volStatsCache.Get(ctx, req.VolumeId, azcache.CacheReadTypeDefault)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	if cache != nil {
		resp := cache.(*csi.NodeGetVolumeStatsResponse)
		klog.V(6).Infof("NodeGetVolumeStats: volume stats for volume %s path %s is cached", req.VolumeId, req.VolumePath)
		return resp, nil
	}

	mc := csiMetrics.NewCSIMetricContext("node_get_volume_stats").WithBasicVolumeInfo(d.cloud.ResourceGroup, "", d.Name)
	isOperationSucceeded := false
	defer func() {
		mc.WithAdditionalVolumeInfo(VolumeID, req.VolumeId)
		mc.Observe(isOperationSucceeded)
	}()

	if _, err := os.Lstat(req.VolumePath); err != nil {
		if os.IsNotExist(err) {
			return nil, status.Errorf(codes.NotFound, "path %s does not exist", req.VolumePath)
		}
		return nil, status.Errorf(codes.Internal, "failed to stat file %s: %v", req.VolumePath, err)
	}

	klog.V(6).Infof("NodeGetVolumeStats: begin to get VolumeStats on volume %s path %s", req.VolumeId, req.VolumePath)

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

	resp := &csi.NodeGetVolumeStatsResponse{
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
	}

	isOperationSucceeded = true
	klog.V(6).Infof("NodeGetVolumeStats: volume stats for volume %s path %s is %v", req.VolumeId, req.VolumePath, resp)
	// cache the volume stats per volume
	d.volStatsCache.Set(req.VolumeId, resp)
	return resp, nil
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
		// Use ReadDir(1) instead of full os.ReadDir to avoid expensive directory listing
		// on blobfuse mounts with many files. ReadDir(1) makes only one BlockBlob.List()
		// call (returning up to 5000 entries) regardless of directory size.
		f, err := os.Open(target)
		if err == nil {
			defer f.Close()
			_, err = f.ReadDir(1)
			// EOF means empty directory, which is valid
			if err == io.EOF {
				err = nil
			}
		}
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

func checkGidPresentInMountFlags(mountFlags []string) bool {
	for _, mountFlag := range mountFlags {
		if strings.Contains(mountFlag, "gid=") {
			return true
		}
	}
	return false
}

// useWorkloadIdentity checks whether workload identity is used based on the presence of clientID or mountWithWIToken in volume attributes
func useWorkloadIdentity(attrib map[string]string) bool {
	if getValueInMap(attrib, clientIDField) != "" || getValueInMap(attrib, mountWithWITokenField) == trueValue {
		return true
	}
	return false
}
