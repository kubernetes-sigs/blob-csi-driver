/*
Copyright 2026 The Kubernetes Authors.

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
	"testing"
)

func TestNormalizeInlineVolumeServerValidation(t *testing.T) {
	tests := []struct {
		name     string
		server   string
		account  string
		suffix   string
		expected bool
	}{
		{
			name:     "public blob endpoint",
			server:   "account.blob.core.windows.net",
			account:  "account",
			suffix:   "core.windows.net",
			expected: true,
		},
		{
			name:     "HTTPS blob endpoint with trailing slash",
			server:   "https://account.blob.core.windows.net/",
			account:  "account",
			suffix:   "core.windows.net",
			expected: true,
		},
		{
			name:     "public DFS endpoint",
			server:   "account.dfs.core.windows.net",
			account:  "account",
			suffix:   "core.windows.net",
			expected: true,
		},
		{
			name:     "private link blob endpoint",
			server:   "account.privatelink.blob.core.windows.net",
			account:  "account",
			suffix:   "core.windows.net",
			expected: true,
		},
		{
			name:     "private link DFS endpoint",
			server:   "account.privatelink.dfs.core.windows.net",
			account:  "account",
			suffix:   "core.windows.net",
			expected: true,
		},
		{
			name:     "sovereign cloud endpoint",
			server:   "account.blob.core.usgovcloudapi.net",
			account:  "account",
			suffix:   "core.usgovcloudapi.net",
			expected: true,
		},
		{
			name:     "sovereign cloud DFS endpoint",
			server:   "account.dfs.core.chinacloudapi.cn",
			account:  "account",
			suffix:   "core.chinacloudapi.cn",
			expected: true,
		},
		{
			name:     "case insensitive hostname with trailing dot",
			server:   "ACCOUNT.BLOB.CORE.WINDOWS.NET.",
			account:  "account",
			suffix:   "core.windows.net",
			expected: true,
		},
		{
			name:    "empty server",
			server:  "",
			account: "account",
			suffix:  "core.windows.net",
		},
		{
			name:    "leading whitespace",
			server:  " account.blob.core.windows.net",
			account: "account",
			suffix:  "core.windows.net",
		},
		{
			name:    "trailing whitespace",
			server:  "account.blob.core.windows.net ",
			account: "account",
			suffix:  "core.windows.net",
		},
		{
			name:    "malformed URL",
			server:  "https://[account.blob.core.windows.net",
			account: "account",
			suffix:  "core.windows.net",
		},
		{
			name:    "IPv4 address",
			server:  "192.0.2.10",
			account: "account",
			suffix:  "core.windows.net",
		},
		{
			name:    "IPv6 address",
			server:  "[2001:db8::1]",
			account: "account",
			suffix:  "core.windows.net",
		},
		{
			name:    "HTTP endpoint",
			server:  "http://account.blob.core.windows.net",
			account: "account",
			suffix:  "core.windows.net",
		},
		{
			name:    "non-HTTPS endpoint",
			server:  "ftp://account.blob.core.windows.net",
			account: "account",
			suffix:  "core.windows.net",
		},
		{
			name:    "different storage account",
			server:  "other.blob.core.windows.net",
			account: "account",
			suffix:  "core.windows.net",
		},
		{
			name:    "arbitrary subdomain",
			server:  "account.blob.core.windows.net.example.com",
			account: "account",
			suffix:  "core.windows.net",
		},
		{
			name:    "userinfo",
			server:  "https://user@account.blob.core.windows.net",
			account: "account",
			suffix:  "core.windows.net",
		},
		{
			name:    "explicit port",
			server:  "https://account.blob.core.windows.net:443",
			account: "account",
			suffix:  "core.windows.net",
		},
		{
			name:    "path",
			server:  "https://account.blob.core.windows.net/container",
			account: "account",
			suffix:  "core.windows.net",
		},
		{
			name:    "query string",
			server:  "https://account.blob.core.windows.net/?comp=list",
			account: "account",
			suffix:  "core.windows.net",
		},
		{
			name:    "fragment",
			server:  "https://account.blob.core.windows.net/#fragment",
			account: "account",
			suffix:  "core.windows.net",
		},
		{
			name:    "hostname missing",
			server:  "https://",
			account: "account",
			suffix:  "core.windows.net",
		},
		{
			name:    "storage account missing",
			server:  "account.blob.core.windows.net",
			account: "",
			suffix:  "core.windows.net",
		},
		{
			name:    "endpoint suffix missing",
			server:  "account.blob.core.windows.net",
			account: "account",
			suffix:  "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := normalizeInlineVolumeServer(test.server, test.account, test.suffix)
			if test.expected && err != nil {
				t.Fatalf("normalizeInlineVolumeServer() returned unexpected error: %v", err)
			}
			if !test.expected && err == nil {
				t.Fatal("normalizeInlineVolumeServer() returned nil, expected an error")
			}
		})
	}
}

func TestNormalizeInlineVolumeServer(t *testing.T) {
	server, err := normalizeInlineVolumeServer(
		"https://ACCOUNT.blob.core.windows.net/",
		"account",
		"core.windows.net",
	)
	if err != nil {
		t.Fatalf("normalizeInlineVolumeServer() returned unexpected error: %v", err)
	}
	if server != "account.blob.core.windows.net" {
		t.Fatalf("normalizeInlineVolumeServer() returned %q, expected hostname only", server)
	}
}

func TestNormalizeInlineVolumeServerWithAzureDNSZoneEndpoint(t *testing.T) {
	tests := []string{
		"account.z1.blob.storage.azure.net",
		"account.z9.dfs.storage.azure.net",
		"account.z10.blob.storage.azure.net",
		"account.z22.blob.storage.azure.net",
		"account.z22.dfs.storage.azure.net",
		"account.z99.blob.storage.azure.net",
		"account.z22.privatelink.blob.storage.azure.net",
		"account.z22.privatelink.dfs.storage.azure.net",
		"account-secondary.z22.privatelink.blob.storage.azure.net",
		"account-secondary.z22.privatelink.dfs.storage.azure.net",
	}
	for _, expected := range tests {
		t.Run(expected, func(t *testing.T) {
			server, err := normalizeInlineVolumeServer(
				expected,
				"account",
				"core.windows.net",
			)
			if err != nil {
				t.Fatalf("normalizeInlineVolumeServer() returned unexpected error: %v", err)
			}
			if server != expected {
				t.Fatalf("normalizeInlineVolumeServer() returned %q, expected %q", server, expected)
			}
		})
	}
}

func TestNormalizeInlineVolumeServerRejectsInvalidAzureDNSZoneEndpoints(t *testing.T) {
	tests := []string{
		"other.z22.blob.storage.azure.net",
		"account.z0.blob.storage.azure.net",
		"account.z00.blob.storage.azure.net",
		"account.z01.dfs.storage.azure.net",
		"account.z100.blob.storage.azure.net",
		"account.z22.file.storage.azure.net",
		"account.z22.privatelink.file.storage.azure.net",
		"account.z22.other.blob.storage.azure.net",
		"account.z22.blob.storage.azure.net.example.com",
	}
	for _, server := range tests {
		t.Run(server, func(t *testing.T) {
			_, err := normalizeInlineVolumeServer(server, "account", "core.windows.net")
			if err == nil {
				t.Fatal("normalizeInlineVolumeServer() returned nil, expected an error")
			}
		})
	}

	_, err := normalizeInlineVolumeServer(
		"account.z22.blob.storage.azure.net",
		"account",
		"core.usgovcloudapi.net",
	)
	if err == nil {
		t.Fatal("normalizeInlineVolumeServer() accepted a public Azure DNS-zone endpoint for a sovereign cloud")
	}
}
