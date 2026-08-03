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

func TestValidateVolumeAttributeKeys(t *testing.T) {
	tests := []struct {
		name    string
		input   map[string]string
		wantErr bool
		wantLen int
	}{
		{
			name:    "nil map",
			input:   nil,
			wantErr: false,
			wantLen: 0,
		},
		{
			name:    "empty map",
			input:   map[string]string{},
			wantErr: false,
			wantLen: 0,
		},
		{
			name:    "no collisions",
			input:   map[string]string{"clientid": "abc", "tenantid": "def"},
			wantErr: false,
			wantLen: 2,
		},
		{
			name: "case-colliding keys with different values should error",
			input: map[string]string{
				"clientID": "value-a",
				"ClientID": "value-b",
			},
			wantErr: true,
		},
		{
			name: "case-colliding keys with same values should pass",
			input: map[string]string{
				"clientID": "same",
				"ClientID": "same",
			},
			wantErr: false,
			wantLen: 2,
		},
		{
			name: "mixed case keys no collision",
			input: map[string]string{
				"StorageAccount": "myaccount",
				"containerName":  "mycontainer",
				"protocol":       "fuse2",
			},
			wantErr: false,
			wantLen: 3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ValidateVolumeAttributeKeys(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if tc.input == nil {
				if result != nil {
					t.Errorf("expected nil result for nil input")
				}
				return
			}
			if len(result) != tc.wantLen {
				t.Errorf("expected %d keys, got %d", tc.wantLen, len(result))
			}
		})
	}
}

func TestValidateContainerNameAdditional(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid", input: "mycontainer", wantErr: false},
		{name: "valid with hyphens", input: "my-container-123", wantErr: false},
		{name: "empty allowed", input: "", wantErr: false},
		{name: "too short", input: "ab", wantErr: true},
		{name: "consecutive hyphens", input: "my--container", wantErr: true},
		{name: "contains spaces", input: "abc def", wantErr: true},
		{name: "contains tab", input: "abc\tdef", wantErr: true},
		{name: "uppercase", input: "MyContainer", wantErr: true},
		{name: "starts with hyphen", input: "-container", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateContainerName(tc.input)
			if tc.wantErr && err == nil {
				t.Errorf("expected error for %q", tc.input)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error for %q: %v", tc.input, err)
			}
		})
	}
}
