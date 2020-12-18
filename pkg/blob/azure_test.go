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
	"io/ioutil"
	"os"
	"reflect"
	"testing"

	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/stretchr/testify/assert"

	azure2 "sigs.k8s.io/cloud-provider-azure/pkg/provider"
)

// TestGetCloudProvider tests the func GetCloudProvider().
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
		desc        string
		kubeconfig  string
		expectedErr error
	}{
		{
			desc:        "[failure] out of cluster, no kubeconfig, no credential file",
			kubeconfig:  "",
			expectedErr: fmt.Errorf("Failed to load config from file: %s, cloud not get azure cloud provider", DefaultCredFilePath),
		},
		{
			desc:        "[failure] out of cluster & in cluster, specify a non-exist kubeconfig, no credential file",
			kubeconfig:  "/tmp/non-exist.json",
			expectedErr: fmt.Errorf("Failed to load config from file: %s, cloud not get azure cloud provider", DefaultCredFilePath),
		},
		{
			desc:        "[failure] out of cluster & in cluster, specify a empty kubeconfig, no credential file",
			kubeconfig:  emptyKubeConfig,
			expectedErr: fmt.Errorf("failed to get KubeClient: invalid configuration: no configuration has been provided, try setting KUBERNETES_MASTER environment variable"),
		},
		{
			desc:        "[failure] out of cluster & in cluster, specify a fake kubeconfig, no credential file",
			kubeconfig:  fakeKubeConfig,
			expectedErr: fmt.Errorf("Failed to load config from file: %s, cloud not get azure cloud provider", DefaultCredFilePath),
		},
		{
			desc:        "[success] out of cluster & in cluster, no kubeconfig, a fake credential file",
			kubeconfig:  "",
			expectedErr: nil,
		},
	}

	for _, test := range tests {
		if test.desc == "[failure] out of cluster & in cluster, specify a fake kubeconfig, no credential file" {
			err := createTestFile(fakeKubeConfig)
			if err != nil {
				t.Error(err)
			}
			defer func() {
				if err := os.Remove(fakeKubeConfig); err != nil {
					t.Error(err)
				}
			}()

			if err := ioutil.WriteFile(fakeKubeConfig, []byte(fakeContent), 0666); err != nil {
				t.Error(err)
			}
		}
		if test.desc == "[success] out of cluster & in cluster, no kubeconfig, a fake credential file" {
			err := createTestFile(fakeCredFile)
			if err != nil {
				t.Error(err)
			}
			defer func() {
				if err := os.Remove(fakeCredFile); err != nil {
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
		_, err := GetCloudProvider(test.kubeconfig)
		if !reflect.DeepEqual(err, test.expectedErr) {
			t.Errorf("desc: %s,\n input: %q, GetCloudProvider err: %v, expectedErr: %v", test.desc, test.kubeconfig, err, test.expectedErr)
		}
	}
}

func TestGetServicePrincipalToken(t *testing.T) {
	env := azure.Environment{
		ActiveDirectoryEndpoint: "unit-test",
	}
	resource := "unit-test"
	d := NewFakeDriver()
	d.cloud = &azure2.Cloud{}
	_, err := d.getServicePrincipalToken(env, resource)
	expectedErr := fmt.Errorf("parameter 'clientID' cannot be empty")
	if !reflect.DeepEqual(expectedErr, err) {
		t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
	}
	d.cloud.AADClientID = "unit-test"
	d.cloud.AADClientSecret = "unit-test"
	_, err = d.getServicePrincipalToken(env, resource)
	assert.NoError(t, err)
}

func TestGetKeyvaultToken(t *testing.T) {
	env := azure.Environment{
		ActiveDirectoryEndpoint: "unit-test",
		KeyVaultEndpoint:        "unit-test",
	}
	d := NewFakeDriver()
	d.cloud = &azure2.Cloud{}
	d.cloud.Environment = env
	_, err := d.getKeyvaultToken()
	expectedErr := fmt.Errorf("parameter 'clientID' cannot be empty")
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
	d.cloud = &azure2.Cloud{}
	d.cloud.Environment = env
	_, err := d.initializeKvClient()
	expectedErr := fmt.Errorf("parameter 'clientID' cannot be empty")
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
	d.cloud = &azure2.Cloud{}
	d.cloud.Environment = env
	valueURL := "unit-test"
	secretName := "unit-test"
	secretVersion := "v1"
	_, err := d.getKeyVaultSecretContent(context.TODO(), valueURL, secretName, secretVersion)
	expectedErr := fmt.Errorf("failed to get keyvaultClient: parameter 'clientID' cannot be empty")
	if !reflect.DeepEqual(expectedErr, err) {
		t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
	}
	d.cloud.AADClientID = "unit-test"
	d.cloud.AADClientSecret = "unit-test"
	expectedErr = fmt.Errorf("failed to use vaultURL(unit-test), sercretName(unit-test), secretVersion(v1) to get secret: keyvault.BaseClient#GetSecret: Failure preparing request: StatusCode=0 -- Original Error: autorest: No scheme detected in URL unit-test")
	_, err = d.getKeyVaultSecretContent(context.TODO(), valueURL, secretName, secretVersion)
	if !reflect.DeepEqual(expectedErr, err) {
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
