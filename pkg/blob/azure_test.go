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
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	network "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v6"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"

	"sigs.k8s.io/blob-csi-driver/pkg/util"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/mock_azclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/subnetclient/mock_subnetclient"
	azureconfig "sigs.k8s.io/cloud-provider-azure/pkg/provider/config"
	"sigs.k8s.io/cloud-provider-azure/pkg/provider/storage"
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
    token: 2fef7f7c64127579b48d61434c44ad46d87793169ee6a4199af3ce16a3cf5be3371
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
			desc:                                  "[success] out of cluster, no kubeconfig, no credential file",
			nodeID:                                "",
			allowEmptyCloudConfig:                 true,
			aadFederatedTokenFile:                 "",
			useFederatedWorkloadIdentityExtension: false,
			aadClientID:                           "",
			tenantID:                              "",
			expectedErr:                           nil,
		},
		{
			desc:                                  "[linux][failure][disallowEmptyCloudConfig] out of cluster, no kubeconfig, no credential file",
			nodeID:                                "",
			allowEmptyCloudConfig:                 false,
			aadFederatedTokenFile:                 "",
			useFederatedWorkloadIdentityExtension: false,
			aadClientID:                           "",
			tenantID:                              "",
			expectedErr:                           syscall.ENOENT,
		},
		{
			desc:                                  "[windows][failure][disallowEmptyCloudConfig] out of cluster, no kubeconfig, no credential file",
			nodeID:                                "",
			allowEmptyCloudConfig:                 false,
			aadFederatedTokenFile:                 "",
			useFederatedWorkloadIdentityExtension: false,
			aadClientID:                           "",
			tenantID:                              "",
			expectedErr:                           syscall.ENOTDIR,
		},
		{
			desc:                                  "[success] out of cluster & in cluster, specify a fake kubeconfig, no credential file",
			createFakeKubeConfig:                  true,
			kubeconfig:                            fakeKubeConfig,
			nodeID:                                "",
			allowEmptyCloudConfig:                 true,
			aadFederatedTokenFile:                 "",
			useFederatedWorkloadIdentityExtension: false,
			aadClientID:                           "",
			tenantID:                              "",
			expectedErr:                           nil,
		},
		{
			desc:                                  "[success] out of cluster & in cluster, no kubeconfig, a fake credential file",
			createFakeCredFile:                    true,
			nodeID:                                "",
			userAgent:                             "useragent",
			allowEmptyCloudConfig:                 true,
			aadFederatedTokenFile:                 "",
			useFederatedWorkloadIdentityExtension: false,
			aadClientID:                           "",
			tenantID:                              "",
			expectedErr:                           nil,
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

	var kubeClient kubernetes.Interface
	for _, test := range tests {
		if strings.HasPrefix(test.desc, "[linux]") && runtime.GOOS != "linux" {
			continue
		}
		if strings.HasPrefix(test.desc, "[windows]") && runtime.GOOS != "windows" {
			continue
		}
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

			kubeClient, err = util.GetKubeClient(test.kubeconfig, 25.0, 50, "")
			if err != nil {
				t.Error(err)
			}
		} else {
			kubeClient = nil
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
		if test.setFederatedWorkloadIdentityEnv {
			t.Setenv("AZURE_TENANT_ID", test.tenantID)
			t.Setenv("AZURE_CLIENT_ID", test.aadClientID)
			t.Setenv("AZURE_FEDERATED_TOKEN_FILE", test.aadFederatedTokenFile)
		}

		cloud, err := GetCloudProvider(context.Background(), kubeClient, test.nodeID, "", "", test.userAgent, test.allowEmptyCloudConfig)
		assert.ErrorIs(t, err, test.expectedErr)

		if cloud == nil {
			t.Errorf("return value of getCloudProvider should not be nil even there is error")
		} else {
			assert.Equal(t, cloud.Environment.StorageEndpointSuffix, "core.windows.net")
			assert.Equal(t, cloud.UserAgent, test.userAgent)
			assert.Equal(t, cloud.AADFederatedTokenFile, test.aadFederatedTokenFile)
			assert.Equal(t, cloud.UseFederatedWorkloadIdentityExtension, test.useFederatedWorkloadIdentityExtension)
			assert.Equal(t, cloud.AADClientID, test.aadClientID)
			assert.Equal(t, cloud.TenantID, test.tenantID)
		}
	}
}

func TestGetKeyVaultSecretContent(t *testing.T) {
	d := NewFakeDriver()
	var err error
	valueURL := "unit-test"
	secretName := "unit-test"
	secretVersion := "v1"
	_, err = d.getKeyVaultSecretContent(context.TODO(), valueURL, secretName, secretVersion)
	expectedErr := fmt.Errorf("no Host in request URL")
	if !strings.Contains(err.Error(), expectedErr.Error()) {
		t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
	}
	d.cloud.AADClientID = "unit-test"
	d.cloud.AADClientSecret = "unit-test"
	expectedErr = fmt.Errorf("invalid tenantID")
	_, err = d.getKeyVaultSecretContent(context.TODO(), valueURL, secretName, secretVersion)
	if !strings.Contains(err.Error(), expectedErr.Error()) {
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
	mockSubnetClient := mock_subnetclient.NewMockInterface(ctrl)
	networkClientFactory := mock_azclient.NewMockClientFactory(ctrl)
	networkClientFactory.EXPECT().GetSubnetClient().Return(mockSubnetClient).AnyTimes()
	config := azureconfig.Config{
		ResourceGroup: "rg",
		Location:      "loc",
		VnetName:      "fake-vnet",
		SubnetName:    "fake-subnet",
	}

	d.cloud = &storage.AccountRepo{
		Config:               config,
		NetworkClientFactory: networkClientFactory,
	}
	d.networkClientFactory = networkClientFactory
	ctx := context.TODO()

	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "[fail] subnet name is nil",
			testFunc: func(t *testing.T) {
				mockSubnetClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(&network.Subnet{}, nil).Times(1)
				mockSubnetClient.EXPECT().CreateOrUpdate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)

				_, err := d.updateSubnetServiceEndpoints(ctx, "", "", "subnetname")
				expectedErr := fmt.Errorf("subnet name is nil")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("Unexpected error: %v", err)
				}
			},
		},
		{
			name: "[success] ServiceEndpoints is nil",
			testFunc: func(t *testing.T) {
				fakeSubnet := &network.Subnet{
					Properties: &network.SubnetPropertiesFormat{},
					Name:       ptr.To("subnetName"),
				}

				mockSubnetClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(fakeSubnet, nil).Times(1)
				_, err := d.updateSubnetServiceEndpoints(ctx, "", "", "subnetname")
				if !reflect.DeepEqual(err, nil) {
					t.Errorf("Unexpected error: %v", err)
				}
			},
		},
		{
			name: "[success] storageService does not exists",
			testFunc: func(t *testing.T) {
				fakeSubnet := &network.Subnet{
					Properties: &network.SubnetPropertiesFormat{
						ServiceEndpoints: []*network.ServiceEndpointPropertiesFormat{},
					},
					Name: ptr.To("subnetName"),
				}

				mockSubnetClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(fakeSubnet, nil).AnyTimes()

				_, err := d.updateSubnetServiceEndpoints(ctx, "", "", "subnetname")
				if !reflect.DeepEqual(err, nil) {
					t.Errorf("Unexpected error: %v", err)
				}
			},
		},
		{
			name: "[success] storageService already exists",
			testFunc: func(t *testing.T) {
				fakeSubnet := &network.Subnet{
					Properties: &network.SubnetPropertiesFormat{
						ServiceEndpoints: []*network.ServiceEndpointPropertiesFormat{
							{
								Service: &storageService,
							},
						},
					},
					Name: ptr.To("subnetName"),
				}

				mockSubnetClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(fakeSubnet, nil).AnyTimes()

				_, err := d.updateSubnetServiceEndpoints(ctx, "", "", "subnetname")
				if !reflect.DeepEqual(err, nil) {
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
		cloud          *storage.AccountRepo
		expectedSuffix string
	}{
		{
			name:           "nil cloud",
			cloud:          nil,
			expectedSuffix: "core.windows.net",
		},
		{
			name:           "empty cloud",
			cloud:          &storage.AccountRepo{},
			expectedSuffix: "core.windows.net",
		},
		{
			name: "cloud with storage endpoint suffix",
			cloud: &storage.AccountRepo{
				Environment: &azclient.Environment{
					StorageEndpointSuffix: "suffix",
				},
			},
			expectedSuffix: "suffix",
		},
		{
			name: "public cloud",
			cloud: &storage.AccountRepo{
				Environment: &azclient.Environment{
					StorageEndpointSuffix: "core.windows.net",
				},
			},
			expectedSuffix: "core.windows.net",
		},
		{
			name: "china cloud",
			cloud: &storage.AccountRepo{
				Environment: &azclient.Environment{
					StorageEndpointSuffix: "core.chinacloudapi.cn",
				},
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

func TestGetBackOff(t *testing.T) {
	tests := []struct {
		desc     string
		config   azureconfig.Config
		expected wait.Backoff
	}{
		{
			desc: "default backoff",
			config: azureconfig.Config{
				AzureClientConfig: azureconfig.AzureClientConfig{
					CloudProviderBackoffRetries:  0,
					CloudProviderBackoffDuration: 5,
				},
				CloudProviderBackoffExponent: 2,
				CloudProviderBackoffJitter:   1,
			},
			expected: wait.Backoff{
				Steps:    1,
				Duration: 5 * time.Second,
				Factor:   2,
				Jitter:   1,
			},
		},
		{
			desc: "backoff with retries > 1",
			config: azureconfig.Config{
				AzureClientConfig: azureconfig.AzureClientConfig{
					CloudProviderBackoffRetries:  3,
					CloudProviderBackoffDuration: 4,
				},
				CloudProviderBackoffExponent: 2,
				CloudProviderBackoffJitter:   1,
			},
			expected: wait.Backoff{
				Steps:    3,
				Duration: 4 * time.Second,
				Factor:   2,
				Jitter:   1,
			},
		},
	}

	for _, test := range tests {
		result := getBackOff(test.config)
		if !reflect.DeepEqual(result, test.expected) {
			t.Errorf("desc: (%s), input: config(%v), getBackOff returned with backoff(%v), not equal to expected(%v)",
				test.desc, test.config, result, test.expected)
		}
	}
}
