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
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	azstorage "github.com/Azure/azure-sdk-for-go/storage"
	"github.com/container-storage-interface/spec/lib/go/csi"
	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/pborman/uuid"
	"google.golang.org/grpc"
	v1 "k8s.io/api/core/v1"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	mount "k8s.io/mount-utils"
	utilexec "k8s.io/utils/exec"

	csicommon "sigs.k8s.io/blob-csi-driver/pkg/csi-common"
	"sigs.k8s.io/blob-csi-driver/pkg/util"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient"
	azcache "sigs.k8s.io/cloud-provider-azure/pkg/cache"
	"sigs.k8s.io/cloud-provider-azure/pkg/provider/storage"
)

const (
	// DefaultDriverName holds the name of the csi-driver
	DefaultDriverName              = "blob.csi.azure.com"
	blobCSIDriverName              = "blob_csi_driver"
	separator                      = "#"
	volumeIDTemplate               = "%s#%s#%s#%s#%s#%s"
	secretNameTemplate             = "azure-storage-account-%s-secret"
	serverNameField                = "server"
	storageEndpointSuffixField     = "storageendpointsuffix"
	tagsField                      = "tags"
	matchTagsField                 = "matchtags"
	protocolField                  = "protocol"
	accountNameField               = "accountname"
	accountKeyField                = "accountkey"
	storageAccountField            = "storageaccount"
	storageAccountTypeField        = "storageaccounttype"
	skuNameField                   = "skuname"
	subscriptionIDField            = "subscriptionid"
	resourceGroupField             = "resourcegroup"
	locationField                  = "location"
	secretNameField                = "secretname"
	secretNamespaceField           = "secretnamespace"
	containerNameField             = "containername"
	containerNamePrefixField       = "containernameprefix"
	storeAccountKeyField           = "storeaccountkey"
	getLatestAccountKeyField       = "getlatestaccountkey"
	isHnsEnabledField              = "ishnsenabled"
	softDeleteBlobsField           = "softdeleteblobs"
	softDeleteContainersField      = "softdeletecontainers"
	enableBlobVersioningField      = "enableblobversioning"
	getAccountKeyFromSecretField   = "getaccountkeyfromsecret"
	storageSPNClientIDField        = "azurestoragespnclientid"
	storageSPNTenantIDField        = "azurestoragespntenantid"
	storageAuthTypeField           = "azurestorageauthtype"
	blobStorageAccountTypeField    = "blobstorageaccounttype"
	storageAuthTypeMSI             = "msi"
	storageIdentityClientIDField   = "azurestorageidentityclientid"
	storageIdentityObjectIDField   = "azurestorageidentityobjectid"
	storageIdentityResourceIDField = "azurestorageidentityresourceid"
	msiEndpointField               = "msiendpoint"
	storageAADEndpointField        = "azurestorageaadendpoint"
	keyVaultURLField               = "keyvaulturl"
	keyVaultSecretNameField        = "keyvaultsecretname"
	keyVaultSecretVersionField     = "keyvaultsecretversion"
	storageAccountNameField        = "storageaccountname"
	allowBlobPublicAccessField     = "allowblobpublicaccess"
	allowSharedKeyAccessField      = "allowsharedkeyaccess"
	publicNetworkAccessField       = "publicnetworkaccess"
	requireInfraEncryptionField    = "requireinfraencryption"
	ephemeralField                 = "csi.storage.k8s.io/ephemeral"
	podNamespaceField              = "csi.storage.k8s.io/pod.namespace"
	serviceAccountTokenField       = "csi.storage.k8s.io/serviceAccount.tokens"
	clientIDField                  = "clientid"
	mountWithWITokenField          = "mountwithworkloadidentitytoken"
	tenantIDField                  = "tenantid"
	mountOptionsField              = "mountoptions"
	falseValue                     = "false"
	trueValue                      = "true"
	defaultSecretAccountName       = "azurestorageaccountname"
	defaultSecretAccountKey        = "azurestorageaccountkey"
	accountSasTokenField           = "azurestorageaccountsastoken"
	msiSecretField                 = "msisecret"
	storageSPNClientSecretField    = "azurestoragespnclientsecret"
	Fuse                           = "fuse"
	Fuse2                          = "fuse2"
	NFS                            = "nfs"
	AZNFS                          = "aznfs"
	NFSv3                          = "nfsv3"
	vnetResourceGroupField         = "vnetresourcegroup"
	vnetNameField                  = "vnetname"
	vnetLinkNameField              = "vnetlinkname"
	subnetNameField                = "subnetname"
	accessTierField                = "accesstier"
	networkEndpointTypeField       = "networkendpointtype"
	mountPermissionsField          = "mountpermissions"
	fsGroupChangePolicyField       = "fsgroupchangepolicy"
	useDataPlaneAPIField           = "usedataplaneapi"

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
	Protocol = "protocol"

	defaultStorageEndPointSuffix = "core.windows.net"

	FSGroupChangeNone = "None"
	// define tag value delimiter and default is comma
	tagValueDelimiterField = "tagvaluedelimiter"

	DefaultTokenAudience = "api://AzureADTokenExchange" //nolint:gosec // G101 ignore this!
)

var (
	supportedProtocolList            = []string{Fuse, Fuse2, NFS, AZNFS}
	retriableErrors                  = []string{accountNotProvisioned, tooManyRequests, statusCodeNotFound, containerBeingDeletedDataplaneAPIError, containerBeingDeletedManagementAPIError, clientThrottled}
	supportedFSGroupChangePolicyList = []string{FSGroupChangeNone, string(v1.FSGroupChangeAlways), string(v1.FSGroupChangeOnRootMismatch)}

	// azcopyCloneVolumeOptions used in volume cloning between different storage account and --check-length to false because volume data may be in changing state, copy volume is not same as current source volume,
	// set --s2s-preserve-access-tier=false to avoid BlobAccessTierNotSupportedForAccountType error in azcopy
	azcopyCloneVolumeOptions  = []string{"--recursive", "--check-length=false", "--s2s-preserve-access-tier=false", "--log-level=ERROR"}
	defaultAzureOAuthTokenDir = "/var/lib/kubelet/plugins/" + DefaultDriverName

	subscriptionIDRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

// DriverOptions defines driver parameters specified in driver deployment
type DriverOptions struct {
	NodeID                                 string
	DriverName                             string
	BlobfuseProxyEndpoint                  string
	EnableBlobfuseProxy                    bool
	BlobfuseProxyConnTimeout               int
	EnableBlobMockMount                    bool
	AllowInlineVolumeKeyAccessWithIdentity bool
	EnableGetVolumeStats                   bool
	AppendTimeStampInCacheDir              bool
	AppendMountErrorHelpLink               bool
	MountPermissions                       uint64
	EnableAznfsMount                       bool
	VolStatsCacheExpireInMinutes           int
	SasTokenExpirationMinutes              int
	WaitForAzCopyTimeoutMinutes            int
	EnableVolumeMountGroup                 bool
	FSGroupChangePolicy                    string
}

func (option *DriverOptions) AddFlags() {
	flag.StringVar(&option.BlobfuseProxyEndpoint, "blobfuse-proxy-endpoint", "unix://tmp/blobfuse-proxy.sock", "blobfuse-proxy endpoint")
	flag.StringVar(&option.NodeID, "nodeid", "", "node id")
	flag.StringVar(&option.DriverName, "drivername", DefaultDriverName, "name of the driver")
	flag.BoolVar(&option.EnableBlobfuseProxy, "enable-blobfuse-proxy", false, "using blobfuse proxy for mounts")
	flag.IntVar(&option.BlobfuseProxyConnTimeout, "blobfuse-proxy-connect-timeout", 5, "blobfuse proxy connection timeout(seconds)")
	flag.BoolVar(&option.EnableBlobMockMount, "enable-blob-mock-mount", false, "enable mock mount(only for testing)")
	flag.BoolVar(&option.EnableGetVolumeStats, "enable-get-volume-stats", false, "allow GET_VOLUME_STATS on agent node")
	flag.BoolVar(&option.AppendTimeStampInCacheDir, "append-timestamp-cache-dir", false, "append timestamp into cache directory on agent node")
	flag.Uint64Var(&option.MountPermissions, "mount-permissions", 0777, "mounted folder permissions")
	flag.BoolVar(&option.AllowInlineVolumeKeyAccessWithIdentity, "allow-inline-volume-key-access-with-idenitity", false, "allow accessing storage account key using cluster identity for inline volume")
	flag.BoolVar(&option.AppendMountErrorHelpLink, "append-mount-error-help-link", true, "Whether to include a link for help with mount errors when a mount error occurs.")
	flag.BoolVar(&option.EnableAznfsMount, "enable-aznfs-mount", false, "replace nfs mount with aznfs mount")
	flag.IntVar(&option.VolStatsCacheExpireInMinutes, "vol-stats-cache-expire-in-minutes", 10, "The cache expire time in minutes for volume stats cache")
	flag.IntVar(&option.SasTokenExpirationMinutes, "sas-token-expiration-minutes", 1440, "sas token expiration minutes during volume cloning")
	flag.IntVar(&option.WaitForAzCopyTimeoutMinutes, "wait-for-azcopy-timeout-minutes", 18, "timeout in minutes for waiting for azcopy to finish")
	flag.BoolVar(&option.EnableVolumeMountGroup, "enable-volume-mount-group", true, "indicates whether enabling VOLUME_MOUNT_GROUP")
	flag.StringVar(&option.FSGroupChangePolicy, "fsgroup-change-policy", "", "indicates how the volume's ownership will be changed by the driver, OnRootMismatch is the default value")
}

// Driver implements all interfaces of CSI drivers
type Driver struct {
	csicommon.CSIDriver
	// Embed UnimplementedXXXServer to ensure the driver returns Unimplemented for any
	// new RPC methods that might be introduced in future versions of the spec.
	csi.UnimplementedControllerServer
	csi.UnimplementedIdentityServer
	csi.UnimplementedNodeServer

	cloud                 *storage.AccountRepo
	clientFactory         azclient.ClientFactory
	networkClientFactory  azclient.ClientFactory
	KubeClient            kubernetes.Interface
	blobfuseProxyEndpoint string
	// enableBlobMockMount is only for testing, DO NOT set as true in non-testing scenario
	enableBlobMockMount                    bool
	enableBlobfuseProxy                    bool
	enableGetVolumeStats                   bool
	allowInlineVolumeKeyAccessWithIdentity bool
	appendTimeStampInCacheDir              bool
	appendMountErrorHelpLink               bool
	blobfuseProxyConnTimeout               int
	mountPermissions                       uint64
	enableAznfsMount                       bool
	enableVolumeMountGroup                 bool
	fsGroupChangePolicy                    string
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
	dataPlaneAPIVolCache azcache.Resource
	// a timed cache storing account search history (solve account list throttling issue)
	accountSearchCache azcache.Resource
	// a timed cache storing volume stats <volumeID, volumeStats>
	volStatsCache azcache.Resource
	// a timed cache storing account which should use sastoken for azcopy based volume cloning
	azcopySasTokenCache azcache.Resource
	// a timed cache storing subnet operations
	subnetCache azcache.Resource
	// sas expiry time for azcopy in volume clone
	sasTokenExpirationMinutes int
	// timeout in minutes for waiting for azcopy to finish
	waitForAzCopyTimeoutMinutes int
	// azcopy for provide exec mock for ut
	azcopy *util.Azcopy

	// if azcopy has to trust the driver's supplying endpoint
	requiredAzCopyToTrust bool
}

// NewDriver Creates a NewCSIDriver object. Assumes vendor version is equal to driver version &
// does not support optional driver plugin info manifest field. Refer to CSI spec for more details.
func NewDriver(options *DriverOptions, kubeClient kubernetes.Interface, cloud *storage.AccountRepo) *Driver {
	d := Driver{
		volLockMap:                             util.NewLockMap(),
		subnetLockMap:                          util.NewLockMap(),
		volumeLocks:                            newVolumeLocks(),
		blobfuseProxyEndpoint:                  options.BlobfuseProxyEndpoint,
		enableBlobfuseProxy:                    options.EnableBlobfuseProxy,
		allowInlineVolumeKeyAccessWithIdentity: options.AllowInlineVolumeKeyAccessWithIdentity,
		blobfuseProxyConnTimeout:               options.BlobfuseProxyConnTimeout,
		enableBlobMockMount:                    options.EnableBlobMockMount,
		enableGetVolumeStats:                   options.EnableGetVolumeStats,
		enableVolumeMountGroup:                 options.EnableVolumeMountGroup,
		appendMountErrorHelpLink:               options.AppendMountErrorHelpLink,
		mountPermissions:                       options.MountPermissions,
		enableAznfsMount:                       options.EnableAznfsMount,
		sasTokenExpirationMinutes:              options.SasTokenExpirationMinutes,
		waitForAzCopyTimeoutMinutes:            options.WaitForAzCopyTimeoutMinutes,
		fsGroupChangePolicy:                    options.FSGroupChangePolicy,
		azcopy:                                 &util.Azcopy{ExecCmd: &util.ExecCommand{}},
		KubeClient:                             kubeClient,
		cloud:                                  cloud,
	}
	d.Name = options.DriverName
	d.Version = driverVersion
	d.NodeID = options.NodeID
	if d.cloud != nil {
		d.clientFactory = d.cloud.ComputeClientFactory
		d.networkClientFactory = d.cloud.NetworkClientFactory
		if d.networkClientFactory == nil {
			d.networkClientFactory = d.cloud.ComputeClientFactory
		}
	}

	var err error
	getter := func(_ context.Context, _ string) (interface{}, error) { return nil, nil }
	if d.accountSearchCache, err = azcache.NewTimedCache(time.Minute, getter, false); err != nil {
		klog.Fatalf("%v", err)
	}
	if d.dataPlaneAPIVolCache, err = azcache.NewTimedCache(24*30*time.Hour, getter, false); err != nil {
		klog.Fatalf("%v", err)
	}
	if d.azcopySasTokenCache, err = azcache.NewTimedCache(15*time.Minute, getter, false); err != nil {
		klog.Fatalf("%v", err)
	}

	if options.VolStatsCacheExpireInMinutes <= 0 {
		options.VolStatsCacheExpireInMinutes = 10 // default expire in 10 minutes
	}
	if d.volStatsCache, err = azcache.NewTimedCache(time.Duration(options.VolStatsCacheExpireInMinutes)*time.Minute, getter, false); err != nil {
		klog.Fatalf("%v", err)
	}
	if d.subnetCache, err = azcache.NewTimedCache(10*time.Minute, getter, false); err != nil {
		klog.Fatalf("%v", err)
	}

	requiredAzCopyToTrust := d.getStorageEndPointSuffix() != "" && !strings.Contains(azcopyTrustedSuffixesAAD, d.getStorageEndPointSuffix())
	if requiredAzCopyToTrust {
		klog.V(2).Infof("storage endpoint suffix %s is not in azcopy trusted suffixes, azcopy will trust it temporarily during volume clone and snapshot restore", d.getStorageEndPointSuffix())
	}
	d.requiredAzCopyToTrust = requiredAzCopyToTrust

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
			csi.ControllerServiceCapability_RPC_CLONE_VOLUME,
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
	if d.enableVolumeMountGroup {
		nodeCap = append(nodeCap, csi.NodeServiceCapability_RPC_VOLUME_MOUNT_GROUP)
	}
	d.AddNodeServiceCapabilities(nodeCap)

	return &d
}

// Run driver initialization
func (d *Driver) Run(ctx context.Context, endpoint string) error {
	versionMeta, err := GetVersionYAML(d.Name)
	if err != nil {
		klog.Fatalf("%v", err)
	}
	klog.Infof("\nDRIVER INFORMATION:\n-------------------\n%s\n\nStreaming logs below:", versionMeta)

	if d.enableBlobfuseProxy && d.blobfuseProxyEndpoint != "" {
		monitor := NewBlobfuseProxyMonitor(d.blobfuseProxyEndpoint)
		go monitor.Start(ctx)
	}

	opts := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(
			grpcprom.NewServerMetrics().UnaryServerInterceptor(),
			csicommon.LogGRPC,
		),
	}
	s := grpc.NewServer(opts...)
	csi.RegisterIdentityServer(s, d)
	csi.RegisterControllerServer(s, d)
	csi.RegisterNodeServer(s, d)

	go func() {
		//graceful shutdown
		<-ctx.Done()
		s.GracefulStop()
	}()
	// Driver d act as IdentityServer, ControllerServer and NodeServer
	listener, err := csicommon.Listen(ctx, endpoint)
	if err != nil {
		klog.Fatalf("failed to listen to endpoint, error: %v", err)
	}
	err = s.Serve(listener)
	if errors.Is(err, grpc.ErrServerStopped) {
		klog.Infof("gRPC server stopped serving")
		return nil
	}
	return err
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
	if len(segments) > 5 && segments[5] != "" {
		if isValidSubscriptionID(segments[5]) {
			subsID = segments[5]
		} else {
			klog.Warningf("the subscription ID %s parsed from volume ID %s is not valid, ignore it", segments[5], id)
		}
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
		containerName = generateVolumeName(fmt.Sprintf("pvc-%s", protocol), uuid.NewUUID().String(), 63)
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
	rgName, accountName, containerName, secretNamespace, subsID, err := GetContainerInfo(volumeID)
	if err != nil {
		// ignore volumeID parsing error
		klog.V(6).Infof("parsing volumeID(%s) return with error: %v", volumeID, err)
		err = nil
	}

	var (
		accountKey              string
		accountSasToken         string
		msiSecret               string
		storageSPNClientSecret  string
		storageSPNClientID      string
		storageSPNTenantID      string
		secretName              string
		pvcNamespace            string
		keyVaultURL             string
		keyVaultSecretName      string
		keyVaultSecretVersion   string
		azureStorageAuthType    string
		authEnv                 []string
		getAccountKeyFromSecret bool
		getLatestAccountKey     bool
		clientID                string
		mountWithWIToken        bool
		tenantID                string
		serviceAccountToken     string
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
		case storageAuthTypeField:
			azureStorageAuthType = v
			authEnv = append(authEnv, "AZURE_STORAGE_AUTH_TYPE="+v)
		case storageIdentityClientIDField:
			authEnv = append(authEnv, "AZURE_STORAGE_IDENTITY_CLIENT_ID="+v)
		case storageIdentityObjectIDField:
			authEnv = append(authEnv, "AZURE_STORAGE_IDENTITY_OBJECT_ID="+v)
		case storageIdentityResourceIDField:
			authEnv = append(authEnv, "AZURE_STORAGE_IDENTITY_RESOURCE_ID="+v)
		case msiEndpointField:
			authEnv = append(authEnv, "MSI_ENDPOINT="+v)
		case storageSPNClientIDField:
			storageSPNClientID = v
		case storageSPNTenantIDField:
			storageSPNTenantID = v
		case storageAADEndpointField:
			authEnv = append(authEnv, "AZURE_STORAGE_AAD_ENDPOINT="+v)
		case getLatestAccountKeyField:
			if getLatestAccountKey, err = strconv.ParseBool(v); err != nil {
				return rgName, accountName, accountKey, containerName, authEnv, fmt.Errorf("invalid %s: %s in volume context", getLatestAccountKeyField, v)
			}
		case clientIDField:
			clientID = v
		case mountWithWITokenField:
			if mountWithWIToken, err = strconv.ParseBool(v); err != nil {
				return rgName, accountName, accountKey, containerName, authEnv, fmt.Errorf("invalid %s: %s in volume context", mountWithWITokenField, v)
			}
		case tenantIDField:
			tenantID = v
		case strings.ToLower(serviceAccountTokenField):
			serviceAccountToken = v
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

	if tenantID == "" {
		tenantID = d.cloud.TenantID
	}

	if mountWithWIToken {
		if clientID == "" {
			clientID = d.cloud.Config.AzureAuthConfig.UserAssignedIdentityID
			if clientID == "" {
				return rgName, accountName, accountKey, containerName, authEnv, fmt.Errorf("mountWithWorkloadIdentityToken is true but clientID is not specified")
			}
		}
		klog.V(2).Infof("mountWithWorkloadIdentityToken is specified, use workload identity auth for mount, clientID: %s, tenantID: %s", clientID, tenantID)

		workloadIdentityToken, err := parseServiceAccountToken(serviceAccountToken)
		if err != nil {
			return rgName, accountName, accountKey, containerName, authEnv, err
		}
		tokenFileName := clientID + "-" + accountName
		if !isValidTokenFileName(tokenFileName) {
			return rgName, accountName, accountKey, containerName, authEnv, fmt.Errorf("the generated token file name %s is invalid", tokenFileName)
		}
		azureOAuthTokenFile := filepath.Join(defaultAzureOAuthTokenDir, tokenFileName)
		// check whether token value is the same as the one in the token file
		existingToken, readErr := os.ReadFile(azureOAuthTokenFile)
		if readErr == nil && string(existingToken) == workloadIdentityToken {
			klog.V(6).Infof("the existing workload identity token file %s is up-to-date, no need to rewrite", azureOAuthTokenFile)
		} else {
			// write the token to a file
			if err := os.WriteFile(azureOAuthTokenFile, []byte(workloadIdentityToken), 0600); err != nil {
				return rgName, accountName, accountKey, containerName, authEnv, fmt.Errorf("failed to write workload identity token file %s: %v", azureOAuthTokenFile, err)
			}
		}

		authEnv = append(authEnv, "AZURE_STORAGE_SPN_CLIENT_ID="+clientID)
		if tenantID != "" {
			authEnv = append(authEnv, "AZURE_STORAGE_SPN_TENANT_ID="+tenantID)
		}
		authEnv = append(authEnv, "AZURE_OAUTH_TOKEN_FILE="+azureOAuthTokenFile)
		klog.V(2).Infof("workload identity auth: %v", authEnv)
		return rgName, accountName, accountKey, containerName, authEnv, err
	}

	if clientID != "" {
		klog.V(2).Infof("clientID(%s) is specified, use service account token to get account key", clientID)
		if subsID == "" {
			subsID = d.cloud.SubscriptionID
		}
		accountKey, err := d.cloud.GetStorageAccesskeyFromServiceAccountToken(ctx, subsID, accountName, rgName, clientID, tenantID, serviceAccountToken)
		authEnv = append(authEnv, "AZURE_STORAGE_ACCESS_KEY="+accountKey)
		return rgName, accountName, accountKey, containerName, authEnv, err
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
				var name, spnClientID, spnTenantID string
				name, accountKey, accountSasToken, msiSecret, storageSPNClientSecret, spnClientID, spnTenantID, err = d.GetInfoFromSecret(ctx, secretName, secretNamespace)
				if name != "" {
					accountName = name
				}
				if spnClientID != "" {
					storageSPNClientID = spnClientID
				}
				if spnTenantID != "" {
					storageSPNTenantID = spnTenantID
				}
				if err != nil && strings.EqualFold(azureStorageAuthType, storageAuthTypeMSI) {
					klog.V(2).Infof("ignore error(%v) since secret is optional for auth type(%s)", err, azureStorageAuthType)
					err = nil
				}
				if err != nil && !getAccountKeyFromSecret && (azureStorageAuthType == "" || strings.EqualFold(azureStorageAuthType, "key")) {
					klog.V(2).Infof("get account(%s) key from secret(%s, %s) failed with error: %v, use cluster identity to get account key instead",
						accountName, secretNamespace, secretName, err)
					accountKey, err = d.GetStorageAccesskeyWithSubsID(ctx, subsID, accountName, rgName, getLatestAccountKey)
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
				case storageSPNClientIDField:
					storageSPNClientID = v
				case storageSPNTenantIDField:
					storageSPNTenantID = v
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

	if storageSPNClientID != "" {
		klog.V(2).Infof("storageSPNClientID(%s) is not empty, use it to access storage account(%s), container(%s)", storageSPNClientID, accountName, containerName)
		authEnv = append(authEnv, "AZURE_STORAGE_SPN_CLIENT_ID="+storageSPNClientID)
	}

	if storageSPNTenantID != "" {
		klog.V(2).Infof("storageSPNTenantID(%s) is not empty, use it to access storage account(%s), container(%s)", storageSPNTenantID, accountName, containerName)
		authEnv = append(authEnv, "AZURE_STORAGE_SPN_TENANT_ID="+storageSPNTenantID)
	}

	if azureStorageAuthType == storageAuthTypeMSI {
		// check whether authEnv contains AZURE_STORAGE_IDENTITY_ prefix
		containsIdentityEnv := false
		for _, env := range authEnv {
			if strings.HasPrefix(env, "AZURE_STORAGE_IDENTITY_") {
				klog.V(2).Infof("AZURE_STORAGE_IDENTITY_ is already set in authEnv, skip setting it again")
				containsIdentityEnv = true
				break
			}
		}
		if !containsIdentityEnv && d.cloud != nil && d.cloud.Config.AzureAuthConfig.UserAssignedIdentityID != "" {
			klog.V(2).Infof("azureStorageAuthType is set to %s, add AZURE_STORAGE_IDENTITY_CLIENT_ID(%s) into authEnv",
				azureStorageAuthType, d.cloud.Config.AzureAuthConfig.UserAssignedIdentityID)
			authEnv = append(authEnv, "AZURE_STORAGE_IDENTITY_CLIENT_ID="+d.cloud.Config.AzureAuthConfig.UserAssignedIdentityID)
		}
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
		getLatestAccountKey   bool
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
		case getLatestAccountKeyField:
			if getLatestAccountKey, err = strconv.ParseBool(v); err != nil {
				return "", "", "", "", fmt.Errorf("invalid %s: %s in volume context", getLatestAccountKeyField, v)
			}
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
			accountKey, err = d.GetStorageAccesskeyWithSubsID(ctx, subsID, accountName, rgName, getLatestAccountKey)
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
		if protocol == v || protocol == NFSv3 {
			return true
		}
	}
	return false
}

func isSupportedAccessTier(accessTier string) bool {
	if accessTier == "" {
		return true
	}
	for _, tier := range armstorage.PossibleAccessTierValues() {
		if accessTier == string(tier) {
			return true
		}
	}
	return false
}

func isSupportedPublicNetworkAccess(publicNetworkAccess string) bool {
	if publicNetworkAccess == "" {
		return true
	}
	for _, tier := range armstorage.PossiblePublicNetworkAccessValues() {
		if publicNetworkAccess == string(tier) {
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

// isNFSProtocol checks if the protocol is NFS or AZNFS
func isNFSProtocol(protocol string) bool {
	protocol = strings.ToLower(protocol)
	return protocol == NFS || protocol == AZNFS || protocol == NFSv3
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
		return accountName, accountKey, fmt.Errorf("could not find %s or %s field in secrets", accountNameField, defaultSecretAccountName)
	}
	if accountKey == "" {
		return accountName, accountKey, fmt.Errorf("could not find %s or %s field in secrets", accountKeyField, defaultSecretAccountKey)
	}

	accountName = strings.TrimSpace(accountName)
	klog.V(4).Infof("got storage account(%s) from secret", accountName)
	return accountName, accountKey, nil
}

func getContainerReference(containerName string, secrets map[string]string, storageEndpointSuffix string) (*azstorage.Container, error) {
	accountName, accountKey, rerr := getStorageAccount(secrets)
	if rerr != nil {
		return nil, rerr
	}
	client, err := azstorage.NewClient(accountName, accountKey, storageEndpointSuffix, azstorage.DefaultAPIVersion, true)
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
	if apierror.IsAlreadyExists(err) {
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
func (d *Driver) GetStorageAccesskey(ctx context.Context, accountOptions *storage.AccountOptions, secrets map[string]string, secretName, secretNamespace string) (string, string, error) {
	if len(secrets) > 0 {
		return getStorageAccount(secrets)
	}

	// read from k8s secret first
	if secretName == "" {
		secretName = fmt.Sprintf(secretNameTemplate, accountOptions.Name)
	}
	_, accountKey, _, _, _, _, _, err := d.GetInfoFromSecret(ctx, secretName, secretNamespace) //nolint
	if err != nil && d.cloud != nil {
		klog.V(2).Infof("could not get account(%s) key from secret(%s) namespace(%s), error: %v, use cluster identity to get account key instead", accountOptions.Name, secretName, secretNamespace, err)
		accountKey, err = d.GetStorageAccesskeyWithSubsID(ctx, accountOptions.SubscriptionID, accountOptions.Name, accountOptions.ResourceGroup, accountOptions.GetLatestAccountKey)
	}
	return accountOptions.Name, accountKey, err
}

// GetStorageAccesskeyWithSubsID get Azure storage account key from storage account directly
func (d *Driver) GetStorageAccesskeyWithSubsID(ctx context.Context, subsID, account, resourceGroup string, getLatestAccountKey bool) (string, error) {
	if d.cloud == nil || d.cloud.ComputeClientFactory == nil {
		return "", fmt.Errorf("could not get account key: cloud or ComputeClientFactory is nil")
	}
	accountClient, err := d.cloud.ComputeClientFactory.GetAccountClientForSub(subsID)
	if err != nil {
		return "", err
	}
	return d.cloud.GetStorageAccesskey(ctx, accountClient, account, resourceGroup, getLatestAccountKey)
}

// GetInfoFromSecret get info from k8s secret
// return <accountName, accountKey, accountSasToken, msiSecret, spnClientSecret, spnClientID, spnTenantID, error>
func (d *Driver) GetInfoFromSecret(ctx context.Context, secretName, secretNamespace string) (string, string, string, string, string, string, string, error) {
	if d.KubeClient == nil {
		return "", "", "", "", "", "", "", fmt.Errorf("could not get account key from secret(%s): KubeClient is nil", secretName)
	}

	secret, err := d.KubeClient.CoreV1().Secrets(secretNamespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return "", "", "", "", "", "", "", fmt.Errorf("could not get secret(%v): %w", secretName, err)
	}

	accountName := strings.TrimSpace(string(secret.Data[defaultSecretAccountName][:]))
	accountKey := strings.TrimSpace(string(secret.Data[defaultSecretAccountKey][:]))
	accountSasToken := strings.TrimSpace(string(secret.Data[accountSasTokenField][:]))
	msiSecret := strings.TrimSpace(string(secret.Data[msiSecretField][:]))
	spnClientSecret := strings.TrimSpace(string(secret.Data[storageSPNClientSecretField][:]))
	spnClientID := strings.TrimSpace(string(secret.Data[storageSPNClientIDField][:]))
	spnTenantID := strings.TrimSpace(string(secret.Data[storageSPNTenantIDField][:]))

	klog.V(4).Infof("got storage account(%s) from secret(%s) namespace(%s)", accountName, secretName, secretNamespace)
	return accountName, accountKey, accountSasToken, msiSecret, spnClientSecret, spnClientID, spnTenantID, nil
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

func (d *Driver) useDataPlaneAPI(ctx context.Context, volumeID, accountName string) bool {
	cache, err := d.dataPlaneAPIVolCache.Get(ctx, volumeID, azcache.CacheReadTypeDefault)
	if err != nil {
		klog.Errorf("get(%s) from dataPlaneAPIVolCache failed with error: %v", volumeID, err)
	}
	if cache != nil {
		return true
	}
	cache, err = d.dataPlaneAPIVolCache.Get(ctx, accountName, azcache.CacheReadTypeDefault)
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
	expectedPerms := mode & os.ModePerm
	if perm != expectedPerms {
		klog.V(2).Infof("chmod targetPath(%s, mode:0%o) with permissions(0%o)", targetPath, info.Mode(), expectedPerms)
		// only change the permission mode bits, keep the other bits as is
		if err := os.Chmod(targetPath, (info.Mode()&^os.ModePerm)|os.FileMode(expectedPerms)); err != nil {
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

// getValueInMap get value from map by key
// key in the map is case insensitive
func getValueInMap(m map[string]string, key string) string {
	if m == nil {
		return ""
	}
	for k, v := range m {
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
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

func isSupportedFSGroupChangePolicy(policy string) bool {
	if policy == "" {
		return true
	}
	for _, v := range supportedFSGroupChangePolicyList {
		if policy == v {
			return true
		}
	}
	return false
}

func isReadOnlyFromCapability(vc *csi.VolumeCapability) bool {
	if vc.GetAccessMode() == nil {
		return false
	}
	mode := vc.GetAccessMode().GetMode()
	return (mode == csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY ||
		mode == csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY)
}

// generateVolumeName returns a PV name with clusterName prefix. The function
// should be used to generate a name of GCE PD or Cinder volume. It basically
// adds "<clusterName>-dynamic-" before the PV name, making sure the resulting
// string fits given length and cuts "dynamic" if not.
func generateVolumeName(clusterName, pvName string, maxLength int) string {
	prefix := clusterName + "-dynamic"
	pvLen := len(pvName)
	// cut the "<clusterName>-dynamic" to fit full pvName into maxLength
	// +1 for the '-' dash
	if pvLen+1+len(prefix) > maxLength {
		prefix = prefix[:maxLength-pvLen-1]
	}
	return prefix + "-" + pvName
}

// serviceAccountToken represents the service account token sent from NodePublishVolume Request.
// ref: https://kubernetes-csi.github.io/docs/token-requests.html
type serviceAccountToken struct {
	APIAzureADTokenExchange struct {
		Token               string    `json:"token"`
		ExpirationTimestamp time.Time `json:"expirationTimestamp"`
	} `json:"api://AzureADTokenExchange"`
}

// parseServiceAccountToken parses the bound service account token from the token passed from NodePublishVolume Request.
// ref: https://kubernetes-csi.github.io/docs/token-requests.html
func parseServiceAccountToken(tokenStr string) (string, error) {
	if len(tokenStr) == 0 {
		return "", fmt.Errorf("service account token is empty")
	}
	token := serviceAccountToken{}
	if err := json.Unmarshal([]byte(tokenStr), &token); err != nil {
		return "", fmt.Errorf("failed to unmarshal service account tokens, error: %w", err)
	}
	if token.APIAzureADTokenExchange.Token == "" {
		return "", fmt.Errorf("token for audience %s not found", DefaultTokenAudience)
	}
	return token.APIAzureADTokenExchange.Token, nil
}

// isValidTokenFileName checks if the token file name is valid
// fileName should only contain alphanumeric characters, hyphens
func isValidTokenFileName(fileName string) bool {
	if fileName == "" {
		return false
	}
	for _, c := range fileName {
		if !(('a' <= c && c <= 'z') ||
			('A' <= c && c <= 'Z') ||
			('0' <= c && c <= '9') ||
			(c == '-')) {
			return false
		}
	}
	return true
}

func isValidSubscriptionID(subsID string) bool {
	return subscriptionIDRegex.MatchString(subsID)
}
