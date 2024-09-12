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
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/sas"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/service"
	azstorage "github.com/Azure/azure-sdk-for-go/storage"
	"github.com/container-storage-interface/spec/lib/go/csi"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"

	"sigs.k8s.io/blob-csi-driver/pkg/util"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/blobcontainerclient"
	azcache "sigs.k8s.io/cloud-provider-azure/pkg/cache"
	"sigs.k8s.io/cloud-provider-azure/pkg/metrics"
	"sigs.k8s.io/cloud-provider-azure/pkg/provider"
	azure "sigs.k8s.io/cloud-provider-azure/pkg/provider"
)

const (
	privateEndpoint = "privateendpoint"

	azcopyAutoLoginType             = "AZCOPY_AUTO_LOGIN_TYPE"
	azcopySPAApplicationID          = "AZCOPY_SPA_APPLICATION_ID"
	azcopySPAClientSecret           = "AZCOPY_SPA_CLIENT_SECRET"
	azcopyTenantID                  = "AZCOPY_TENANT_ID"
	azcopyMSIClientID               = "AZCOPY_MSI_CLIENT_ID"
	MSI                             = "MSI"
	SPN                             = "SPN"
	authorizationPermissionMismatch = "AuthorizationPermissionMismatch"

	createdByMetadata = "createdBy"
)

// CreateVolume provisions a volume
func (d *Driver) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	if err := d.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		klog.Errorf("invalid create volume req: %v", req)
		return nil, err
	}

	volName := req.GetName()
	if len(volName) == 0 {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume Name must be provided")
	}

	if err := isValidVolumeCapabilities(req.GetVolumeCapabilities()); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	volSizeBytes := int64(req.GetCapacityRange().GetRequiredBytes())
	requestGiB := int(util.RoundUpGiB(volSizeBytes))

	volContentSource := req.GetVolumeContentSource()
	secrets := req.GetSecrets()

	parameters := req.GetParameters()
	if parameters == nil {
		parameters = make(map[string]string)
	}
	var storageAccountType, subsID, resourceGroup, location, account, containerName, containerNamePrefix, protocol, customTags, secretName, secretNamespace, pvcNamespace, tagValueDelimiter string
	var isHnsEnabled, requireInfraEncryption, enableBlobVersioning, createPrivateEndpoint, enableNfsV3, allowSharedKeyAccess *bool
	var vnetResourceGroup, vnetName, subnetName, accessTier, networkEndpointType, storageEndpointSuffix, fsGroupChangePolicy string
	var matchTags, useDataPlaneAPI, getLatestAccountKey bool
	var softDeleteBlobs, softDeleteContainers int32
	var vnetResourceIDs []string
	var err error
	// set allowBlobPublicAccess as false by default
	allowBlobPublicAccess := ptr.To(false)

	containerNameReplaceMap := map[string]string{}

	// store account key to k8s secret by default
	storeAccountKey := true

	// Apply ProvisionerParameters (case-insensitive). We leave validation of
	// the values to the cloud provider.
	for k, v := range parameters {
		switch strings.ToLower(k) {
		case skuNameField:
			storageAccountType = v
		case storageAccountTypeField:
			storageAccountType = v
		case locationField:
			location = v
		case storageAccountField:
			account = v
		case subscriptionIDField:
			subsID = v
		case resourceGroupField:
			resourceGroup = v
		case containerNameField:
			containerName = v
		case containerNamePrefixField:
			containerNamePrefix = v
		case protocolField:
			protocol = v
		case tagsField:
			customTags = v
		case matchTagsField:
			matchTags = strings.EqualFold(v, trueValue)
		case secretNameField:
			secretName = v
		case secretNamespaceField:
			secretNamespace = v
		case isHnsEnabledField:
			if strings.EqualFold(v, trueValue) {
				isHnsEnabled = ptr.To(true)
			}
		case softDeleteBlobsField:
			days, err := parseDays(v)
			if err != nil {
				return nil, err
			}
			softDeleteBlobs = days
		case softDeleteContainersField:
			days, err := parseDays(v)
			if err != nil {
				return nil, err
			}
			softDeleteContainers = days
		case enableBlobVersioningField:
			enableBlobVersioning = ptr.To(strings.EqualFold(v, trueValue))
		case storeAccountKeyField:
			if strings.EqualFold(v, falseValue) {
				storeAccountKey = false
			}
		case getLatestAccountKeyField:
			if getLatestAccountKey, err = strconv.ParseBool(v); err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "invalid %s: %s in volume context", getLatestAccountKeyField, v)
			}
		case allowBlobPublicAccessField:
			if strings.EqualFold(v, trueValue) {
				allowBlobPublicAccess = ptr.To(true)
			}
		case allowSharedKeyAccessField:
			var boolValue bool
			if boolValue, err = strconv.ParseBool(v); err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "invalid %s: %s in volume context", allowSharedKeyAccessField, v)
			}
			allowSharedKeyAccess = ptr.To(boolValue)
		case requireInfraEncryptionField:
			if strings.EqualFold(v, trueValue) {
				requireInfraEncryption = ptr.To(true)
			}
		case pvcNamespaceKey:
			pvcNamespace = v
			containerNameReplaceMap[pvcNamespaceMetadata] = v
		case pvcNameKey:
			containerNameReplaceMap[pvcNameMetadata] = v
		case pvNameKey:
			containerNameReplaceMap[pvNameMetadata] = v
		case serverNameField:
		case storageAuthTypeField:
		case storageIdentityClientIDField:
		case storageIdentityObjectIDField:
		case storageIdentityResourceIDField:
		case msiEndpointField:
		case storageAADEndpointField:
			// no op, only used in NodeStageVolume
		case storageEndpointSuffixField:
			storageEndpointSuffix = v
		case vnetResourceGroupField:
			vnetResourceGroup = v
		case vnetNameField:
			vnetName = v
		case subnetNameField:
			subnetName = v
		case accessTierField:
			accessTier = v
		case networkEndpointTypeField:
			networkEndpointType = v
		case mountPermissionsField:
			// only do validations here, used in NodeStageVolume, NodePublishVolume
			if v != "" {
				if _, err := strconv.ParseUint(v, 8, 32); err != nil {
					return nil, status.Errorf(codes.InvalidArgument, "invalid mountPermissions %s in storage class", v)
				}
			}
		case useDataPlaneAPIField:
			useDataPlaneAPI = strings.EqualFold(v, trueValue)
		case fsGroupChangePolicyField:
			fsGroupChangePolicy = v
		case tagValueDelimiterField:
			tagValueDelimiter = v
		default:
			return nil, status.Errorf(codes.InvalidArgument, "invalid parameter %q in storage class", k)
		}
	}

	if ptr.Deref(enableBlobVersioning, false) {
		if isNFSProtocol(protocol) || ptr.Deref(isHnsEnabled, false) {
			return nil, status.Errorf(codes.InvalidArgument, "enableBlobVersioning is not supported for NFS protocol or HNS enabled account")
		}
	}

	if !isSupportedFSGroupChangePolicy(fsGroupChangePolicy) {
		return nil, status.Errorf(codes.InvalidArgument, "fsGroupChangePolicy(%s) is not supported, supported fsGroupChangePolicy list: %v", fsGroupChangePolicy, supportedFSGroupChangePolicyList)
	}

	if matchTags && account != "" {
		return nil, status.Errorf(codes.InvalidArgument, "matchTags must set as false when storageAccount(%s) is provided", account)
	}

	if resourceGroup == "" {
		resourceGroup = d.cloud.ResourceGroup
	}

	if secretNamespace == "" {
		if pvcNamespace == "" {
			secretNamespace = defaultNamespace
		} else {
			secretNamespace = pvcNamespace
		}
	}

	if protocol == "" {
		protocol = Fuse
	}
	if !isSupportedProtocol(protocol) {
		return nil, status.Errorf(codes.InvalidArgument, "protocol(%s) is not supported, supported protocol list: %v", protocol, supportedProtocolList)
	}
	if !isSupportedAccessTier(accessTier) {
		return nil, status.Errorf(codes.InvalidArgument, "accessTier(%s) is not supported, supported AccessTier list: %v", accessTier, armstorage.PossibleAccessTierValues())
	}

	if containerName != "" && containerNamePrefix != "" {
		return nil, status.Errorf(codes.InvalidArgument, "containerName(%s) and containerNamePrefix(%s) could not be specified together", containerName, containerNamePrefix)
	}
	if !isSupportedContainerNamePrefix(containerNamePrefix) {
		return nil, status.Errorf(codes.InvalidArgument, "containerNamePrefix(%s) can only contain lowercase letters, numbers, hyphens, and length should be less than 21", containerNamePrefix)
	}

	enableHTTPSTrafficOnly := true
	if strings.EqualFold(networkEndpointType, privateEndpoint) {
		if strings.Contains(subnetName, ",") {
			return nil, status.Errorf(codes.InvalidArgument, "subnetName(%s) can only contain one subnet for private endpoint", subnetName)
		}
		createPrivateEndpoint = ptr.To(true)
	}
	accountKind := string(armstorage.KindStorageV2)
	if isNFSProtocol(protocol) {
		isHnsEnabled = ptr.To(true)
		enableNfsV3 = ptr.To(true)
		// NFS protocol does not need account key
		storeAccountKey = false
		if !ptr.Deref(createPrivateEndpoint, false) {
			// set VirtualNetworkResourceIDs for storage account firewall setting
			var err error
			if vnetResourceIDs, err = d.updateSubnetServiceEndpoints(ctx, vnetResourceGroup, vnetName, subnetName); err != nil {
				return nil, status.Errorf(codes.Internal, "update service endpoints failed with error: %v", err)
			}
		}
	}

	if strings.HasPrefix(strings.ToLower(storageAccountType), "premium") {
		accountKind = string(armstorage.KindBlockBlobStorage)
	}
	if IsAzureStackCloud(d.cloud) {
		accountKind = string(armstorage.KindStorage)
		if storageAccountType != "" && storageAccountType != string(armstorage.SKUNameStandardLRS) && storageAccountType != string(armstorage.SKUNamePremiumLRS) {
			return nil, status.Errorf(codes.InvalidArgument, "Invalid skuName value: %s, as Azure Stack only supports %s and %s Storage Account types.", storageAccountType, armstorage.SKUNamePremiumLRS, armstorage.SKUNameStandardLRS)
		}
	}

	tags, err := util.ConvertTagsToMap(customTags, tagValueDelimiter)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}

	if strings.TrimSpace(storageEndpointSuffix) == "" {
		storageEndpointSuffix = d.getStorageEndPointSuffix()
	}

	if storeAccountKey && !ptr.Deref(allowSharedKeyAccess, true) {
		return nil, status.Errorf(codes.InvalidArgument, "storeAccountKey is not supported for account with shared access key disabled")
	}

	accountOptions := &azure.AccountOptions{
		Name:                            account,
		Type:                            storageAccountType,
		Kind:                            accountKind,
		SubscriptionID:                  subsID,
		ResourceGroup:                   resourceGroup,
		Location:                        location,
		EnableHTTPSTrafficOnly:          enableHTTPSTrafficOnly,
		VirtualNetworkResourceIDs:       vnetResourceIDs,
		Tags:                            tags,
		MatchTags:                       matchTags,
		IsHnsEnabled:                    isHnsEnabled,
		EnableNfsV3:                     enableNfsV3,
		AllowBlobPublicAccess:           allowBlobPublicAccess,
		AllowSharedKeyAccess:            allowSharedKeyAccess,
		RequireInfrastructureEncryption: requireInfraEncryption,
		VNetResourceGroup:               vnetResourceGroup,
		VNetName:                        vnetName,
		SubnetName:                      subnetName,
		AccessTier:                      accessTier,
		CreatePrivateEndpoint:           createPrivateEndpoint,
		StorageType:                     provider.StorageTypeBlob,
		StorageEndpointSuffix:           storageEndpointSuffix,
		EnableBlobVersioning:            enableBlobVersioning,
		SoftDeleteBlobs:                 softDeleteBlobs,
		SoftDeleteContainers:            softDeleteContainers,
		GetLatestAccountKey:             getLatestAccountKey,
	}

	containerName = replaceWithMap(containerName, containerNameReplaceMap)
	validContainerName := containerName
	if validContainerName == "" {
		validContainerName = volName
		if containerNamePrefix != "" {
			validContainerName = containerNamePrefix + "-" + volName
		}
		validContainerName = getValidContainerName(validContainerName, protocol)
		setKeyValueInMap(parameters, containerNameField, validContainerName)
	}

	if acquired := d.volumeLocks.TryAcquire(volName); !acquired {
		// logging the job status if it's volume cloning
		if volContentSource != nil {
			jobState, percent, err := d.azcopy.GetAzcopyJob(validContainerName, []string{})
			return nil, status.Errorf(codes.Aborted, volumeOperationAlreadyExistsWithAzcopyFmt, volName, jobState, percent, err)
		}
		return nil, status.Errorf(codes.Aborted, volumeOperationAlreadyExistsFmt, volName)
	}
	defer d.volumeLocks.Release(volName)

	requestName := "controller_create_volume"
	if volContentSource != nil {
		switch volContentSource.Type.(type) {
		case *csi.VolumeContentSource_Snapshot:
			return nil, status.Errorf(codes.InvalidArgument, "VolumeContentSource Snapshot is not yet implemented")
		case *csi.VolumeContentSource_Volume:
			requestName = "controller_create_volume_from_volume"
		}
	}

	var volumeID string
	mc := metrics.NewMetricContext(blobCSIDriverName, requestName, d.cloud.ResourceGroup, d.cloud.SubscriptionID, d.Name)
	isOperationSucceeded := false
	defer func() {
		mc.ObserveOperationWithResult(isOperationSucceeded, VolumeID, volumeID)
	}()

	var accountKey string
	accountName := account
	if len(secrets) == 0 && accountName == "" {
		if v, ok := d.volMap.Load(volName); ok {
			accountName = v.(string)
		} else {
			lockKey := fmt.Sprintf("%s%s%s%s%s%v", storageAccountType, accountKind, resourceGroup, location, protocol, ptr.Deref(createPrivateEndpoint, false))
			// search in cache first
			cache, err := d.accountSearchCache.Get(lockKey, azcache.CacheReadTypeDefault)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "%v", err)
			}
			if cache != nil {
				accountName = cache.(string)
			} else {
				d.volLockMap.LockEntry(lockKey)
				err = wait.ExponentialBackoff(d.cloud.RequestBackoff(), func() (bool, error) {
					var retErr error
					accountName, accountKey, retErr = d.cloud.EnsureStorageAccount(ctx, accountOptions, protocol)
					if isRetriableError(retErr) {
						klog.Warningf("EnsureStorageAccount(%s) failed with error(%v), waiting for retrying", account, retErr)
						return false, nil
					}
					return true, retErr
				})
				d.volLockMap.UnlockEntry(lockKey)
				if err != nil {
					return nil, status.Errorf(codes.Internal, "ensure storage account failed with %v", err)
				}
				d.accountSearchCache.Set(lockKey, accountName)
				d.volMap.Store(volName, accountName)
			}
		}
	}

	if ptr.Deref(createPrivateEndpoint, false) && isNFSProtocol(protocol) {
		// As for blobfuse/blobfuse2, serverName, i.e.,AZURE_STORAGE_BLOB_ENDPOINT env variable can't include
		// "privatelink", issue: https://github.com/Azure/azure-storage-fuse/issues/1014
		//
		// And use public endpoint will be befine to blobfuse/blobfuse2, because it will be resolved to private endpoint
		// by private dns zone, which includes CNAME record, documented here:
		// https://learn.microsoft.com/en-us/azure/storage/common/storage-private-endpoints?toc=%2Fazure%2Fstorage%2Fblobs%2Ftoc.json&bc=%2Fazure%2Fstorage%2Fblobs%2Fbreadcrumb%2Ftoc.json#dns-changes-for-private-endpoints
		setKeyValueInMap(parameters, serverNameField, fmt.Sprintf("%s.privatelink.blob.%s", accountName, storageEndpointSuffix))
	}

	accountOptions.Name = accountName
	if len(secrets) == 0 && useDataPlaneAPI {
		if accountKey == "" {
			if accountName, accountKey, err = d.GetStorageAccesskey(ctx, accountOptions, secrets, secretName, secretNamespace); err != nil {
				return nil, status.Errorf(codes.Internal, "failed to GetStorageAccesskey on account(%s) rg(%s), error: %v", accountOptions.Name, accountOptions.ResourceGroup, err)
			}
		}
		secrets = createStorageAccountSecret(accountName, accountKey)
	}

	klog.V(2).Infof("begin to create container(%s) on account(%s) type(%s) subsID(%s) rg(%s) location(%s) size(%d)", validContainerName, accountName, storageAccountType, subsID, resourceGroup, location, requestGiB)
	if err := d.CreateBlobContainer(ctx, subsID, resourceGroup, accountName, validContainerName, secrets); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create container(%s) on account(%s) type(%s) rg(%s) location(%s) size(%d), error: %v", validContainerName, accountName, storageAccountType, resourceGroup, location, requestGiB, err)
	}
	if volContentSource != nil {
		accountSASToken, authAzcopyEnv, err := d.getAzcopyAuth(ctx, accountName, accountKey, storageEndpointSuffix, accountOptions, secrets, secretName, secretNamespace, false)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to getAzcopyAuth on account(%s) rg(%s), error: %v", accountOptions.Name, accountOptions.ResourceGroup, err)
		}
		var copyErr error
		copyErr = d.copyVolume(ctx, req, accountName, accountSASToken, authAzcopyEnv, validContainerName, secretNamespace, accountOptions, storageEndpointSuffix)
		if accountSASToken == "" && copyErr != nil && strings.Contains(copyErr.Error(), authorizationPermissionMismatch) {
			klog.Warningf("azcopy copy failed with AuthorizationPermissionMismatch error, should assign \"Storage Blob Data Contributor\" role to controller identity, fall back to use sas token, original error: %v", copyErr)
			accountSASToken, authAzcopyEnv, err := d.getAzcopyAuth(ctx, accountName, accountKey, storageEndpointSuffix, accountOptions, secrets, secretName, secretNamespace, true)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "failed to getAzcopyAuth on account(%s) rg(%s), error: %v", accountOptions.Name, accountOptions.ResourceGroup, err)
			}
			copyErr = d.copyVolume(ctx, req, accountName, accountSASToken, authAzcopyEnv, validContainerName, secretNamespace, accountOptions, storageEndpointSuffix)
		}
		if copyErr != nil {
			return nil, copyErr
		}
	}

	if storeAccountKey && len(req.GetSecrets()) == 0 {
		if accountKey == "" {
			if accountName, accountKey, err = d.GetStorageAccesskey(ctx, accountOptions, secrets, secretName, secretNamespace); err != nil {
				return nil, status.Errorf(codes.Internal, "failed to GetStorageAccesskey on account(%s) rg(%s), error: %v", accountOptions.Name, accountOptions.ResourceGroup, err)
			}
		}

		secretName, err := setAzureCredentials(ctx, d.KubeClient, accountName, accountKey, secretNamespace)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to store storage account key: %v", err)
		}
		if secretName != "" {
			klog.V(2).Infof("store account key to k8s secret(%v) in %s namespace", secretName, secretNamespace)
		}
	}

	var uuid string
	if containerName != "" {
		// add volume name as suffix to differentiate volumeID since "containerName" is specified
		// not necessary for dynamic container name creation since volumeID already contains volume name
		uuid = volName
	}
	volumeID = fmt.Sprintf(volumeIDTemplate, resourceGroup, accountName, validContainerName, uuid, secretNamespace, subsID)
	klog.V(2).Infof("create container %s on storage account %s successfully", validContainerName, accountName)

	if useDataPlaneAPI {
		d.dataPlaneAPIVolCache.Set(volumeID, "")
		d.dataPlaneAPIVolCache.Set(accountName, "")
	}

	isOperationSucceeded = true
	// reset secretNamespace field in VolumeContext
	setKeyValueInMap(parameters, secretNamespaceField, secretNamespace)
	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      volumeID,
			CapacityBytes: req.GetCapacityRange().GetRequiredBytes(),
			VolumeContext: parameters,
			ContentSource: volContentSource,
		},
	}, nil
}

// DeleteVolume delete a volume
func (d *Driver) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}

	if err := d.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		return nil, status.Errorf(codes.Internal, "invalid delete volume req: %v", req)
	}

	if acquired := d.volumeLocks.TryAcquire(volumeID); !acquired {
		return nil, status.Errorf(codes.Aborted, volumeOperationAlreadyExistsFmt, volumeID)
	}
	defer d.volumeLocks.Release(volumeID)

	resourceGroupName, accountName, containerName, _, subsID, err := GetContainerInfo(volumeID)
	if err != nil {
		// According to CSI Driver Sanity Tester, should succeed when an invalid volume id is used
		klog.Errorf("GetContainerInfo(%s) in DeleteVolume failed with error: %v", volumeID, err)
		return &csi.DeleteVolumeResponse{}, nil
	}

	secrets := req.GetSecrets()
	if len(secrets) == 0 && d.useDataPlaneAPI(volumeID, accountName) {
		_, accountName, accountKey, _, _, err := d.GetAuthEnv(ctx, volumeID, "", nil, secrets)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "GetAuthEnv(%s) failed with %v", volumeID, err)
		}
		if accountName != "" && accountKey != "" {
			secrets = createStorageAccountSecret(accountName, accountKey)
		}
	}

	mc := metrics.NewMetricContext(blobCSIDriverName, "controller_delete_volume", d.cloud.ResourceGroup, d.cloud.SubscriptionID, d.Name)
	isOperationSucceeded := false
	defer func() {
		mc.ObserveOperationWithResult(isOperationSucceeded, VolumeID, volumeID)
	}()

	if resourceGroupName == "" {
		resourceGroupName = d.cloud.ResourceGroup
	}
	klog.V(2).Infof("deleting container(%s) rg(%s) account(%s) volumeID(%s)", containerName, resourceGroupName, accountName, volumeID)
	if err := d.DeleteBlobContainer(ctx, subsID, resourceGroupName, accountName, containerName, secrets); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete container(%s) under rg(%s) account(%s) volumeID(%s), error: %v", containerName, resourceGroupName, accountName, volumeID, err)
	}

	isOperationSucceeded = true
	klog.V(2).Infof("container(%s) under rg(%s) account(%s) volumeID(%s) is deleted successfully", containerName, resourceGroupName, accountName, volumeID)
	return &csi.DeleteVolumeResponse{}, nil
}

// ValidateVolumeCapabilities return the capabilities of the volume
func (d *Driver) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}
	if err := isValidVolumeCapabilities(req.GetVolumeCapabilities()); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	resourceGroupName, accountName, containerName, _, subsID, err := GetContainerInfo(volumeID)
	if err != nil {
		klog.Errorf("GetContainerInfo(%s) in ValidateVolumeCapabilities failed with error: %v", volumeID, err)
		return nil, status.Error(codes.NotFound, err.Error())
	}

	var exist bool
	secrets := req.GetSecrets()
	if len(secrets) > 0 {
		container, err := getContainerReference(containerName, secrets, d.getCloudEnvironment())
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		exist, err = container.Exists()
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	} else {
		if resourceGroupName == "" {
			resourceGroupName = d.cloud.ResourceGroup
		}
		blobClient, err := d.clientFactory.GetBlobContainerClientForSub(subsID)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		blobContainer, err := blobClient.Get(ctx, resourceGroupName, accountName, containerName)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		if blobContainer.ContainerProperties == nil {
			return nil, status.Errorf(codes.Internal, "ContainerProperties of volume(%s) is nil", volumeID)
		}
		exist = blobContainer.ContainerProperties.Deleted != nil && !*blobContainer.ContainerProperties.Deleted
	}
	if !exist {
		return nil, status.Errorf(codes.NotFound, "requested volume(%s) does not exist", volumeID)
	}
	klog.V(2).Infof("ValidateVolumeCapabilities on volume(%s) succeeded", volumeID)

	// blob driver supports all AccessModes, no need to check capabilities here
	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeCapabilities: req.GetVolumeCapabilities(),
		},
		Message: "",
	}, nil
}

// ControllerModifyVolume modify volume
func (d *Driver) ControllerModifyVolume(_ context.Context, _ *csi.ControllerModifyVolumeRequest) (*csi.ControllerModifyVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (d *Driver) ControllerPublishVolume(_ context.Context, _ *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ControllerPublishVolume is not yet implemented")
}

func (d *Driver) ControllerUnpublishVolume(_ context.Context, _ *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ControllerUnpublishVolume is not yet implemented")
}

// ControllerGetVolume get volume
func (d *Driver) ControllerGetVolume(context.Context, *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ControllerGetVolume is not yet implemented")
}

// GetCapacity returns the capacity of the total available storage pool
func (d *Driver) GetCapacity(_ context.Context, _ *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	return nil, status.Error(codes.Unimplemented, "GetCapacity is not yet implemented")
}

// ListVolumes return all available volumes
func (d *Driver) ListVolumes(_ context.Context, _ *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ListVolumes is not yet implemented")
}

// CreateSnapshot create snapshot
func (d *Driver) CreateSnapshot(_ context.Context, _ *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "CreateSnapshot is not yet implemented")
}

// DeleteSnapshot delete snapshot
func (d *Driver) DeleteSnapshot(_ context.Context, _ *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "DeleteSnapshot is not yet implemented")
}

// ListSnapshots list snapshots
func (d *Driver) ListSnapshots(_ context.Context, _ *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ListSnapshots is not yet implemented")
}

// ControllerGetCapabilities returns the capabilities of the Controller plugin
func (d *Driver) ControllerGetCapabilities(_ context.Context, _ *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: d.Cap,
	}, nil
}

// ControllerExpandVolume controller expand volume
func (d *Driver) ControllerExpandVolume(_ context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}

	if req.GetCapacityRange() == nil {
		return nil, status.Error(codes.InvalidArgument, "Capacity Range missing in request")
	}

	if err := d.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_EXPAND_VOLUME); err != nil {
		return nil, status.Errorf(codes.Internal, "invalid expand volume req: %v", req)
	}

	volSizeBytes := int64(req.GetCapacityRange().GetRequiredBytes())
	requestGiB := int64(util.RoundUpGiB(volSizeBytes))

	if volSizeBytes > containerMaxSize {
		return nil, status.Errorf(codes.OutOfRange, "required bytes (%d) exceeds the maximum supported bytes (%d)", volSizeBytes, containerMaxSize)
	}

	klog.V(2).Infof("ControllerExpandVolume(%s) successfully, currentQuota: %d Gi", req.VolumeId, requestGiB)

	return &csi.ControllerExpandVolumeResponse{CapacityBytes: req.GetCapacityRange().GetRequiredBytes()}, nil
}

// CreateBlobContainer creates a blob container
func (d *Driver) CreateBlobContainer(ctx context.Context, subsID, resourceGroupName, accountName, containerName string, secrets map[string]string) error {
	if containerName == "" {
		return fmt.Errorf("containerName is empty")
	}
	return wait.ExponentialBackoff(d.cloud.RequestBackoff(), func() (bool, error) {
		var err error
		if len(secrets) > 0 {
			container, getErr := getContainerReference(containerName, secrets, d.getCloudEnvironment())
			if getErr != nil {
				return true, getErr
			}
			container.Metadata = map[string]string{createdByMetadata: d.Name}
			_, err = container.CreateIfNotExists(&azstorage.CreateContainerOptions{Access: azstorage.ContainerAccessTypePrivate})
		} else {
			blobContainer := armstorage.BlobContainer{
				ContainerProperties: &armstorage.ContainerProperties{
					PublicAccess: to.Ptr(armstorage.PublicAccessNone),
					Metadata:     map[string]*string{createdByMetadata: to.Ptr(d.Name)},
				},
			}
			var blobClient blobcontainerclient.Interface
			blobClient, err = d.clientFactory.GetBlobContainerClientForSub(subsID)
			if err != nil {
				return true, err
			}
			_, err = blobClient.CreateContainer(ctx, resourceGroupName, accountName, containerName, blobContainer)
		}
		if err != nil {
			if strings.Contains(err.Error(), containerBeingDeletedDataplaneAPIError) ||
				strings.Contains(err.Error(), containerBeingDeletedManagementAPIError) {
				klog.Warningf("CreateContainer(%s, %s, %s) failed with error(%v), retry", resourceGroupName, accountName, containerName, err)
				return false, nil
			}
		}
		return true, err
	})
}

// DeleteBlobContainer deletes a blob container
func (d *Driver) DeleteBlobContainer(ctx context.Context, subsID, resourceGroupName, accountName, containerName string, secrets map[string]string) error {
	if containerName == "" {
		return fmt.Errorf("containerName is empty")
	}
	return wait.ExponentialBackoff(d.cloud.RequestBackoff(), func() (bool, error) {
		var err error
		if len(secrets) > 0 {
			container, getErr := getContainerReference(containerName, secrets, d.getCloudEnvironment())
			if getErr != nil {
				return true, getErr
			}
			_, err = container.DeleteIfExists(nil)
		} else {
			var blobClient blobcontainerclient.Interface
			blobClient, err = d.clientFactory.GetBlobContainerClientForSub(subsID)
			if err != nil {
				return true, err
			}
			err = blobClient.DeleteContainer(ctx, resourceGroupName, accountName, containerName)
		}
		if err != nil {
			if strings.Contains(err.Error(), containerBeingDeletedDataplaneAPIError) ||
				strings.Contains(err.Error(), containerBeingDeletedManagementAPIError) ||
				strings.Contains(err.Error(), statusCodeNotFound) ||
				strings.Contains(err.Error(), httpCodeNotFound) {
				klog.Warningf("delete container(%s) on account(%s) failed with error(%v), return as success", containerName, accountName, err)
				return true, nil
			}
			return false, fmt.Errorf("failed to delete container(%s) on account(%s), error: %w", containerName, accountName, err)
		}
		return true, err
	})
}

// copyBlobContainer copies source volume content into a destination volume
func (d *Driver) copyBlobContainer(ctx context.Context, req *csi.CreateVolumeRequest, dstAccountName string, dstAccountSasToken string, authAzcopyEnv []string, dstContainerName string, secretNamespace string, accountOptions *azure.AccountOptions, storageEndpointSuffix string) error {
	var sourceVolumeID string
	if req.GetVolumeContentSource() != nil && req.GetVolumeContentSource().GetVolume() != nil {
		sourceVolumeID = req.GetVolumeContentSource().GetVolume().GetVolumeId()

	}
	srcResourceGroupName, srcAccountName, srcContainerName, _, srcSubscriptionID, err := GetContainerInfo(sourceVolumeID) //nolint:dogsled
	if err != nil {
		return status.Error(codes.NotFound, err.Error())
	}
	if dstAccountName == "" {
		dstAccountName = srcAccountName
	}
	if srcAccountName == "" || srcContainerName == "" || dstContainerName == "" {
		return fmt.Errorf("srcAccountName(%s) or srcContainerName(%s) or dstContainerName(%s) is empty", srcAccountName, srcContainerName, dstContainerName)
	}
	srcAccountSasToken := dstAccountSasToken
	if srcAccountName != dstAccountName && dstAccountSasToken != "" {
		srcAccountOptions := &azure.AccountOptions{
			Name:                srcAccountName,
			ResourceGroup:       srcResourceGroupName,
			SubscriptionID:      srcSubscriptionID,
			GetLatestAccountKey: accountOptions.GetLatestAccountKey,
		}
		if srcAccountSasToken, _, err = d.getAzcopyAuth(ctx, srcAccountName, "", storageEndpointSuffix, srcAccountOptions, nil, "", secretNamespace, true); err != nil {
			return err
		}
	}
	srcPath := fmt.Sprintf("https://%s.blob.%s/%s%s", srcAccountName, storageEndpointSuffix, srcContainerName, srcAccountSasToken)
	dstPath := fmt.Sprintf("https://%s.blob.%s/%s%s", dstAccountName, storageEndpointSuffix, dstContainerName, dstAccountSasToken)

	jobState, percent, err := d.azcopy.GetAzcopyJob(dstContainerName, authAzcopyEnv)
	klog.V(2).Infof("azcopy job status: %s, copy percent: %s%%, error: %v", jobState, percent, err)
	switch jobState {
	case util.AzcopyJobError, util.AzcopyJobCompleted:
		return err
	case util.AzcopyJobRunning:
		return fmt.Errorf("wait for the existing AzCopy job to complete, current copy percentage is %s%%", percent)
	case util.AzcopyJobNotFound:
		klog.V(2).Infof("copy blob container %s:%s to %s:%s", srcAccountName, srcContainerName, dstAccountName, dstContainerName)
		execFunc := func() error {
			if out, err := d.execAzcopyCopy(srcPath, dstPath, azcopyCloneVolumeOptions, authAzcopyEnv); err != nil {
				return fmt.Errorf("exec error: %v, output: %v", err, string(out))
			}
			return nil
		}
		timeoutFunc := func() error {
			_, percent, _ := d.azcopy.GetAzcopyJob(dstContainerName, authAzcopyEnv)
			return fmt.Errorf("timeout waiting for copy blob container %s to %s complete, current copy percent: %s%%", srcContainerName, dstContainerName, percent)
		}
		copyErr := util.WaitUntilTimeout(time.Duration(d.waitForAzCopyTimeoutMinutes)*time.Minute, execFunc, timeoutFunc)
		if copyErr != nil {
			klog.Warningf("CopyBlobContainer(%s, %s, %s) failed with error: %v", accountOptions.ResourceGroup, dstAccountName, dstContainerName, copyErr)
		} else {
			klog.V(2).Infof("copied blob container %s to %s successfully", srcContainerName, dstContainerName)
		}
		return copyErr
	}
	return err
}

// copyVolume copies a volume form volume or snapshot, snapshot is not supported now
func (d *Driver) copyVolume(ctx context.Context, req *csi.CreateVolumeRequest, accountName string, accountSASToken string, authAzcopyEnv []string, dstContainerName, secretNamespace string, accountOptions *azure.AccountOptions, storageEndpointSuffix string) error {
	vs := req.VolumeContentSource
	switch vs.Type.(type) {
	case *csi.VolumeContentSource_Snapshot:
		return status.Errorf(codes.InvalidArgument, "VolumeContentSource Snapshot is not yet implemented")
	case *csi.VolumeContentSource_Volume:
		return d.copyBlobContainer(ctx, req, accountName, accountSASToken, authAzcopyEnv, dstContainerName, secretNamespace, accountOptions, storageEndpointSuffix)
	default:
		return status.Errorf(codes.InvalidArgument, "%v is not a proper volume source", vs)
	}
}

// execAzcopyCopy exec azcopy copy command
func (d *Driver) execAzcopyCopy(srcPath, dstPath string, azcopyCopyOptions, authAzcopyEnv []string) ([]byte, error) {
	cmd := exec.Command("azcopy", "copy", srcPath, dstPath)
	cmd.Args = append(cmd.Args, azcopyCopyOptions...)
	if len(authAzcopyEnv) > 0 {
		cmd.Env = append(os.Environ(), authAzcopyEnv...)
	}
	return cmd.CombinedOutput()
}

// authorizeAzcopyWithIdentity returns auth env for azcopy using cluster identity
func (d *Driver) authorizeAzcopyWithIdentity() ([]string, error) {
	azureAuthConfig := d.cloud.Config.AzureAuthConfig
	var authAzcopyEnv []string
	if azureAuthConfig.UseManagedIdentityExtension {
		authAzcopyEnv = append(authAzcopyEnv, fmt.Sprintf("%s=%s", azcopyAutoLoginType, MSI))
		if len(azureAuthConfig.UserAssignedIdentityID) > 0 {
			klog.V(2).Infof("use user assigned managed identity to authorize azcopy")
			authAzcopyEnv = append(authAzcopyEnv, fmt.Sprintf("%s=%s", azcopyMSIClientID, azureAuthConfig.UserAssignedIdentityID))
		} else {
			klog.V(2).Infof("use system-assigned managed identity to authorize azcopy")
		}
		return authAzcopyEnv, nil
	}
	if len(azureAuthConfig.AADClientSecret) > 0 {
		klog.V(2).Infof("use service principal to authorize azcopy")
		authAzcopyEnv = append(authAzcopyEnv, fmt.Sprintf("%s=%s", azcopyAutoLoginType, SPN))
		if azureAuthConfig.AADClientID == "" || azureAuthConfig.TenantID == "" {
			return []string{}, fmt.Errorf("AADClientID and TenantID must be set when use service principal")
		}
		authAzcopyEnv = append(authAzcopyEnv, fmt.Sprintf("%s=%s", azcopySPAApplicationID, azureAuthConfig.AADClientID))
		authAzcopyEnv = append(authAzcopyEnv, fmt.Sprintf("%s=%s", azcopySPAClientSecret, azureAuthConfig.AADClientSecret))
		authAzcopyEnv = append(authAzcopyEnv, fmt.Sprintf("%s=%s", azcopyTenantID, azureAuthConfig.TenantID))
		klog.V(2).Infof("set AZCOPY_SPA_APPLICATION_ID=%s, AZCOPY_TENANT_ID=%s successfully", azureAuthConfig.AADClientID, azureAuthConfig.TenantID)

		return authAzcopyEnv, nil
	}
	return []string{}, fmt.Errorf("service principle or managed identity are both not set")
}

// getAzcopyAuth will only generate sas token for azcopy in following conditions:
// 1. secrets is not empty
// 2. driver is not using managed identity and service principal
// 3. parameter useSasToken is true
func (d *Driver) getAzcopyAuth(ctx context.Context, accountName, accountKey, storageEndpointSuffix string, accountOptions *azure.AccountOptions, secrets map[string]string, secretName, secretNamespace string, useSasToken bool) (string, []string, error) {
	var authAzcopyEnv []string
	var err error
	if !useSasToken && !d.useDataPlaneAPI("", accountName) && len(secrets) == 0 && len(secretName) == 0 {
		// search in cache first
		if cache, err := d.azcopySasTokenCache.Get(accountName, azcache.CacheReadTypeDefault); err == nil && cache != nil {
			klog.V(2).Infof("use sas token for account(%s) since this account is found in azcopySasTokenCache", accountName)
			return cache.(string), nil, nil
		}

		authAzcopyEnv, err = d.authorizeAzcopyWithIdentity()
		if err != nil {
			klog.Warningf("failed to authorize azcopy with identity, error: %v", err)
		}
	}

	if len(secrets) > 0 || len(secretName) > 0 || len(authAzcopyEnv) == 0 || useSasToken {
		if accountKey == "" {
			if _, accountKey, err = d.GetStorageAccesskey(ctx, accountOptions, secrets, secretName, secretNamespace); err != nil {
				return "", nil, err
			}
		}
		klog.V(2).Infof("generate sas token for account(%s)", accountName)
		sasToken, err := d.generateSASToken(accountName, accountKey, storageEndpointSuffix, d.sasTokenExpirationMinutes)
		return sasToken, nil, err
	}
	return "", authAzcopyEnv, nil
}

// isValidVolumeCapabilities validates the given VolumeCapability array is valid
func isValidVolumeCapabilities(volCaps []*csi.VolumeCapability) error {
	if len(volCaps) == 0 {
		return fmt.Errorf("volume capabilities missing in request")
	}
	for _, c := range volCaps {
		if c.GetBlock() != nil {
			return fmt.Errorf("block volume capability not supported")
		}
	}
	return nil
}

func parseDays(dayStr string) (int32, error) {
	days, err := strconv.Atoi(dayStr)
	if err != nil {
		return 0, status.Errorf(codes.InvalidArgument, "invalid %s:%s in storage class", softDeleteBlobsField, dayStr)
	}
	if days <= 0 || days > 365 {
		return 0, status.Errorf(codes.InvalidArgument, "invalid %s:%s in storage class, should be in range [1, 365]", softDeleteBlobsField, dayStr)
	}

	return int32(days), nil
}

// generateSASToken generate a sas token for storage account
func (d *Driver) generateSASToken(accountName, accountKey, storageEndpointSuffix string, expiryTime int) (string, error) {
	// search in cache first
	cache, err := d.azcopySasTokenCache.Get(accountName, azcache.CacheReadTypeDefault)
	if err != nil {
		return "", fmt.Errorf("get(%s) from azcopySasTokenCache failed with error: %v", accountName, err)
	}
	if cache != nil {
		klog.V(2).Infof("use sas token for account(%s) since this account is found in azcopySasTokenCache", accountName)
		return cache.(string), nil
	}

	credential, err := azblob.NewSharedKeyCredential(accountName, accountKey)
	if err != nil {
		return "", status.Errorf(codes.Internal, "failed to generate sas token in creating new shared key credential, accountName: %s, err: %v", accountName, err)
	}
	clientOptions := service.ClientOptions{}
	clientOptions.InsecureAllowCredentialWithHTTP = true
	serviceClient, err := service.NewClientWithSharedKeyCredential(fmt.Sprintf("https://%s.blob.%s/", accountName, storageEndpointSuffix), credential, &clientOptions)
	if err != nil {
		return "", status.Errorf(codes.Internal, "failed to generate sas token in creating new client with shared key credential, accountName: %s, err: %v", accountName, err)
	}
	sasURL, err := serviceClient.GetSASURL(
		sas.AccountResourceTypes{Object: true, Service: false, Container: true},
		sas.AccountPermissions{Read: true, List: true, Write: true},
		time.Now().Add(time.Duration(expiryTime)*time.Minute),
		&service.GetSASURLOptions{StartTime: to.Ptr(time.Now())},
	)
	if err != nil {
		return "", err
	}
	u, err := url.Parse(sasURL)
	if err != nil {
		return "", err
	}
	sasToken := "?" + u.RawQuery
	d.azcopySasTokenCache.Set(accountName, sasToken)
	return sasToken, nil
}
