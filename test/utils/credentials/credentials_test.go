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

package credentials

import (
	"bytes"
	"os"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
)

const (
	testTenantID           = "test-tenant-id"
	testSubscriptionID     = "test-subscription-id"
	testAadClientID        = "test-aad-client-id"
	testAadClientSecret    = "test-aad-client-secret"
	testResourceGroup      = "test-resource-group"
	testLocation           = "test-location"
	testFederatedTokenFile = "test-federated-token-file"
)

func TestCreateAzureCredentialFileOnAzurePublicCloud(t *testing.T) {
	t.Run("WithEnvironmentVariables", func(t *testing.T) {
		os.Setenv(tenantIDEnvVar, testTenantID)
		os.Setenv(subscriptionIDEnvVar, testSubscriptionID)
		os.Setenv(aadClientIDEnvVar, testAadClientID)
		os.Setenv(aadClientSecretEnvVar, testAadClientSecret)
		os.Setenv(resourceGroupEnvVar, testResourceGroup)
		os.Setenv(locationEnvVar, testLocation)
		os.Setenv(federatedTokenFileVar, testFederatedTokenFile)
		withEnvironmentVariables(t)
	})
}

func TestCreateAzureCredentialFileOnAzureStackCloud(t *testing.T) {
	t.Run("WithEnvironmentVariables", func(t *testing.T) {
		os.Setenv(cloudNameEnvVar, "AzureStackCloud")
		os.Setenv(tenantIDEnvVar, testTenantID)
		os.Setenv(subscriptionIDEnvVar, testSubscriptionID)
		os.Setenv(aadClientIDEnvVar, testAadClientID)
		os.Setenv(aadClientSecretEnvVar, testAadClientSecret)
		os.Setenv(resourceGroupEnvVar, testResourceGroup)
		os.Setenv(locationEnvVar, testLocation)
		os.Setenv(federatedTokenFileVar, testFederatedTokenFile)
		withEnvironmentVariables(t)
	})
}

func withEnvironmentVariables(t *testing.T) {
	creds, err := CreateAzureCredentialFile()
	defer func() {
		err := DeleteAzureCredentialFile()
		assert.NoError(t, err)
	}()
	assert.NoError(t, err)

	parsedCreds, err := ParseAzureCredentialFile()
	assert.NoError(t, err)
	assert.Equal(t, creds, parsedCreds)

	var cloud string
	cloud = os.Getenv(cloudNameEnvVar)
	if cloud == "" {
		cloud = AzurePublicCloud
	}

	assert.Equal(t, cloud, creds.Cloud)
	assert.Equal(t, testTenantID, creds.TenantID)
	assert.Equal(t, testSubscriptionID, creds.SubscriptionID)
	assert.Equal(t, testAadClientID, creds.AADClientID)
	assert.Equal(t, testAadClientSecret, creds.AADClientSecret)
	assert.Equal(t, testResourceGroup, creds.ResourceGroup)
	assert.Equal(t, testLocation, creds.Location)
	assert.Equal(t, testFederatedTokenFile, creds.AADFederatedTokenFile)

	azureCredentialFileContent, err := os.ReadFile(TempAzureCredentialFilePath)
	assert.NoError(t, err)

	const expectedAzureCredentialFileContent = `
	{
		"cloud": "{{.Cloud}}",
		"tenantId": "test-tenant-id",
		"subscriptionId": "test-subscription-id",
		"aadClientId": "test-aad-client-id",
		"aadClientSecret": "test-aad-client-secret",
		"aadFederatedTokenFile": "test-federated-token-file",
		"resourceGroup": "test-resource-group",
		"location": "test-location"
	}
	`
	tmpl := template.New("expectedAzureCredentialFileContent")
	tmpl, err = tmpl.Parse(expectedAzureCredentialFileContent)
	assert.NoError(t, err)

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, struct {
		Cloud string
	}{
		cloud,
	})
	assert.NoError(t, err)
	assert.JSONEq(t, buf.String(), string(azureCredentialFileContent))
}

func TestDeleteAzureCredentialFileNotExists(t *testing.T) {
	// Ensure the file doesn't exist first
	os.Remove(TempAzureCredentialFilePath)
	
	// Trying to delete a non-existent file should not error
	err := DeleteAzureCredentialFile()
	assert.NoError(t, err)
}

func TestParseAzureCredentialFileNotExists(t *testing.T) {
	// Ensure the file doesn't exist first
	os.Remove(TempAzureCredentialFilePath)
	
	// Trying to parse a non-existent file should error
	_, err := ParseAzureCredentialFile()
	assert.Error(t, err)
}

func TestCreateAzureCredentialFileWithMissingEnvVars(t *testing.T) {
	// Clear all environment variables
	os.Unsetenv(tenantIDEnvVar)
	os.Unsetenv(subscriptionIDEnvVar)
	os.Unsetenv(aadClientIDEnvVar)
	os.Unsetenv(aadClientSecretEnvVar)
	os.Unsetenv(resourceGroupEnvVar)
	os.Unsetenv(locationEnvVar)
	os.Unsetenv(federatedTokenFileVar)
	os.Unsetenv(cloudNameEnvVar)
	
	creds, err := CreateAzureCredentialFile()
	defer func() {
		DeleteAzureCredentialFile()
	}()
	
	// Should error when required env vars are missing
	assert.Error(t, err)
	assert.Nil(t, creds)
}
