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

package blob

import (
	"fmt"
	"strings"

	"golang.org/x/net/context"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/pborman/uuid"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	k8sutil "k8s.io/kubernetes/pkg/volume/util"
	utilexec "k8s.io/utils/exec"
	"k8s.io/utils/mount"

	csicommon "sigs.k8s.io/blob-csi-driver/pkg/csi-common"
	"sigs.k8s.io/blob-csi-driver/pkg/util"
	azure "sigs.k8s.io/cloud-provider-azure/pkg/provider"
)

const (
	// DriverName holds the name of the csi-driver
	DriverName                 = "blob.csi.azure.com"
	separator                  = "#"
	volumeIDTemplate           = "%s#%s#%s"
	secretNameTemplate         = "azure-storage-account-%s-secret"
	fileMode                   = "file_mode"
	dirMode                    = "dir_mode"
	vers                       = "vers"
	defaultFileMode            = "0777"
	defaultDirMode             = "0777"
	defaultVers                = "3.0"
	serverNameField            = "server"
	tagsField                  = "tags"
	protocolField              = "protocol"
	accountNameField           = "accountname"
	accountKeyField            = "accountkey"
	storageAccountField        = "storageaccount"
	storageAccountTypeField    = "storageaccounttype"
	skuNameField               = "skuname"
	resourceGroupField         = "resourcegroup"
	locationField              = "location"
	secretNamespaceField       = "secretnamespace"
	containerNameField         = "containername"
	storeAccountKeyField       = "storeaccountkey"
	keyVaultURLField           = "keyvaulturl"
	keyVaultSecretNameField    = "keyvaultsecretname"
	keyVaultSecretVersionField = "keyvaultsecretversion"
	storageAccountNameField    = "storageaccountname"
	storeAccountKeyFalse       = "false"
	defaultSecretAccountName   = "azurestorageaccountname"
	defaultSecretAccountKey    = "azurestorageaccountkey"
	defaultSecretNamespace     = "default"
	fuse                       = "fuse"
	nfs                        = "nfs"

	// See https://docs.microsoft.com/en-us/rest/api/storageservices/naming-and-referencing-containers--blobs--and-metadata#container-names
	containerNameMinLength = 3
	containerNameMaxLength = 63

	accountNotProvisioned = "StorageAccountIsNotProvisioned"
	tooManyRequests       = "TooManyRequests"
	shareNotFound         = "The specified share does not exist"
	shareBeingDeleted     = "The specified share is being deleted"
	clientThrottled       = "client throttled"

	// containerMaxSize is the max size of the blob container. See https://docs.microsoft.com/en-us/azure/storage/blobs/scalability-targets#scale-targets-for-blob-storage
	containerMaxSize = 100 * util.TiB
)

var (
	supportedProtocolList = []string{fuse, nfs}
	retriableErrors       = []string{accountNotProvisioned, tooManyRequests, shareNotFound, shareBeingDeleted, clientThrottled}
)

// Driver implements all interfaces of CSI drivers
type Driver struct {
	csicommon.CSIDriver
	cloud      *azure.Cloud
	mounter    *mount.SafeFormatAndMount
	volLockMap *util.LockMap
}

// NewDriver Creates a NewCSIDriver object. Assumes vendor version is equal to driver version &
// does not support optional driver plugin info manifest field. Refer to CSI spec for more details.
func NewDriver(nodeID string) *Driver {
	driver := Driver{}
	driver.Name = DriverName
	driver.Version = driverVersion
	driver.NodeID = nodeID
	driver.volLockMap = util.NewLockMap()
	return &driver
}

// Run driver initialization
func (d *Driver) Run(endpoint, kubeconfig string, testBool bool) {
	versionMeta, err := GetVersionYAML()
	if err != nil {
		klog.Fatalf("%v", err)
	}
	klog.Infof("\nDRIVER INFORMATION:\n-------------------\n%s\n\nStreaming logs below:", versionMeta)

	cloud, err := GetCloudProvider(kubeconfig)
	if err != nil || cloud.TenantID == "" || cloud.SubscriptionID == "" {
		klog.Fatalf("failed to get Azure Cloud Provider, error: %v", err)
	}
	d.cloud = cloud

	if d.NodeID == "" {
		// Disable UseInstanceMetadata for controller to mitigate a timeout issue using IMDS
		// https://github.com/kubernetes-sigs/azuredisk-csi-driver/issues/168
		klog.Infoln("disable UseInstanceMetadata for controller")
		d.cloud.Config.UseInstanceMetadata = false
	}

	d.mounter = &mount.SafeFormatAndMount{
		Interface: mount.New(""),
		Exec:      utilexec.New(),
	}

	// Initialize default library driver
	d.AddControllerServiceCapabilities(
		[]csi.ControllerServiceCapability_RPC_Type{
			csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
			//csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
			//csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS,
			csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
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
	s.Start(endpoint, d, d, d, testBool)
	s.Wait()
}

// GetContainerInfo get container info according to volume id, e.g.
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

// A container name must be a valid DNS name, conforming to the following naming rules:
//	1. Container names must start with a letter or number, and can contain only letters, numbers, and the dash (-) character.
//	2. Every dash (-) character must be immediately preceded and followed by a letter or number; consecutive dashes are not permitted in container names.
//	3. All letters in a container name must be lowercase.
//	4. Container names must be from 3 through 63 characters long.
//
// See https://docs.microsoft.com/en-us/rest/api/storageservices/naming-and-referencing-containers--blobs--and-metadata#container-names
func getValidContainerName(volumeName, protocol string) string {
	containerName := strings.ToLower(volumeName)
	if len(containerName) > containerNameMaxLength {
		containerName = containerName[0:containerNameMaxLength]
	}
	if !checkContainerNameBeginAndEnd(containerName) || len(containerName) < containerNameMinLength {
		// now we set as 63 for maximum container name length
		// todo: get cluster name
		containerName = k8sutil.GenerateVolumeName(fmt.Sprintf("pvc-%s", protocol), uuid.NewUUID().String(), 63)
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

// GetAuthEnv return <accountName, containerName, authEnv, error>
func (d *Driver) GetAuthEnv(ctx context.Context, volumeID, protocol string, attrib, secrets map[string]string) (string, string, []string, error) {
	var (
		accountName           string
		accountKey            string
		accountSasToken       string
		containerName         string
		keyVaultURL           string
		keyVaultSecretName    string
		keyVaultSecretVersion string
		authEnv               []string
		err                   error
	)

	for k, v := range attrib {
		switch strings.ToLower(k) {
		case containerNameField:
			containerName = v
		case keyVaultURLField:
			keyVaultURL = v
		case keyVaultSecretNameField:
			keyVaultSecretName = v
		case keyVaultSecretVersionField:
			keyVaultSecretVersion = v
		case storageAccountField:
			accountName = v
		case storageAccountNameField: // for compatibility
			accountName = v
		case "azurestorageauthtype":
			authEnv = append(authEnv, "AZURE_STORAGE_AUTH_TYPE="+v)
		case "azurestorageidentityclientid":
			authEnv = append(authEnv, "AZURE_STORAGE_IDENTITY_CLIENT_ID="+v)
		case "azurestorageidentityobjectid":
			authEnv = append(authEnv, "AZURE_STORAGE_IDENTITY_OBJECT_ID="+v)
		case "azurestorageidentityresourceid":
			authEnv = append(authEnv, "AZURE_STORAGE_IDENTITY_RESOURCE_ID="+v)
		case "msiendpoint":
			authEnv = append(authEnv, "MSI_ENDPOINT="+v)
		case "azurestoragespnclientid":
			authEnv = append(authEnv, "AZURE_STORAGE_SPN_CLIENT_ID="+v)
		case "azurestoragespntenantid":
			authEnv = append(authEnv, "AZURE_STORAGE_SPN_TENANT_ID="+v)
		case "azurestorageaadendpoint":
			authEnv = append(authEnv, "AZURE_STORAGE_AAD_ENDPOINT="+v)
		}
	}
	klog.V(2).Infof("volumeID(%s) authEnv: %s", volumeID, authEnv)

	if protocol == nfs {
		return accountName, containerName, authEnv, err
	}

	// 1. If keyVaultURL is not nil, preferentially use the key stored in key vault.
	// 2. Then if secrets map is not nil, use the key stored in the secrets map.
	// 3. Finally if both keyVaultURL and secrets map are nil, get the key from Azure.
	if keyVaultURL != "" {
		key, err := d.getKeyVaultSecretContent(ctx, keyVaultURL, keyVaultSecretName, keyVaultSecretVersion)
		if err != nil {
			return accountName, containerName, authEnv, err
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
				return accountName, containerName, authEnv, err
			}

			if resourceGroupName == "" {
				resourceGroupName = d.cloud.ResourceGroup
			}

			// read from k8s secret first
			accountKey, err = d.GetStorageAccesskeyFromSecret(accountName, attrib[secretNamespaceField])
			if err != nil {
				klog.V(2).Infof("could not get account(%s) key from secret, error: %v, use cluster identity to get account key instead", accountName, err)
				accountKey, err = d.cloud.GetStorageAccesskey(accountName, resourceGroupName)
				if err != nil {
					return accountName, containerName, authEnv, fmt.Errorf("no key for storage account(%s) under resource group(%s), err %v", accountName, resourceGroupName, err)
				}
			}
		} else {
			for k, v := range secrets {
				switch strings.ToLower(k) {
				case accountNameField:
					accountName = v
				case defaultSecretAccountName: // for compatibility with built-in blobfuse plugin
					accountName = v
				case accountKeyField:
					accountKey = v
				case defaultSecretAccountKey: // for compatibility with built-in blobfuse plugin
					accountKey = v
				case "azurestorageaccountsastoken":
					accountSasToken = v
				case "msisecret":
					authEnv = append(authEnv, "MSI_SECRET="+v)
				case "azurestoragespnclientsecret":
					authEnv = append(authEnv, "AZURE_STORAGE_SPN_CLIENT_SECRET="+v)
				}
			}
		}
	}

	if containerName == "" {
		err = fmt.Errorf("could not find containerName from attributes(%v) or volumeID(%v)", attrib, volumeID)
	}

	if accountSasToken != "" {
		authEnv = append(authEnv, "AZURE_STORAGE_SAS_TOKEN="+accountSasToken)
	}

	if accountKey != "" {
		authEnv = append(authEnv, "AZURE_STORAGE_ACCESS_KEY="+accountKey)
	}

	return accountName, containerName, authEnv, err
}

// GetStorageAccountAndContainer get storage account and container info
// returns <accountName, accountKey, accountSasToken, containerName>
// only for e2e testing
func (d *Driver) GetStorageAccountAndContainer(ctx context.Context, volumeID string, attrib, secrets map[string]string) (string, string, string, string, error) {
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
		case containerNameField:
			containerName = v
		case keyVaultURLField:
			keyVaultURL = v
		case keyVaultSecretNameField:
			keyVaultSecretName = v
		case keyVaultSecretVersionField:
			keyVaultSecretVersion = v
		case storageAccountField:
			accountName = v
		case storageAccountNameField: // for compatibility
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
		}
	}

	if containerName == "" {
		return "", "", "", "", fmt.Errorf("could not find containerName from attributes(%v) or volumeID(%v)", attrib, volumeID)
	}

	return accountName, accountKey, accountSasToken, containerName, nil
}

func IsCorruptedDir(dir string) bool {
	_, pathErr := mount.PathExists(dir)
	return pathErr != nil && mount.IsCorruptedMnt(pathErr)
}

func isRetriableError(err error) bool {
	if err != nil {
		for _, v := range retriableErrors {
			if strings.Contains(strings.ToLower(err.Error()), strings.ToLower(v)) {
				return true
			}
		}
	}
	return false
}

func isSupportedProtocol(protocol string) bool {
	if protocol == "" {
		return true
	}
	for _, v := range supportedProtocolList {
		if protocol == v {
			return true
		}
	}
	return false
}

// get storage account from secrets map
func getStorageAccount(secrets map[string]string) (string, string, error) {
	if secrets == nil {
		return "", "", fmt.Errorf("unexpected: getStorageAccount secrets is nil")
	}

	var accountName, accountKey string
	for k, v := range secrets {
		switch strings.ToLower(k) {
		case accountNameField:
			accountName = v
		case defaultSecretAccountName: // for compatibility with built-in azurefile plugin
			accountName = v
		case accountKeyField:
			accountKey = v
		case defaultSecretAccountKey: // for compatibility with built-in azurefile plugin
			accountKey = v
		}
	}

	if accountName == "" {
		return accountName, accountKey, fmt.Errorf("could not find %s or %s field secrets(%v)", accountNameField, defaultSecretAccountName, secrets)
	}
	if accountKey == "" {
		return accountName, accountKey, fmt.Errorf("could not find %s or %s field in secrets(%v)", accountKeyField, defaultSecretAccountKey, secrets)
	}

	accountName = strings.TrimSpace(accountName)
	klog.V(4).Infof("got storage account(%s) from secret", accountName)
	return accountName, accountKey, nil
}

func setAzureCredentials(kubeClient kubernetes.Interface, accountName, accountKey, secretNamespace string) (string, error) {
	if kubeClient == nil {
		klog.Warningf("could not create secret: kubeClient is nil")
		return "", nil
	}
	if accountName == "" || accountKey == "" {
		return "", fmt.Errorf("the account info is not enough, accountName(%v), accountKey(%v)", accountName, accountKey)
	}
	if secretNamespace == "" {
		secretNamespace = defaultSecretNamespace
	}
	secretName := fmt.Sprintf(secretNameTemplate, accountName)
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: defaultSecretNamespace,
			Name:      secretName,
		},
		Data: map[string][]byte{
			defaultSecretAccountName: []byte(accountName),
			defaultSecretAccountKey:  []byte(accountKey),
		},
		Type: "Opaque",
	}
	_, err := kubeClient.CoreV1().Secrets(secretNamespace).Create(context.TODO(), secret, metav1.CreateOptions{})
	if errors.IsAlreadyExists(err) {
		err = nil
	}
	if err != nil {
		return "", fmt.Errorf("couldn't create secret %v", err)
	}
	return secretName, err
}

// GetStorageAccesskey get Azure storage (account name, account key)
func (d *Driver) GetStorageAccesskey(accountOptions *azure.AccountOptions, secrets map[string]string, secretNamespace string) (string, string, error) {
	if len(secrets) > 0 {
		return getStorageAccount(secrets)
	}

	// read from k8s secret first
	accountKey, err := d.GetStorageAccesskeyFromSecret(accountOptions.Name, secretNamespace)
	if err != nil {
		klog.V(2).Infof("could not get account(%s) key from secret, error: %v, use cluster identity to get account key instead", accountOptions.Name, err)
		accountKey, err = d.cloud.GetStorageAccesskey(accountOptions.Name, accountOptions.ResourceGroup)
	}
	return accountOptions.Name, accountKey, err
}

// GetStorageAccesskeyFromSecret get storage account key from k8s secret
func (d *Driver) GetStorageAccesskeyFromSecret(accountName, secretNamespace string) (string, error) {
	if d.cloud.KubeClient == nil {
		return "", fmt.Errorf("could not get account(%s) key from secret: KubeClient is nil", accountName)
	}

	secretName := fmt.Sprintf(secretNameTemplate, accountName)
	if secretNamespace == "" {
		secretNamespace = defaultSecretNamespace
	}
	secret, err := d.cloud.KubeClient.CoreV1().Secrets(secretNamespace).Get(context.TODO(), secretName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("could not get secret(%v): %v", secretName, err)
	}

	return string(secret.Data[defaultSecretAccountKey][:]), nil
}
