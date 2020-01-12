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

package blobfuse

import (
	"reflect"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

func TestGetNodePublishMountOptions(t *testing.T) {
	tests := []struct {
		request  *csi.NodePublishVolumeRequest
		expected []string
	}{
		{
			request: &csi.NodePublishVolumeRequest{
				VolumeCapability: &csi.VolumeCapability{},
			},
			expected: []string{"bind"},
		},
		{
			request: &csi.NodePublishVolumeRequest{
				VolumeCapability: &csi.VolumeCapability{},
				Readonly:         true,
			},
			expected: []string{"bind", "ro"},
		},
		{
			request: &csi.NodePublishVolumeRequest{
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							MountFlags: []string{"rw"},
						},
					},
				},
			},
			expected: []string{"bind", "rw"},
		},
	}

	for _, test := range tests {
		result := getNodePublishMountOptions(test.request)
		if !reflect.DeepEqual(result, test.expected) {
			t.Errorf("input: %v, getFStype result: %v, expected: %v", test.request, result, test.expected)
		}
	}
}
