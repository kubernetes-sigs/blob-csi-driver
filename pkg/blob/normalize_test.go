/*
Copyright 2026 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package blob

import (
	"testing"
)

func TestNormalizeVolumeAttributes(t *testing.T) {
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
			name: "case-colliding keys with same values should deduplicate",
			input: map[string]string{
				"clientID": "same",
				"ClientID": "same",
			},
			wantErr: false,
			wantLen: 1,
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
			result, err := NormalizeVolumeAttributes(tc.input)
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
			for k := range result {
				for _, c := range k {
					if c >= 'A' && c <= 'Z' {
						t.Errorf("key %q contains uppercase characters", k)
						break
					}
				}
			}
		})
	}
}
