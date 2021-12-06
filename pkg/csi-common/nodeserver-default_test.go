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

package csicommon

import (
	"context"
	"reflect"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestNodeGetInfo(t *testing.T) {
	d := NewFakeDriver()

	ns := NewDefaultNodeServer(d)

	// Test valid request
	req := csi.NodeGetInfoRequest{}
	resp, err := ns.NodeGetInfo(context.Background(), &req)
	assert.NoError(t, err)
	assert.Equal(t, resp.GetNodeId(), fakeNodeID)
}

func TestNodeGetCapabilities(t *testing.T) {
	d := NewFakeDriver()

	ns := NewDefaultNodeServer(d)

	// Test valid request
	req := csi.NodeGetCapabilitiesRequest{}
	_, err := ns.NodeGetCapabilities(context.Background(), &req)
	assert.NoError(t, err)
}

func TestNodeStageVolume(t *testing.T) {
	d := NewFakeDriver()
	ns := NewDefaultNodeServer(d)
	req := csi.NodeStageVolumeRequest{}
	resp, err := ns.NodeStageVolume(context.Background(), &req)
	assert.Nil(t, resp)
	if !reflect.DeepEqual(err, status.Error(codes.Unimplemented, "")) {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestNodeUnstageVolume(t *testing.T) {
	d := NewFakeDriver()
	ns := NewDefaultNodeServer(d)
	req := csi.NodeUnstageVolumeRequest{}
	resp, err := ns.NodeUnstageVolume(context.Background(), &req)
	assert.Nil(t, resp)
	if !reflect.DeepEqual(err, status.Error(codes.Unimplemented, "")) {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestNodePublishVolume(t *testing.T) {
	d := NewFakeDriver()
	ns := NewDefaultNodeServer(d)
	req := csi.NodePublishVolumeRequest{}
	resp, err := ns.NodePublishVolume(context.Background(), &req)
	assert.Nil(t, resp)
	if !reflect.DeepEqual(err, status.Error(codes.Unimplemented, "")) {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestNodeUnpublishVolume(t *testing.T) {
	d := NewFakeDriver()
	ns := NewDefaultNodeServer(d)
	req := csi.NodeUnpublishVolumeRequest{}
	resp, err := ns.NodeUnpublishVolume(context.Background(), &req)
	assert.Nil(t, resp)
	if !reflect.DeepEqual(err, status.Error(codes.Unimplemented, "")) {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestNodeGetVolumeStats(t *testing.T) {
	d := NewFakeDriver()
	ns := NewDefaultNodeServer(d)
	req := csi.NodeGetVolumeStatsRequest{}
	resp, err := ns.NodeGetVolumeStats(context.Background(), &req)
	assert.Nil(t, resp)
	if !reflect.DeepEqual(err, status.Error(codes.Unimplemented, "")) {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestNodeExpandVolume(t *testing.T) {
	d := NewFakeDriver()
	ns := NewDefaultNodeServer(d)
	req := csi.NodeExpandVolumeRequest{}
	resp, err := ns.NodeExpandVolume(context.Background(), &req)
	assert.Nil(t, resp)
	if !reflect.DeepEqual(err, status.Error(codes.Unimplemented, "")) {
		t.Errorf("Unexpected error: %v", err)
	}
}
