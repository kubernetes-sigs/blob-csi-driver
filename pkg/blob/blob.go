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
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/storage/mgmt/2021-09-01/storage"
	azstorage "github.com/Azure/azure-sdk-for-go/storage"
	az "github.com/Azure/go-autorest/autorest/azure"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/pborman/uuid"
	"golang.org/x/net/context"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	k8sutil "k8s.io/kubernetes/pkg/volume/util"
	mount "k8s.io/mount-utils"
	utilexec "k8s.io/utils/exec"

	csicommon "sigs.k8s.io/blob-csi-driver/pkg/csi-common"
	"sigs.k8s.io/blob-csi-driver/pkg/util"
	azcache "sigs.k8s.io/cloud-provider-azure/pkg/cache"
	azure "sigs.k8s.io/cloud-provider-azure/pkg/provider"
)

const (
	// DefaultDriverName holds the name of the csi-driver
	DefaultDriverName            = "blob.csi.azure.com"
	blobCSIDriverName            = "blob_csi_driver"
	separator                    = "#"
	volumeIDTemplate             = "%s#%s#%s#%s#%s#%s"
	secretNameTemplate           = "azure-storage-account-%s-secret"
	serverNameField              = "server"
	storageEndpointSuffixField   = "storageendpointsuffix"
	tagsField                    = "tags"
	matchTagsField               = "matchtags"
	protocolField                = "protocol"
	accountNameField             = "accountname"
	accountKeyField              = "accountkey"
	storageAccountField          = "storageaccount"
	storageAccountTypeField      = "storageaccounttype"
	skuNameField                 = "skuname"
	subscriptionIDField          = "subscriptionid"
	resourceGroupField           = "resourcegroup"
	locationField                = "location"
	secretNameField              = "secretname"
	secretNamespaceField         = "secretnamespace"
	containerNameField           = "containername"
	containerNamePrefixField     = "containernameprefix"
	storeAccountKeyField         = "storeaccountkey"
	isHnsEnabledField            = "ishnsenabled"
	softDeleteBlobsField         = "softdeleteblobs"
	softDeleteContainersField    = "softdeletecontainers"
	enableBlobVersioningField    = "enableblobversioning"
	getAccountKeyFromSecretField = "getaccountkeyfromsecret"
	keyVaultURLField             = "keyvaulturl"
	keyVaultSecretNameField      = "keyvaultsecretname"
	keyVaultSecretVersionField   = "keyvaultsecretversion"
	storageAccountNameField      = "storageaccountname"
	allowBlobPublicAccessField   = "allowblobpublicaccess"
	requireInfraEncryptionField  = "requireinfraencryption"
	ephemeralField               = "csi.storage.k8s.io/ephemeral"
	podNamespaceField            = "csi.storage.k8s.io/pod.namespace"
	mountOptionsField            = "mountoptions"
	falseValue                   = "false"
	trueValue                    = "true"
	defaultSecretAccountName     = "azurestorageaccountname"
	defaultSecretAccountKey      = "azurestorageaccountkey"
	accountSasTokenField         = "azurestorageaccountsastoken"
	msiSecretField               = "msisecret"
	storageSPNClientSecretField  = "azurestoragespnclientsecret"
	Fuse                         = "fuse"
	Fuse2                        = "fuse2"
	NFS                          = "nfs"
	vnetResourceGroupField       = "vnetresourcegroup"
	vnetNameField                = "vnetname"
	subnetNameField              = "subnetname"
	accessTierField              = "accesstier"
	networkEndpointTypeField     = "networkendpointtype"
	mountPermissionsField        = "mountpermissions"
	useDataPlaneAPIField         = "usedataplaneapi"

	// See https://docs.microsoft.com/en-us/rest/api/storageservices/naming-and-referencing-containers--blobs--and-metadata#container-names
	containerNameMinLength = 3
	containerNameMaxLength = 63

	accountNotProvisioned                   = "StorageAccountIsNotProvisioned"
	tooManyRequests                         = "TooManyRequests"
	clientThrottled                         = "client throttled"
	containerBeingDeletedDataplaneAPIError  = "ContainerBeingDeleted"
	containerBeingDeletedManagementAPIError = "container is being deleted"
	statusCodeNotFound                      = "StatusCode=404"
	httpCodeNotFound                        = "HTTPStatusCode: 404"

	// containerMaxSize is the max size of the blob container. See https://docs.microsoft.com/en-us/azure/storage/blobs/scalability-targets#scale-targets-for-blob-storage
	containerMaxSize = 100 * util.TiB

	subnetTemplate = "/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/virtualNetworks/%s/subnets/%s"

	defaultNamespace = "default"

	pvcNameKey           = "csi.storage.k8s.io/pvc/name"
	pvcNamespaceKey      = "csi.storage.k8s.io/pvc/namespace"
	pvNameKey            = "csi.storage.k8s.io/pv/name"
	pvcNameMetadata      = "${pvc.metadata.name}"
	pvcNamespaceMetadata = "${pvc.metadata.namespace}"
	pvNameMetadata       = "${pv.metadata.name}"

	VolumeID = "volumeid"

	defaultStorageEndPointSuffix = "core.windows.net"
)

var (
	supportedProtocolList = []string{Fuse, Fuse2, NFS}
	retriableErrors       = []string{accountNotProvisioned, tooManyRequests, statusCodeNotFound, containerBeingDeletedDataplaneAPIError, containerBeingDeletedManagementAPIError, clientThrottled}
)

// DriverOptions defines driver parameters specified in driver deployment
type DriverOptions struct {
	NodeID                                 string
	DriverName                             string
	CloudConfigSecretName                  string
	CloudConfigSecretNamespace             string
	CustomUserAgent                        string
	UserAgentSuffix                        string
	BlobfuseProxyEndpoint                  string
	EnableBlobfuseProxy                    bool
	BlobfuseProxyConnTimout                int
	EnableBlobMockMount                    bool
	AllowEmptyCloudConfig                  bool
	AllowInlineVolumeKeyAccessWithIdentity bool
	EnableGetVolumeStats                   bool
	AppendTimeStampInCacheDir              bool
	AppendMountErrorHelpLink               bool
	MountPermissions                       uint64
	KubeAPIQPS                             float64
	KubeAPIBurst                           int
}

// Driver implements all interfaces of CSI drivers
type Driver struct {
	csicommon.CSIDriver

	cloud                      *azure.Cloud
	cloudConfigSecretName      string
	cloudConfigSecretNamespace string
	customUserAgent            string
	userAgentSuffix            string
	blobfuseProxyEndpoint      string
	// enableBlobMockMount is only for testing, DO NOT set as true in non-testing scenario
	enableBlobMockMount                    bool
	enableBlobfuseProxy                    bool
	allowEmptyCloudConfig                  bool
	enableGetVolumeStats                   bool
	allowInlineVolumeKeyAccessWithIdentity bool
	appendTimeStampInCacheDir              bool
	appendMountErrorHelpLink               bool
	blobfuseProxyConnTimout                int
	mountPermissions                       uint64
	kubeAPIQPS                             float64
	kubeAPIBurst                           int
	mounter                                *mount.SafeFormatAndMount
	volLockMap                             *util.LockMap
	// A map storing all volumes with ongoing operations so that additional operations
	// for that same volume (as defined by VolumeID) return an Aborted error
	volumeLocks *volumeLocks
	// only for nfs feature
	subnetLockMap *util.LockMap
	// a map storing all volumes created by this driver <volumeName, accountName>
	volMap sync.Map
	// a timed cache storing all volumeIDs and storage accounts that are using data plane API
	dataPlaneAPIVolCache *azcache.TimedCache
	// a timed cache storing account search history (solve account list throttling issue)
	accountSearchCache *azcache.TimedCache
}

// NewDriver Creates a NewCSIDriver object. Assumes vendor version is equal to driver version &
// does not support optional driver plugin info manifest field. Refer to CSI spec for more details.
func NewDriver(options *DriverOptions) *Driver {
	d := Driver{
		volLockMap:                             util.NewLockMap(),
		subnetLockMap:                          util.NewLockMap(),
		volumeLocks:                            newVolumeLocks(),
		cloudConfigSecretName:                  options.CloudConfigSecretName,
		cloudConfigSecretNamespace:             options.CloudConfigSecretNamespace,
		customUserAgent:                        options.CustomUserAgent,
		userAgentSuffix:                        options.UserAgentSuffix,
		blobfuseProxyEndpoint:                  options.BlobfuseProxyEndpoint,
		enableBlobfuseProxy:                    options.EnableBlobfuseProxy,
		allowInlineVolumeKeyAccessWithIdentity: options.AllowInlineVolumeKeyAccessWithIdentity,
		blobfuseProxyConnTimout:                options.BlobfuseProxyConnTimout,
		enableBlobMockMount:                    options.EnableBlobMockMount,
		allowEmptyCloudConfig:                  options.AllowEmptyCloudConfig,
		enableGetVolumeStats:                   options.EnableGetVolumeStats,
		appendMountErrorHelpLink:               options.AppendMountErrorHelpLink,
		mountPermissions:                       options.MountPermissions,
		kubeAPIQPS:                             options.KubeAPIQPS,
		kubeAPIBurst:                           options.KubeAPIBurst,
	}
	d.Name = options.DriverName
	d.Version = driverVersion
	d.NodeID = options.NodeID

	var err error
	getter := func(key string) (interface{}, error) { return nil, nil }
	if d.accountSearchCache, err = azcache.NewTimedcache(time.Minute, getter); err != nil {
		klog.Fatalf("%v", err)
	}
	if d.dataPlaneAPIVolCache, err = azcache.NewTimedcache(10*time.Minute, getter); err != nil {
		klog.Fatalf("%v", err)
	}
	return &d
}

// Run driver initialization
func (d *Driver) Run(endpoint, kubeconfig string, testBool bool) {
	versionMeta, err := GetVersionYAML(d.Name)
	if err != nil {
		klog.Fatalf("%v", err)
	}
	klog.Infof("\nDRIVER INFORMATION:\n-------------------\n%s\n\nStreaming logs below:", versionMeta)

	userAgent := GetUserAgent(d.Name, d.customUserAgent, d.userAgentSuffix)
	klog.V(2).Infof("driver userAgent: %s", userAgent)
	d.cloud, err = getCloudProvider(kubeconfig, d.NodeID, d.cloudConfigSecretName, d.cloudConfigSecretNamespace, userAgent, d.allowEmptyCloudConfig, d.kubeAPIQPS, d.kubeAPIBurst)
	if err != nil {
		klog.Fatalf("failed to get Azure Cloud Provider, error: %v", err)
	}
	klog.V(2).Infof("cloud: %s, location: %s, rg: %s, VnetName: %s, VnetResourceGroup: %s, SubnetName: %s", d.cloud.Cloud, d.cloud.Location, d.cloud.ResourceGroup, d.cloud.VnetName, d.cloud.VnetResourceGroup, d.cloud.SubnetName)

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
			csi.ControllerServiceCapability_RPC_SINGLE_NODE_MULTI_WRITER,
		})
	d.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY,
		csi.VolumeCapability_AccessMode_SINGLE_NODE_SINGLE_WRITER,
		csi.VolumeCapability_AccessMode_SINGLE_NODE_MULTI_WRITER,
		csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
		csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER,
		csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
	})

	nodeCap := []csi.NodeServiceCapability_RPC_Type{
		csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
		csi.NodeServiceCapability_RPC_SINGLE_NODE_MULTI_WRITER,
	}
	if d.enableGetVolumeStats {
		nodeCap = append(nodeCap, csi.NodeServiceCapability_RPC_GET_VOLUME_STATS)
	}
	d.AddNodeServiceCapabilities(nodeCap)

	s := csicommon.NewNonBlockingGRPCServer()
	// Driver d act as IdentityServer, ControllerServer and NodeServer
	s.Start(endpoint, d, d, d, testBool)
	s.Wait()
}

// GetContainerInfo get container info according to volume id
// the format of VolumeId is: rg#accountName#containerName#uuid#secretNamespace#subsID
//
// e.g.
// input: "rg#f5713de20cde511e8ba4900#containerName#uuid#"
// output: rg, f5713de20cde511e8ba4900, containerName, "" , ""
// input: "rg#f5713de20cde511e8ba4900#containerName#uuid#namespace#"
// output: rg, f5713de20cde511e8ba4900, containerName, namespace, ""
// input: "rg#f5713de20cde511e8ba4900#containerName#uuid#namespace#subsID"
// output: rg, f5713de20cde511e8ba4900, containerName, namespace, subsID
func GetContainerInfo(id string) (string, string, string, string, string, error) {
	segments := strings.Split(id, separator)
	if len(segments) < 3 {
		return "", "", "", "", "", fmt.Errorf("error parsing volume id: %q, should at least contain two #", id)
	}
	var secretNamespace, subsID string
	if len(segments) > 4 {
		secretNamespace = segments[4]
	}
	if len(segments) > 5 {
		subsID = segments[5]
	}
	return segments[0], segments[1], segments[2], secretNamespace, subsID, nil
}

// A container name must be a valid DNS name, conforming to the following naming rules:
//  1. Container names must start with a letter or number, and can contain only letters, numbers, and the dash (-) character.
//  2. Every dash (-) character must be immediately preceded and followed by a letter or number; consecutive dashes are not permitted in container names.
//  3. All letters in a container name must be lowercase.
//  4. Container names must be from 3 through 63 characters long.
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
		klog.Warningf("requested volume name (%s) is invalid, regenerated as (%q)", volumeName, containerName)
	}
	return strings.Replace(containerName, "--", "-", -1)
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

// isSASToken checks if the key contains the patterns.
// SAS token format could refer to https://docs.microsoft.com/en-us/rest/api/eventhub/generate-sas-token
func isSASToken(key string) bool {
	return strings.HasPrefix(key, "?")
}

// GetAuthEnv return <accountName, containerName, authEnv, error>
func (d *Driver) GetAuthEnv(ctx context.Context, volumeID, protocol string, attrib, secrets map[string]string) (string, string, string, string, []string, error) {
	rgName, accountName, containerName, secretNamespace, _, err := GetContainerInfo(volumeID)
	if err != nil {
		// ignore volumeID parsing error
		klog.V(2).Infof("parsing volumeID(%s) return with error: %v", volumeID, err)
		err = nil
	}

	var (
		subsID                  string
		accountKey              string
		accountSasToken         string
		msiSecret               string
		storageSPNClientSecret  string
		secretName              string
		pvcNamespace            string
		keyVaultURL             string
		keyVaultSecretName      string
		keyVaultSecretVersion   string
		azureStorageAuthType    string
		authEnv                 []string
		getAccountKeyFromSecret bool
	)

	for k, v := range attrib {
		switch strings.ToLower(k) {
		case subscriptionIDField:
			subsID = v
		case resourceGroupField:
			rgName = v
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
		case secretNameField:
			secretName = v
		case secretNamespaceField:
			secretNamespace = v
		case pvcNamespaceKey:
			pvcNamespace = v
		case getAccountKeyFromSecretField:
			getAccountKeyFromSecret = strings.EqualFold(v, trueValue)
		case "azurestorageauthtype":
			azureStorageAuthType = v
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

	if protocol == NFS {
		// nfs protocol does not need account key, return directly
		return rgName, accountName, accountKey, containerName, authEnv, err
	}

	if secretNamespace == "" {
		if pvcNamespace == "" {
			secretNamespace = defaultNamespace
		} else {
			secretNamespace = pvcNamespace
		}
	}

	if rgName == "" {
		rgName = d.cloud.ResourceGroup
	}

	// 1. If keyVaultURL is not nil, preferentially use the key stored in key vault.
	// 2. Then if secrets map is not nil, use the key stored in the secrets map.
	// 3. Finally if both keyVaultURL and secrets map are nil, get the key from Azure.
	if keyVaultURL != "" {
		key, err := d.getKeyVaultSecretContent(ctx, keyVaultURL, keyVaultSecretName, keyVaultSecretVersion)
		if err != nil {
			return rgName, accountName, accountKey, containerName, authEnv, err
		}
		if isSASToken(key) {
			accountSasToken = key
		} else {
			accountKey = key
		}
	} else {
		if len(secrets) == 0 {
			if secretName == "" && accountName != "" {
				secretName = fmt.Sprintf(secretNameTemplate, accountName)
			}
			if secretName != "" {
				// read from k8s secret first
				var name string
				name, accountKey, accountSasToken, msiSecret, storageSPNClientSecret, err = d.GetInfoFromSecret(ctx, secretName, secretNamespace)
				if name != "" {
					accountName = name
				}
				if err != nil && !getAccountKeyFromSecret && (azureStorageAuthType == "" || strings.EqualFold(azureStorageAuthType, "key")) {
					klog.V(2).Infof("get account(%s) key from secret(%s, %s) failed with error: %v, use cluster identity to get account key instead",
						accountName, secretNamespace, secretName, err)
					accountKey, err = d.cloud.GetStorageAccesskey(ctx, subsID, accountName, rgName)
					if err != nil {
						return rgName, accountName, accountKey, containerName, authEnv, fmt.Errorf("no key for storage account(%s) under resource group(%s), err %w", accountName, rgName, err)
					}
				}
			}
		} else {
			for k, v := range secrets {
				v = strings.TrimSpace(v)
				switch strings.ToLower(k) {
				case accountNameField:
					accountName = v
				case defaultSecretAccountName: // for compatibility with built-in blobfuse plugin
					accountName = v
				case accountKeyField:
					accountKey = v
				case defaultSecretAccountKey: // for compatibility with built-in blobfuse plugin
					accountKey = v
				case accountSasTokenField:
					accountSasToken = v
				case msiSecretField:
					msiSecret = v
				case storageSPNClientSecretField:
					storageSPNClientSecret = v
				}
			}
		}
	}

	if containerName == "" {
		err = fmt.Errorf("could not find containerName from attributes(%v) or volumeID(%v)", attrib, volumeID)
	}

	if accountKey != "" {
		authEnv = append(authEnv, "AZURE_STORAGE_ACCESS_KEY="+accountKey)
	}

	if accountSasToken != "" {
		klog.V(2).Infof("accountSasToken is not empty, use it to access storage account(%s), container(%s)", accountName, containerName)
		authEnv = append(authEnv, "AZURE_STORAGE_SAS_TOKEN="+accountSasToken)
	}

	if msiSecret != "" {
		klog.V(2).Infof("msiSecret is not empty, use it to access storage account(%s), container(%s)", accountName, containerName)
		authEnv = append(authEnv, "MSI_SECRET="+msiSecret)
	}

	if storageSPNClientSecret != "" {
		klog.V(2).Infof("storageSPNClientSecret is not empty, use it to access storage account(%s), container(%s)", accountName, containerName)
		authEnv = append(authEnv, "AZURE_STORAGE_SPN_CLIENT_SECRET="+storageSPNClientSecret)
	}

	return rgName, accountName, accountKey, containerName, authEnv, err
}

// GetStorageAccountAndContainer get storage account and container info
// returns <accountName, accountKey, accountSasToken, containerName>
// only for e2e testing
func (d *Driver) GetStorageAccountAndContainer(ctx context.Context, volumeID string, attrib, secrets map[string]string) (string, string, string, string, error) {
	var (
		subsID                string
		accountName           string
		accountKey            string
		accountSasToken       string
		containerName         string
		keyVaultURL           string
		keyVaultSecretName    string
		keyVaultSecretVersion string
		err                   error
	)

	for k, v := range attrib {
		switch strings.ToLower(k) {
		case subscriptionIDField:
			subsID = v
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
			var rgName string
			rgName, accountName, containerName, _, _, err = GetContainerInfo(volumeID)
			if err != nil {
				return "", "", "", "", err
			}

			if rgName == "" {
				rgName = d.cloud.ResourceGroup
			}

			accountKey, err = d.cloud.GetStorageAccesskey(ctx, subsID, accountName, rgName)
			if err != nil {
				return "", "", "", "", fmt.Errorf("no key for storage account(%s) under resource group(%s), err %w", accountName, rgName, err)
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

func isSupportedAccessTier(accessTier string) bool {
	if accessTier == "" {
		return true
	}
	for _, tier := range storage.PossibleAccessTierValues() {
		if accessTier == string(tier) {
			return true
		}
	}
	return false
}

// container names can contain only lowercase letters, numbers, and hyphens,
// and must begin and end with a letter or a number
func isSupportedContainerNamePrefix(prefix string) bool {
	if prefix == "" {
		return true
	}
	if len(prefix) > 20 {
		return false
	}
	if prefix[0] == '-' {
		return false
	}
	for _, v := range prefix {
		if v != '-' && (v < '0' || v > '9') && (v < 'a' || v > 'z') {
			return false
		}
	}
	return true
}

// get storage account from secrets map
func getStorageAccount(secrets map[string]string) (string, string, error) {
	if secrets == nil {
		return "", "", fmt.Errorf("unexpected: getStorageAccount secrets is nil")
	}

	var accountName, accountKey string
	for k, v := range secrets {
		v = strings.TrimSpace(v)
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

func getContainerReference(containerName string, secrets map[string]string, env az.Environment) (*azstorage.Container, error) {
	accountName, accountKey, rerr := getStorageAccount(secrets)
	if rerr != nil {
		return nil, rerr
	}
	client, err := azstorage.NewBasicClientOnSovereignCloud(accountName, accountKey, env)
	if err != nil {
		return nil, err
	}
	blobClient := client.GetBlobService()
	container := blobClient.GetContainerReference(containerName)
	if container == nil {
		return nil, fmt.Errorf("ContainerReference of %s is nil", containerName)
	}
	return container, nil
}

func setAzureCredentials(ctx context.Context, kubeClient kubernetes.Interface, accountName, accountKey, secretNamespace string) (string, error) {
	if kubeClient == nil {
		klog.Warningf("could not create secret: kubeClient is nil")
		return "", nil
	}
	if accountName == "" || accountKey == "" {
		return "", fmt.Errorf("the account info is not enough, accountName(%v), accountKey(%v)", accountName, accountKey)
	}
	secretName := fmt.Sprintf(secretNameTemplate, accountName)
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: secretNamespace,
			Name:      secretName,
		},
		Data: map[string][]byte{
			defaultSecretAccountName: []byte(accountName),
			defaultSecretAccountKey:  []byte(accountKey),
		},
		Type: "Opaque",
	}
	_, err := kubeClient.CoreV1().Secrets(secretNamespace).Create(ctx, secret, metav1.CreateOptions{})
	if errors.IsAlreadyExists(err) {
		err = nil
	}
	if err != nil {
		return "", fmt.Errorf("couldn't create secret %w", err)
	}
	return secretName, err
}

// GetStorageAccesskey get Azure storage account key from
//  1. secrets (if not empty)
//  2. use k8s client identity to read from k8s secret
//  3. use cluster identity to get from storage account directly
func (d *Driver) GetStorageAccesskey(ctx context.Context, accountOptions *azure.AccountOptions, secrets map[string]string, secretName, secretNamespace string) (string, string, error) {
	if len(secrets) > 0 {
		return getStorageAccount(secrets)
	}

	// read from k8s secret first
	if secretName == "" {
		secretName = fmt.Sprintf(secretNameTemplate, accountOptions.Name)
	}
	_, accountKey, _, _, _, err := d.GetInfoFromSecret(ctx, secretName, secretNamespace) //nolint
	if err != nil {
		klog.V(2).Infof("could not get account(%s) key from secret(%s) namespace(%s), error: %v, use cluster identity to get account key instead", accountOptions.Name, secretName, secretNamespace, err)
		accountKey, err = d.cloud.GetStorageAccesskey(ctx, accountOptions.SubscriptionID, accountOptions.Name, accountOptions.ResourceGroup)
	}
	return accountOptions.Name, accountKey, err
}

// GetInfoFromSecret get info from k8s secret
// return <accountName, accountKey, accountSasToken, msiSecret, spnClientSecret, error>
func (d *Driver) GetInfoFromSecret(ctx context.Context, secretName, secretNamespace string) (string, string, string, string, string, error) {
	if d.cloud.KubeClient == nil {
		return "", "", "", "", "", fmt.Errorf("could not get account key from secret(%s): KubeClient is nil", secretName)
	}

	secret, err := d.cloud.KubeClient.CoreV1().Secrets(secretNamespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return "", "", "", "", "", fmt.Errorf("could not get secret(%v): %w", secretName, err)
	}

	accountName := strings.TrimSpace(string(secret.Data[defaultSecretAccountName][:]))
	accountKey := strings.TrimSpace(string(secret.Data[defaultSecretAccountKey][:]))
	accountSasToken := strings.TrimSpace(string(secret.Data[accountSasTokenField][:]))
	msiSecret := strings.TrimSpace(string(secret.Data[msiSecretField][:]))
	spnClientSecret := strings.TrimSpace(string(secret.Data[storageSPNClientSecretField][:]))

	klog.V(4).Infof("got storage account(%s) from secret", accountName)
	return accountName, accountKey, accountSasToken, msiSecret, spnClientSecret, nil
}

// getSubnetResourceID get default subnet resource ID from cloud provider config
func (d *Driver) getSubnetResourceID(vnetResourceGroup, vnetName, subnetName string) string {
	subsID := d.cloud.SubscriptionID
	if len(d.cloud.NetworkResourceSubscriptionID) > 0 {
		subsID = d.cloud.NetworkResourceSubscriptionID
	}

	if len(vnetResourceGroup) == 0 {
		vnetResourceGroup = d.cloud.ResourceGroup
		if len(d.cloud.VnetResourceGroup) > 0 {
			vnetResourceGroup = d.cloud.VnetResourceGroup
		}
	}

	if len(vnetName) == 0 {
		vnetName = d.cloud.VnetName
	}

	if len(subnetName) == 0 {
		subnetName = d.cloud.SubnetName
	}
	return fmt.Sprintf(subnetTemplate, subsID, vnetResourceGroup, vnetName, subnetName)
}

func (d *Driver) useDataPlaneAPI(volumeID, accountName string) bool {
	cache, err := d.dataPlaneAPIVolCache.Get(volumeID, azcache.CacheReadTypeDefault)
	if err != nil {
		klog.Errorf("get(%s) from dataPlaneAPIVolCache failed with error: %v", volumeID, err)
	}
	if cache != nil {
		return true
	}
	cache, err = d.dataPlaneAPIVolCache.Get(accountName, azcache.CacheReadTypeDefault)
	if err != nil {
		klog.Errorf("get(%s) from dataPlaneAPIVolCache failed with error: %v", accountName, err)
	}
	if cache != nil {
		return true
	}
	return false
}

// appendDefaultMountOptions return mount options combined with mountOptions and defaultMountOptions
func appendDefaultMountOptions(mountOptions []string, tmpPath, containerName string) []string {
	var defaultMountOptions = map[string]string{
		"--pre-mount-validate": "true",
		"--use-https":          "true",
		"--tmp-path":           tmpPath,
		"--container-name":     containerName,
		// prevent billing charges on mounting
		"--cancel-list-on-mount-seconds": "10",
		// allow remounting using a non-empty tmp-path
		"--empty-dir-check": "false",
	}

	// stores the mount options already included in mountOptions
	included := make(map[string]bool)

	for _, mountOption := range mountOptions {
		for k := range defaultMountOptions {
			if strings.HasPrefix(mountOption, k) {
				included[k] = true
			}
		}
	}

	allMountOptions := mountOptions

	for k, v := range defaultMountOptions {
		if _, isIncluded := included[k]; !isIncluded {
			if v != "" {
				allMountOptions = append(allMountOptions, fmt.Sprintf("%s=%s", k, v))
			} else {
				allMountOptions = append(allMountOptions, k)
			}
		}
	}

	return allMountOptions
}

// chmodIfPermissionMismatch only perform chmod when permission mismatches
func chmodIfPermissionMismatch(targetPath string, mode os.FileMode) error {
	info, err := os.Lstat(targetPath)
	if err != nil {
		return err
	}
	perm := info.Mode() & os.ModePerm
	if perm != mode {
		klog.V(2).Infof("chmod targetPath(%s, mode:0%o) with permissions(0%o)", targetPath, info.Mode(), mode)
		if err := os.Chmod(targetPath, mode); err != nil {
			return err
		}
	} else {
		klog.V(2).Infof("skip chmod on targetPath(%s) since mode is already 0%o)", targetPath, info.Mode())
	}
	return nil
}

func createStorageAccountSecret(account, key string) map[string]string {
	secret := make(map[string]string)
	secret[defaultSecretAccountName] = account
	secret[defaultSecretAccountKey] = key
	return secret
}

// setKeyValueInMap set key/value pair in map
// key in the map is case insensitive, if key already exists, overwrite existing value
func setKeyValueInMap(m map[string]string, key, value string) {
	if m == nil {
		return
	}
	for k := range m {
		if strings.EqualFold(k, key) {
			m[k] = value
			return
		}
	}
	m[key] = value
}

// replaceWithMap replace key with value for str
func replaceWithMap(str string, m map[string]string) string {
	for k, v := range m {
		if k != "" {
			str = strings.ReplaceAll(str, k, v)
		}
	}
	return str
}
