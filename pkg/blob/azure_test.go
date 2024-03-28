/*
Copyright 2020 The Kubernetes Authors.

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
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/services/network/mgmt/2022-07-01/network"
	"github.com/Azure/azure-sdk-for-go/storage"

	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"sigs.k8s.io/blob-csi-driver/pkg/util"
	"sigs.k8s.io/cloud-provider-azure/pkg/azureclients/subnetclient/mocksubnetclient"
	azureprovider "sigs.k8s.io/cloud-provider-azure/pkg/provider"

	"sigs.k8s.io/cloud-provider-azure/pkg/retry"
)

// TestGetCloudProvider tests the func getCloudProvider().
// To run this unit test successfully, need to ensure /etc/kubernetes/azure.json nonexistent.
func TestGetCloudProvider(t *testing.T) {
	fakeCredFile := "fake-cred-file.json"
	fakeKubeConfig := "fake-kube-config"
	emptyKubeConfig := "empty-kube-config"
	fakeContent := `
apiVersion: v1
clusters:
- cluster:
    server: https://localhost:8080
  name: foo-cluster
contexts:
- context:
    cluster: foo-cluster
    user: foo-user
    namespace: bar
  name: foo-context
current-context: foo-context
kind: Config
users:
- name: foo-user
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1alpha1
      args:
      - arg-1
      - arg-2
      command: foo-command
`

	err := createTestFile(emptyKubeConfig)
	if err != nil {
		t.Error(err)
	}
	defer func() {
		if err := os.Remove(emptyKubeConfig); err != nil {
			t.Error(err)
		}
	}()

	tests := []struct {
		desc                                  string
		createFakeCredFile                    bool
		createFakeKubeConfig                  bool
		setFederatedWorkloadIdentityEnv       bool
		kubeconfig                            string
		nodeID                                string
		userAgent                             string
		allowEmptyCloudConfig                 bool
		expectedErr                           error
		aadFederatedTokenFile                 string
		useFederatedWorkloadIdentityExtension bool
		aadClientID                           string
		tenantID                              string
	}{
		{
			desc:                  "out of cluster, no kubeconfig, no credential file",
			kubeconfig:            "",
			nodeID:                "",
			allowEmptyCloudConfig: true,
			expectedErr:           nil,
		},
		{
			desc:                  "[failure][disallowEmptyCloudConfig]  out of cluster, no kubeconfig, no credential file",
			kubeconfig:            "",
			nodeID:                "",
			allowEmptyCloudConfig: false,
			expectedErr:           nil,
		},
		{
			desc:                  "[failure] out of cluster & in cluster, specify a non-exist kubeconfig, no credential file",
			kubeconfig:            "/tmp/non-exist.json",
			nodeID:                "",
			allowEmptyCloudConfig: true,
			expectedErr:           nil,
		},
		{
			desc:                  "[failure] out of cluster & in cluster, specify a empty kubeconfig, no credential file",
			kubeconfig:            emptyKubeConfig,
			nodeID:                "",
			allowEmptyCloudConfig: true,
			expectedErr:           fmt.Errorf("invalid configuration: no configuration has been provided, try setting KUBERNETES_MASTER environment variable"),
		},
		{
			desc:                  "[failure] out of cluster & in cluster, specify a fake kubeconfig, no credential file",
			createFakeKubeConfig:  true,
			kubeconfig:            fakeKubeConfig,
			nodeID:                "",
			allowEmptyCloudConfig: true,
			expectedErr:           nil,
		},
		{
			desc:                  "[success] out of cluster & in cluster, no kubeconfig, a fake credential file",
			createFakeCredFile:    true,
			kubeconfig:            "",
			nodeID:                "",
			userAgent:             "useragent",
			allowEmptyCloudConfig: true,
			expectedErr:           nil,
		},
		{
			desc:                                  "[success] get azure client with workload identity",
			createFakeKubeConfig:                  true,
			createFakeCredFile:                    true,
			setFederatedWorkloadIdentityEnv:       true,
			kubeconfig:                            fakeKubeConfig,
			nodeID:                                "",
			userAgent:                             "useragent",
			useFederatedWorkloadIdentityExtension: true,
			aadFederatedTokenFile:                 "fake-token-file",
			aadClientID:                           "fake-client-id",
			tenantID:                              "fake-tenant-id",
			expectedErr:                           nil,
		},
	}

	for _, test := range tests {
		if test.createFakeKubeConfig {
			if err := createTestFile(fakeKubeConfig); err != nil {
				t.Error(err)
			}
			defer func() {
				if err := os.Remove(fakeKubeConfig); err != nil && !os.IsNotExist(err) {
					t.Error(err)
				}
			}()

			if err := os.WriteFile(fakeKubeConfig, []byte(fakeContent), 0666); err != nil {
				t.Error(err)
			}
		}
		if test.createFakeCredFile {
			if err := createTestFile(fakeCredFile); err != nil {
				t.Error(err)
			}
			defer func() {
				if err := os.Remove(fakeCredFile); err != nil && !os.IsNotExist(err) {
					t.Error(err)
				}
			}()

			originalCredFile, ok := os.LookupEnv(DefaultAzureCredentialFileEnv)
			if ok {
				defer os.Setenv(DefaultAzureCredentialFileEnv, originalCredFile)
			} else {
				defer os.Unsetenv(DefaultAzureCredentialFileEnv)
			}
			os.Setenv(DefaultAzureCredentialFileEnv, fakeCredFile)
		}
		kubeClient, err := util.GetKubeClient(test.kubeconfig, 25.0, 50, "")
		if err != nil {
			if !reflect.DeepEqual(err, test.expectedErr) && test.expectedErr != nil && !strings.Contains(err.Error(), test.expectedErr.Error()) {
				t.Errorf("desc: %s,\n input: %q, GetCloudProvider err: %v, expectedErr: %v", test.desc, test.kubeconfig, err, test.expectedErr)
			}
			continue
		}
		if test.setFederatedWorkloadIdentityEnv {
			t.Setenv("AZURE_TENANT_ID", test.tenantID)
			t.Setenv("AZURE_CLIENT_ID", test.aadClientID)
			t.Setenv("AZURE_FEDERATED_TOKEN_FILE", test.aadFederatedTokenFile)
		}

		cloud, err := GetCloudProvider(context.Background(), kubeClient, test.nodeID, "", "", test.userAgent, test.allowEmptyCloudConfig)
		if !reflect.DeepEqual(err, test.expectedErr) && test.expectedErr != nil && !strings.Contains(err.Error(), test.expectedErr.Error()) {
			t.Errorf("desc: %s,\n input: %q, GetCloudProvider err: %v, expectedErr: %v", test.desc, test.kubeconfig, err, test.expectedErr)
		}
		if cloud == nil {
			t.Errorf("return value of getCloudProvider should not be nil even there is error")
		} else {
			assert.Equal(t, cloud.Environment.StorageEndpointSuffix, storage.DefaultBaseURL)
			assert.Equal(t, cloud.UserAgent, test.userAgent)
			assert.Equal(t, cloud.AADFederatedTokenFile, test.aadFederatedTokenFile)
			assert.Equal(t, cloud.UseFederatedWorkloadIdentityExtension, test.useFederatedWorkloadIdentityExtension)
			assert.Equal(t, cloud.AADClientID, test.aadClientID)
			assert.Equal(t, cloud.TenantID, test.tenantID)
		}
	}
}

func TestGetKeyvaultToken(t *testing.T) {
	env := azure.Environment{
		ActiveDirectoryEndpoint: "unit-test",
		KeyVaultEndpoint:        "unit-test",
	}
	d := NewFakeDriver()
	d.cloud = &azureprovider.Cloud{}
	d.cloud.Environment = env
	_, err := d.getKeyvaultToken()
	expectedErr := fmt.Errorf("no credentials provided for Azure cloud provider")
	if !reflect.DeepEqual(expectedErr, err) {
		t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
	}
	d.cloud.AADClientID = "unit-test"
	d.cloud.AADClientSecret = "unit-test"
	_, err = d.getKeyvaultToken()
	assert.NoError(t, err)

}

func TestInitializeKvClient(t *testing.T) {
	env := azure.Environment{
		ActiveDirectoryEndpoint: "unit-test",
		KeyVaultEndpoint:        "unit-test",
	}
	d := NewFakeDriver()
	d.cloud = &azureprovider.Cloud{}
	d.cloud.Environment = env
	_, err := d.initializeKvClient()
	expectedErr := fmt.Errorf("no credentials provided for Azure cloud provider")
	if !reflect.DeepEqual(expectedErr, err) {
		t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
	}
	d.cloud.AADClientID = "unit-test"
	d.cloud.AADClientSecret = "unit-test"
	_, err = d.initializeKvClient()
	assert.NoError(t, err)
}

func TestGetKeyVaultSecretContent(t *testing.T) {
	env := azure.Environment{
		ActiveDirectoryEndpoint: "unit-test",
		KeyVaultEndpoint:        "unit-test",
	}
	d := NewFakeDriver()
	d.cloud = &azureprovider.Cloud{}
	d.cloud.Environment = env
	valueURL := "unit-test"
	secretName := "unit-test"
	secretVersion := "v1"
	_, err := d.getKeyVaultSecretContent(context.TODO(), valueURL, secretName, secretVersion)
	expectedErr := fmt.Errorf("failed to get keyvaultClient: no credentials provided for Azure cloud provider")
	if !strings.EqualFold(expectedErr.Error(), err.Error()) {
		t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
	}
	d.cloud.AADClientID = "unit-test"
	d.cloud.AADClientSecret = "unit-test"
	expectedErr = fmt.Errorf("get secret from vaultURL(unit-test), sercretName(unit-test), secretVersion(v1) failed with error: keyvault.BaseClient#GetSecret: Failure preparing request: StatusCode=0 -- Original Error: autorest: No scheme detected in URL unit-test")
	_, err = d.getKeyVaultSecretContent(context.TODO(), valueURL, secretName, secretVersion)
	if !strings.EqualFold(expectedErr.Error(), err.Error()) {
		t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
	}
}

func createTestFile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return nil
}

func TestUpdateSubnetServiceEndpoints(t *testing.T) {
	d := NewFakeDriver()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockSubnetClient := mocksubnetclient.NewMockInterface(ctrl)

	config := azureprovider.Config{
		ResourceGroup: "rg",
		Location:      "loc",
		VnetName:      "fake-vnet",
		SubnetName:    "fake-subnet",
	}

	d.cloud = &azureprovider.Cloud{
		SubnetsClient: mockSubnetClient,
		Config:        config,
	}
	ctx := context.TODO()

	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "[fail] no subnet",
			testFunc: func(t *testing.T) {
				retErr := retry.NewError(false, fmt.Errorf("the subnet does not exist"))
				mockSubnetClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(network.Subnet{}, retErr).Times(1)
				expectedErr := fmt.Errorf("failed to get the subnet %s under vnet %s: %v", config.SubnetName, config.VnetName, retErr)
				err := d.updateSubnetServiceEndpoints(ctx, "", "", "")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("Unexpected error: %v", err)
				}
			},
		},
		{
			name: "[success] subnetPropertiesFormat is nil",
			testFunc: func(t *testing.T) {
				mockSubnetClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(network.Subnet{}, nil).Times(1)
				mockSubnetClient.EXPECT().CreateOrUpdate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)

				err := d.updateSubnetServiceEndpoints(ctx, "", "", "")
				if !reflect.DeepEqual(err, nil) {
					t.Errorf("Unexpected error: %v", err)
				}
			},
		},
		{
			name: "[success] ServiceEndpoints is nil",
			testFunc: func(t *testing.T) {
				fakeSubnet := network.Subnet{
					SubnetPropertiesFormat: &network.SubnetPropertiesFormat{},
				}

				mockSubnetClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(fakeSubnet, nil).Times(1)
				mockSubnetClient.EXPECT().CreateOrUpdate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)

				err := d.updateSubnetServiceEndpoints(ctx, "", "", "")
				if !reflect.DeepEqual(err, nil) {
					t.Errorf("Unexpected error: %v", err)
				}
			},
		},
		{
			name: "[success] storageService does not exists",
			testFunc: func(t *testing.T) {
				fakeSubnet := network.Subnet{
					SubnetPropertiesFormat: &network.SubnetPropertiesFormat{
						ServiceEndpoints: &[]network.ServiceEndpointPropertiesFormat{},
					},
				}

				mockSubnetClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(fakeSubnet, nil).Times(1)
				mockSubnetClient.EXPECT().CreateOrUpdate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)

				err := d.updateSubnetServiceEndpoints(ctx, "", "", "")
				if !reflect.DeepEqual(err, nil) {
					t.Errorf("Unexpected error: %v", err)
				}
			},
		},
		{
			name: "[success] storageService already exists",
			testFunc: func(t *testing.T) {
				fakeSubnet := network.Subnet{
					SubnetPropertiesFormat: &network.SubnetPropertiesFormat{
						ServiceEndpoints: &[]network.ServiceEndpointPropertiesFormat{
							{
								Service: &storageService,
							},
						},
					},
				}

				mockSubnetClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(fakeSubnet, nil).Times(1)

				err := d.updateSubnetServiceEndpoints(ctx, "", "", "")
				if !reflect.DeepEqual(err, nil) {
					t.Errorf("Unexpected error: %v", err)
				}
			},
		},
		{
			name: "[fail] SubnetsClient is nil",
			testFunc: func(t *testing.T) {
				d.cloud.SubnetsClient = nil
				expectedErr := fmt.Errorf("SubnetsClient is nil")
				err := d.updateSubnetServiceEndpoints(ctx, "", "", "")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("Unexpected error: %v", err)
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func TestGetStorageEndPointSuffix(t *testing.T) {
	d := NewFakeDriver()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name           string
		cloud          *azureprovider.Cloud
		expectedSuffix string
	}{
		{
			name:           "nil cloud",
			cloud:          nil,
			expectedSuffix: "core.windows.net",
		},
		{
			name:           "empty cloud",
			cloud:          &azureprovider.Cloud{},
			expectedSuffix: "core.windows.net",
		},
		{
			name: "cloud with storage endpoint suffix",
			cloud: &azureprovider.Cloud{
				Environment: azure.Environment{
					StorageEndpointSuffix: "suffix",
				},
			},
			expectedSuffix: "suffix",
		},
		{
			name: "public cloud",
			cloud: &azureprovider.Cloud{
				Environment: azure.PublicCloud,
			},
			expectedSuffix: "core.windows.net",
		},
		{
			name: "china cloud",
			cloud: &azureprovider.Cloud{
				Environment: azure.ChinaCloud,
			},
			expectedSuffix: "core.chinacloudapi.cn",
		},
	}

	for _, test := range tests {
		d.cloud = test.cloud
		suffix := d.getStorageEndPointSuffix()
		assert.Equal(t, test.expectedSuffix, suffix, test.name)
	}
}

func TestGetCloudEnvironment(t *testing.T) {
	d := NewFakeDriver()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name        string
		cloud       *azureprovider.Cloud
		expectedEnv azure.Environment
	}{
		{
			name:        "nil cloud",
			cloud:       nil,
			expectedEnv: azure.PublicCloud,
		},
		{
			name: "cloud with environment",
			cloud: &azureprovider.Cloud{
				Environment: azure.ChinaCloud,
			},
			expectedEnv: azure.ChinaCloud,
		},
	}

	for _, test := range tests {
		d.cloud = test.cloud
		env := d.getCloudEnvironment()
		assert.Equal(t, test.expectedEnv, env, test.name)
	}
}
