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
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	network "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v6"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
	azure2 "github.com/Azure/go-autorest/autorest/azure"
	"golang.org/x/net/context"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/configloader"
	azcache "sigs.k8s.io/cloud-provider-azure/pkg/cache"
	azure "sigs.k8s.io/cloud-provider-azure/pkg/provider"
	azureconfig "sigs.k8s.io/cloud-provider-azure/pkg/provider/config"
	"sigs.k8s.io/cloud-provider-azure/pkg/provider/storage"
)

var (
	DefaultAzureCredentialFileEnv = "AZURE_CREDENTIAL_FILE"
	DefaultCredFilePath           = "/etc/kubernetes/azure.json"
	storageService                = "Microsoft.Storage"
)

// IsAzureStackCloud decides whether the driver is running on Azure Stack Cloud.
func IsAzureStackCloud(cloud *storage.AccountRepo) bool {
	return cloud != nil && !cloud.DisableAzureStackCloud && strings.EqualFold(cloud.Cloud, "AZURESTACKCLOUD")
}

// getCloudProvider get Azure Cloud Provider
func GetCloudProvider(ctx context.Context, kubeClient kubernetes.Interface, nodeID, secretName, secretNamespace, userAgent string, allowEmptyCloudConfig bool) (*storage.AccountRepo, error) {
	var (
		config     *azureconfig.Config
		fromSecret bool
		err        error
	)

	repo := &storage.AccountRepo{}
	defer func() {
		if repo.Environment == nil || repo.Environment.StorageEndpointSuffix == "" {
			repo.Environment = &azclient.Environment{
				StorageEndpointSuffix: defaultStorageEndPointSuffix,
			}
		}
	}()

	if kubeClient != nil {
		klog.V(2).Infof("reading cloud config from secret %s/%s", secretNamespace, secretName)
		config, err = configloader.Load[azureconfig.Config](ctx, &configloader.K8sSecretLoaderConfig{
			K8sSecretConfig: configloader.K8sSecretConfig{
				SecretName:      secretName,
				SecretNamespace: secretNamespace,
				CloudConfigKey:  "cloud-config",
			},
			KubeClient: kubeClient,
		}, nil)
		if err == nil && config != nil {
			fromSecret = true
		}
		if err != nil {
			klog.V(2).Infof("InitializeCloudFromSecret: failed to get cloud config from secret %s/%s: %v", secretNamespace, secretName, err)
		}
	}

	if config == nil {
		klog.V(2).Infof("could not read cloud config from secret %s/%s", secretNamespace, secretName)
		credFile, ok := os.LookupEnv(DefaultAzureCredentialFileEnv)
		if ok && strings.TrimSpace(credFile) != "" {
			klog.V(2).Infof("%s env var set as %v", DefaultAzureCredentialFileEnv, credFile)
		} else {
			credFile = DefaultCredFilePath
			klog.V(2).Infof("use default %s env var: %v", DefaultAzureCredentialFileEnv, credFile)
		}

		config, err = configloader.Load[azureconfig.Config](ctx, nil, &configloader.FileLoaderConfig{
			FilePath: credFile,
		})
		if err != nil {
			klog.Warningf("load azure config from file(%s) failed with %v", credFile, err)
		}
	}

	if config == nil {
		if allowEmptyCloudConfig {
			klog.V(2).Infof("no cloud config provided, error: %v, driver will run without cloud config", err)
		} else {
			return repo, fmt.Errorf("no cloud config provided, error: %w", err)
		}
	} else {
		config.UserAgent = userAgent
		config.CloudProviderBackoff = true
		// these environment variables are injected by workload identity webhook
		if tenantID := os.Getenv("AZURE_TENANT_ID"); tenantID != "" {
			config.TenantID = tenantID
		}
		if clientID := os.Getenv("AZURE_CLIENT_ID"); clientID != "" {
			config.AADClientID = clientID
		}
		if federatedTokenFile := os.Getenv("AZURE_FEDERATED_TOKEN_FILE"); federatedTokenFile != "" {
			config.AADFederatedTokenFile = federatedTokenFile
			config.UseFederatedWorkloadIdentityExtension = true
		}
		az := &azure.Cloud{}
		if err = az.InitializeCloudFromConfig(ctx, config, fromSecret, false); err != nil {
			klog.Warningf("InitializeCloudFromConfig failed with error: %v", err)
		}
		_, env, err := azclient.GetAzureCloudConfigAndEnvConfig(&config.ARMClientConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to get AzureCloudConfigAndEnvConfig: %v", err)
		}

		if nodeID == "" {
			// Disable UseInstanceMetadata for controller to mitigate a timeout issue using IMDS
			// https://github.com/kubernetes-sigs/azuredisk-csi-driver/issues/168
			klog.V(2).Infof("disable UseInstanceMetadata for controller server")
			config.UseInstanceMetadata = false
			klog.V(2).Infof("starting controller server...")
		} else {
			klog.V(2).Infof("starting node server on node(%s)", nodeID)
		}

		repo, err = storage.NewRepository(*config, env, az.ComputeClientFactory, az.NetworkClientFactory)
		if err != nil {
			return nil, fmt.Errorf("failed to create storage repository: %v", err)
		}
	}

	return repo, nil
}

// getKeyVaultSecretContent get content of the keyvault secret
func (d *Driver) getKeyVaultSecretContent(ctx context.Context, vaultURL string, secretName string, secretVersion string) (content string, err error) {
	var authProvider *azclient.AuthProvider
	authProvider, err = azclient.NewAuthProvider(&d.cloud.ARMClientConfig, &d.cloud.AzureAuthConfig)
	if err != nil {
		return "", err
	}
	kvClient, err := azsecrets.NewClient(vaultURL, authProvider.GetAzIdentity(), nil)
	if err != nil {
		return "", fmt.Errorf("failed to get keyvaultClient: %w", err)
	}

	klog.V(2).Infof("get secret from vaultURL(%v), sercretName(%v), secretVersion(%v)", vaultURL, secretName, secretVersion)
	secret, err := kvClient.GetSecret(ctx, secretName, secretVersion, nil)
	if err != nil {
		return "", fmt.Errorf("get secret from vaultURL(%v), sercretName(%v), secretVersion(%v) failed with error: %w", vaultURL, secretName, secretVersion, err)
	}
	return *secret.Value, nil
}

func (d *Driver) updateSubnetServiceEndpoints(ctx context.Context, vnetResourceGroup, vnetName, subnetName string) ([]string, error) {
	var vnetResourceIDs []string
	if d.networkClientFactory == nil {
		return vnetResourceIDs, fmt.Errorf("networkClientFactory is nil")
	}

	if vnetResourceGroup == "" {
		vnetResourceGroup = d.cloud.ResourceGroup
		if len(d.cloud.VnetResourceGroup) > 0 {
			vnetResourceGroup = d.cloud.VnetResourceGroup
		}
	}

	location := d.cloud.Location
	if vnetName == "" {
		vnetName = d.cloud.VnetName
	}

	klog.V(2).Infof("updateSubnetServiceEndpoints on vnetName: %s, subnetName: %s, location: %s", vnetName, subnetName, location)
	if vnetName == "" || location == "" {
		return vnetResourceIDs, fmt.Errorf("vnetName or location is empty")
	}

	lockKey := vnetResourceGroup + vnetName + subnetName
	cache, err := d.subnetCache.Get(ctx, lockKey, azcache.CacheReadTypeDefault)
	if err != nil {
		return nil, err
	}
	if cache != nil {
		vnetResourceIDs = cache.([]string)
		klog.V(2).Infof("subnet %s under vnet %s in rg %s is already updated, vnetResourceIDs: %v", subnetName, vnetName, vnetResourceGroup, vnetResourceIDs)
		return vnetResourceIDs, nil
	}

	d.subnetLockMap.LockEntry(lockKey)
	defer d.subnetLockMap.UnlockEntry(lockKey)

	var subnets []*network.Subnet
	if subnetName != "" {
		// list multiple subnets separated by comma
		subnetNames := strings.Split(subnetName, ",")
		for _, sn := range subnetNames {
			sn = strings.TrimSpace(sn)
			subnet, rerr := d.networkClientFactory.GetSubnetClient().Get(ctx, vnetResourceGroup, vnetName, sn, nil)
			if rerr != nil {
				return vnetResourceIDs, fmt.Errorf("failed to get the subnet %s under rg %s vnet %s: %v", subnetName, vnetResourceGroup, vnetName, rerr.Error())
			}
			subnets = append(subnets, subnet)
		}
	} else {
		var rerr error
		subnets, rerr = d.networkClientFactory.GetSubnetClient().List(ctx, vnetResourceGroup, vnetName)
		if rerr != nil {
			return vnetResourceIDs, fmt.Errorf("failed to list the subnets under rg %s vnet %s: %v", vnetResourceGroup, vnetName, rerr.Error())
		}
	}

	for _, subnet := range subnets {
		if subnet.Name == nil {
			return vnetResourceIDs, fmt.Errorf("subnet name is nil")
		}
		sn := *subnet.Name
		vnetResourceID := d.getSubnetResourceID(vnetResourceGroup, vnetName, sn)
		klog.V(2).Infof("set vnetResourceID %s", vnetResourceID)
		vnetResourceIDs = append(vnetResourceIDs, vnetResourceID)

		endpointLocaions := []*string{to.Ptr(location)}
		storageServiceEndpoint := &network.ServiceEndpointPropertiesFormat{
			Service:   &storageService,
			Locations: endpointLocaions,
		}
		storageServiceExists := false
		if subnet.Properties == nil {
			subnet.Properties = &network.SubnetPropertiesFormat{}
		}
		if subnet.Properties.ServiceEndpoints == nil {
			subnet.Properties.ServiceEndpoints = []*network.ServiceEndpointPropertiesFormat{}
		}
		serviceEndpoints := subnet.Properties.ServiceEndpoints
		for _, v := range serviceEndpoints {
			if strings.HasPrefix(ptr.Deref(v.Service, ""), storageService) {
				storageServiceExists = true
				klog.V(4).Infof("serviceEndpoint(%s) is already in subnet(%s)", storageService, sn)
				break
			}
		}

		if !storageServiceExists {
			serviceEndpoints = append(serviceEndpoints, storageServiceEndpoint)
			subnet.Properties.ServiceEndpoints = serviceEndpoints

			klog.V(2).Infof("begin to update the subnet %s under vnet %s in rg %s", sn, vnetName, vnetResourceGroup)
			if _, err := d.networkClientFactory.GetSubnetClient().CreateOrUpdate(ctx, vnetResourceGroup, vnetName, sn, *subnet); err != nil {
				return vnetResourceIDs, fmt.Errorf("failed to update the subnet %s under vnet %s: %v", sn, vnetName, err)
			}
		}
	}
	// cache the subnet update
	d.subnetCache.Set(lockKey, vnetResourceIDs)
	return vnetResourceIDs, nil
}

func (d *Driver) getStorageEndPointSuffix() string {
	if d.cloud == nil || d.cloud.Environment == nil || d.cloud.Environment.StorageEndpointSuffix == "" {
		return defaultStorageEndPointSuffix
	}
	return d.cloud.Environment.StorageEndpointSuffix
}

func (d *Driver) getCloudEnvironment() azure2.Environment {
	if d.cloud == nil || d.cloud.Environment == nil {
		return azure2.PublicCloud
	}
	return azure2.Environment{
		Name:                       d.cloud.Environment.Name,
		ServiceManagementEndpoint:  d.cloud.Environment.ServiceManagementEndpoint,
		ResourceManagerEndpoint:    d.cloud.Environment.ResourceManagerEndpoint,
		ActiveDirectoryEndpoint:    d.cloud.Environment.ActiveDirectoryEndpoint,
		StorageEndpointSuffix:      d.cloud.Environment.StorageEndpointSuffix,
		ContainerRegistryDNSSuffix: d.cloud.Environment.ContainerRegistryDNSSuffix,
		TokenAudience:              d.cloud.Environment.TokenAudience,
	}
}

// getBackOff returns a backoff object based on the config
func getBackOff(config azureconfig.Config) wait.Backoff {
	steps := config.CloudProviderBackoffRetries
	if steps < 1 {
		steps = 1
	}
	return wait.Backoff{
		Steps:    steps,
		Factor:   config.CloudProviderBackoffExponent,
		Jitter:   config.CloudProviderBackoffJitter,
		Duration: time.Duration(config.CloudProviderBackoffDuration) * time.Second,
	}
}
