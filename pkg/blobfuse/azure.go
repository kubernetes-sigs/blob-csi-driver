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
	"os"
	"strings"

	kv "github.com/Azure/azure-sdk-for-go/services/keyvault/2016-10-01/keyvault"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure"

	"k8s.io/klog"
	azureprovider "k8s.io/legacy-cloud-providers/azure"

	"golang.org/x/net/context"
)

// GetCloudProvider get Azure Cloud Provider
func GetCloudProvider() (*azureprovider.Cloud, error) {
	credFile, ok := os.LookupEnv("AZURE_CREDENTIAL_FILE")
	if ok {
		klog.V(2).Infof("AZURE_CREDENTIAL_FILE env var set as %v", credFile)
	} else {
		credFile = "/etc/kubernetes/azure.json"
		klog.V(2).Infof("use default AZURE_CREDENTIAL_FILE env var: %v", credFile)
	}

	f, err := os.Open(credFile)
	if err != nil {
		klog.Errorf("Failed to load config from file: %s", credFile)
		return nil, fmt.Errorf("Failed to load config from file: %s, cloud not get azure cloud provider", credFile)
	}
	defer f.Close()

	return azureprovider.NewCloudWithoutFeatureGates(f)
}

// getKeyVaultSecretContent get content of the keyvault secret
func (d *Driver) getKeyVaultSecretContent(ctx context.Context, vaultURL string, secretName string, secretVersion string) (content string, err error) {
	kvClient, err := d.initializeKvClient()
	if err != nil {
		return "", fmt.Errorf("failed to get keyvaultClient: %v", err)
	}

	secret, err := kvClient.GetSecret(ctx, vaultURL, secretName, secretVersion)
	if err != nil {
		return "", fmt.Errorf("failed to use vaultURL(%v), sercretName(%v), secretVersion(%v) to get secret: %v", vaultURL, secretName, secretVersion, err)
	}
	return *secret.Value, nil
}

func (d *Driver) initializeKvClient() (*kv.BaseClient, error) {
	kvClient := kv.New()
	token, err := d.getKeyvaultToken()
	if err != nil {
		return nil, err
	}

	kvClient.Authorizer = token
	return &kvClient, nil
}

// getKeyvaultToken retrieves a new service principal token to access keyvault
func (d *Driver) getKeyvaultToken() (authorizer autorest.Authorizer, err error) {
	env := d.cloud.Environment

	kvEndPoint := strings.TrimSuffix(env.KeyVaultEndpoint, "/")
	servicePrincipalToken, err := d.getServicePrincipalToken(env, kvEndPoint)
	if err != nil {
		return nil, err
	}
	authorizer = autorest.NewBearerAuthorizer(servicePrincipalToken)
	return authorizer, nil
}

// getServicePrincipalToken creates a new service principal token based on the configuration
func (d *Driver) getServicePrincipalToken(env azure.Environment, resource string) (*adal.ServicePrincipalToken, error) {
	oauthConfig, err := adal.NewOAuthConfig(env.ActiveDirectoryEndpoint, d.cloud.TenantID)
	if err != nil {
		return nil, fmt.Errorf("creating the OAuth config: %v", err)
	}

	return adal.NewServicePrincipalToken(
		*oauthConfig,
		d.cloud.AADClientID,
		d.cloud.AADClientSecret,
		resource)
}
