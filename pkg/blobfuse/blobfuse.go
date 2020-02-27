/*
Copyright 2019 The Kubernetes Authors.

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
	"strings"

	csicommon "sigs.k8s.io/blobfuse-csi-driver/pkg/csi-common"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/pborman/uuid"
	"golang.org/x/net/context"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/util/mount"
	k8sutil "k8s.io/kubernetes/pkg/volume/util"
	"k8s.io/legacy-cloud-providers/azure"
)

const (
	// DriverName holds the name of the csi-driver
	DriverName       = "blobfuse.csi.azure.com"
	separator        = "#"
	volumeIDTemplate = "%s#%s#%s"
	fileMode         = "file_mode"
	dirMode          = "dir_mode"
	vers             = "vers"
	defaultFileMode  = "0777"
	defaultDirMode   = "0777"
	defaultVers      = "3.0"

	// See https://docs.microsoft.com/en-us/rest/api/storageservices/naming-and-referencing-containers--blobs--and-metadata#container-names
	containerNameMinLength = 3
	containerNameMaxLength = 63
)

// Driver implements all interfaces of CSI drivers
type Driver struct {
	csicommon.CSIDriver
	cloud   *azure.Cloud
	mounter *mount.SafeFormatAndMount
}

// NewDriver Creates a NewCSIDriver object. Assumes vendor version is equal to driver version &
// does not support optional driver plugin info manifest field. Refer to CSI spec for more details.
func NewDriver(nodeID string) *Driver {
	driver := Driver{}
	driver.Name = DriverName
	driver.Version = driverVersion
	driver.NodeID = nodeID
	return &driver
}

// Run driver initialization
func (d *Driver) Run(endpoint string) {
	versionMeta, err := GetVersionYAML()
	if err != nil {
		klog.Fatalf("%v", err)
	}
	klog.Infof("\nDRIVER INFORMATION:\n-------------------\n%s\n\nStreaming logs below:", versionMeta)

	cloud, err := GetCloudProvider()
	if err != nil {
		klog.Fatalln("failed to get Azure Cloud Provider")
	}
	d.cloud = cloud

	d.mounter = &mount.SafeFormatAndMount{
		Interface: mount.New(""),
		Exec:      mount.NewOsExec(),
	}

	// Initialize default library driver
	d.AddControllerServiceCapabilities(
		[]csi.ControllerServiceCapability_RPC_Type{
			csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
			//csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
			//csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS,
		})
	d.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY,
		csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
		csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER,
		csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
	})

	d.AddNodeServiceCapabilities([]csi.NodeServiceCapability_RPC_Type{
		csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
		csi.NodeServiceCapability_RPC_UNKNOWN,
	})

	s := csicommon.NewNonBlockingGRPCServer()
	// Driver d act as IdentityServer, ControllerServer and NodeServer
	s.Start(endpoint, d, d, d)
	s.Wait()
}

// get container info according to volume id, e.g.
// input: "rg#f5713de20cde511e8ba4900#pvc-fuse-dynamic-17e43f84-f474-11e8-acd0-000d3a00df41"
// output: rg, f5713de20cde511e8ba4900, pvc-fuse-dynamic-17e43f84-f474-11e8-acd0-000d3a00df41
func GetContainerInfo(id string) (string, string, string, error) {
	segments := strings.Split(id, separator)
	if len(segments) < 3 {
		return "", "", "", fmt.Errorf("error parsing volume id: %q, should at least contain two #", id)
	}
	return segments[0], segments[1], segments[2], nil
}

// check whether mountOptions contains file_mode, dir_mode, vers, if not, append default mode
func appendDefaultMountOptions(mountOptions []string) []string {
	fileModeFlag := false
	dirModeFlag := false
	versFlag := false

	for _, mountOption := range mountOptions {
		if strings.HasPrefix(mountOption, fileMode) {
			fileModeFlag = true
		}
		if strings.HasPrefix(mountOption, dirMode) {
			dirModeFlag = true
		}
		if strings.HasPrefix(mountOption, vers) {
			versFlag = true
		}
	}

	allMountOptions := mountOptions
	if !fileModeFlag {
		allMountOptions = append(allMountOptions, fmt.Sprintf("%s=%s", fileMode, defaultFileMode))
	}

	if !dirModeFlag {
		allMountOptions = append(allMountOptions, fmt.Sprintf("%s=%s", dirMode, defaultDirMode))
	}

	if !versFlag {
		allMountOptions = append(allMountOptions, fmt.Sprintf("%s=%s", vers, defaultVers))
	}

	/* todo: looks like fsGroup is not included in CSI
	if !gidFlag && fsGroup != nil {
		allMountOptions = append(allMountOptions, fmt.Sprintf("%s=%d", gid, *fsGroup))
	}
	*/
	return allMountOptions
}

// get storage account from secrets map
// returns <accountName, accountKey, accountSasToken>
func getStorageAccountFromSecretsMap(secrets map[string]string) (string, string, string, error) {
	if secrets == nil {
		return "", "", "", fmt.Errorf("unexpected: getStorageAccountFromSecretsMap secrets is nil")
	}

	var accountName, accountKey, accountSasToken string
	for k, v := range secrets {
		switch strings.ToLower(k) {
		case "accountname":
			accountName = v
		case "azurestorageaccountname": // for compatibility with built-in blobfuse plugin
			accountName = v
		case "accountkey":
			accountKey = v
		case "azurestorageaccountkey": // for compatibility with built-in blobfuse plugin
			accountKey = v
		case "azurestorageaccountsastoken":
			accountSasToken = v
		}
	}

	if accountName == "" {
		return "", "", "", fmt.Errorf("could not find accountname or azurestorageaccountname field secrets(%v)", secrets)
	}
	if accountKey == "" && accountSasToken == "" {
		return "", "", "", fmt.Errorf("could not find accountkey, azurestorageaccountkey or azurestorageaccountsastoken field in secrets(%v)", secrets)
	}
	if accountKey != "" && accountSasToken != "" {
		return "", "", "", fmt.Errorf("could not specify Access Key and SAS Token together")
	}

	return accountName, accountKey, accountSasToken, nil
}

// A container name must be a valid DNS name, conforming to the following naming rules:
//	1. Container names must start with a letter or number, and can contain only letters, numbers, and the dash (-) character.
//	2. Every dash (-) character must be immediately preceded and followed by a letter or number; consecutive dashes are not permitted in container names.
//	3. All letters in a container name must be lowercase.
//	4. Container names must be from 3 through 63 characters long.
//
// See https://docs.microsoft.com/en-us/rest/api/storageservices/naming-and-referencing-containers--blobs--and-metadata#container-names
func getValidContainerName(volumeName string) string {
	containerName := strings.ToLower(volumeName)
	if len(containerName) > containerNameMaxLength {
		containerName = containerName[0:containerNameMaxLength]
	}
	if !checkContainerNameBeginAndEnd(containerName) || len(containerName) < containerNameMinLength {
		// now we set as 63 for maximum container name length
		// todo: get cluster name
		containerName = k8sutil.GenerateVolumeName("pvc-fuse", uuid.NewUUID().String(), 63)
		klog.Warningf("the requested volume name (%q) is invalid, so it is regenerated as (%q)", volumeName, containerName)
	}
	containerName = strings.Replace(containerName, "--", "-", -1)

	return containerName
}

func checkContainerNameBeginAndEnd(containerName string) bool {
	length := len(containerName)
	if (('a' <= containerName[0] && containerName[0] <= 'z') ||
		('0' <= containerName[0] && containerName[0] <= '9')) &&
		(('a' <= containerName[length-1] && containerName[length-1] <= 'z') ||
			('0' <= containerName[length-1] && containerName[length-1] <= '9')) {
		return true
	}

	return false
}

// isSASToken checks if the key contains the patterns. Because a SAS Token must have these strings, use them to judge.
func isSASToken(key string) bool {
	return strings.Contains(key, "?sv=")
}

// getStorageAccountAndContainer: get storage account and container info
// returns <accountName, accountKey, accountSasToken, containerName>
func (d *Driver) getStorageAccountAndContainer(ctx context.Context, volumeID string, attrib, secrets map[string]string) (string, string, string, string, error) {
	var (
		accountName     string
		accountKey      string
		accountSasToken string

		containerName string

		keyVaultURL           string
		keyVaultSecretName    string
		keyVaultSecretVersion string

		err error
	)

	for k, v := range attrib {
		switch strings.ToLower(k) {
		case "containername":
			containerName = v
		case "keyvaulturl":
			keyVaultURL = v
		case "keyvaultsecretname":
			keyVaultSecretName = v
		case "keyvaultsecretversion":
			keyVaultSecretVersion = v
		case "storageaccountname":
			accountName = v
		}
	}

	// 1. If keyVaultURL is not nil, preferentially use the key stored in key vault.
	// 2. Then if secrets map is not nil, use the key stored in the secrets map.
	// 3. Finally if both keyVaultURL and secrets map are nil, get the key from Azure.
	if keyVaultURL != "" {
		key, err := d.getKeyVaultSecretContent(ctx, keyVaultURL, keyVaultSecretName, keyVaultSecretVersion)
		if err != nil {
			return "", "", "", "", err
		}
		if isSASToken(key) {
			accountSasToken = key
		} else {
			accountKey = key
		}
	} else {
		if len(secrets) == 0 {
			var resourceGroupName string
			resourceGroupName, accountName, containerName, err = GetContainerInfo(volumeID)
			if err != nil {
				return "", "", "", "", err
			}

			if resourceGroupName == "" {
				resourceGroupName = d.cloud.ResourceGroup
			}

			accountKey, err = d.cloud.GetStorageAccesskey(accountName, resourceGroupName)
			if err != nil {
				return "", "", "", "", fmt.Errorf("no key for storage account(%s) under resource group(%s), err %v", accountName, resourceGroupName, err)
			}
		} else {
			accountName, accountKey, accountSasToken, err = getStorageAccountFromSecretsMap(secrets)
			if err != nil {
				return "", "", "", "", err
			}
		}
	}

	if containerName == "" {
		return "", "", "", "", fmt.Errorf("could not find containerName from attributes(%v) or volumeID(%v)", attrib, volumeID)
	}

	return accountName, accountKey, accountSasToken, containerName, nil
}

func IsCorruptedDir(dir string) bool {
	pathExists, pathErr := mount.PathExists(dir)
	fmt.Printf("IsCorruptedDir(%s) returned with error: (%v, %v)\\n", dir, pathExists, pathErr)
	return pathErr != nil && mount.IsCorruptedMnt(pathErr)
}
