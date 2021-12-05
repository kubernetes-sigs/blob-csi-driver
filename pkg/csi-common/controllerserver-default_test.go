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

func TestValidateVolumeCapabilities(t *testing.T) {
	d := NewFakeDriver()
	d.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY,
		csi.VolumeCapability_AccessMode_SINGLE_NODE_SINGLE_WRITER,
		csi.VolumeCapability_AccessMode_SINGLE_NODE_MULTI_WRITER,
		csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
	})

	capability := []*csi.VolumeCapability{
		{AccessMode: NewVolumeCapabilityAccessMode(csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER)},
		{AccessMode: NewVolumeCapabilityAccessMode(csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY)},
		{AccessMode: NewVolumeCapabilityAccessMode(csi.VolumeCapability_AccessMode_SINGLE_NODE_SINGLE_WRITER)},
		{AccessMode: NewVolumeCapabilityAccessMode(csi.VolumeCapability_AccessMode_SINGLE_NODE_MULTI_WRITER)},
		{AccessMode: NewVolumeCapabilityAccessMode(csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY)},
	}
	capabilityDisjoint := []*csi.VolumeCapability{
		{AccessMode: NewVolumeCapabilityAccessMode(csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER)},
	}

	cs := NewDefaultControllerServer(d)

	// Test when there are common capabilities
	req := csi.ValidateVolumeCapabilitiesRequest{VolumeCapabilities: capability}
	resp, err := cs.ValidateVolumeCapabilities(context.Background(), &req)
	assert.NoError(t, err)
	assert.Equal(t, resp.XXX_sizecache, int32(0))

	// Test when there are no common capabilities
	req = csi.ValidateVolumeCapabilitiesRequest{VolumeCapabilities: capabilityDisjoint}
	resp, err = cs.ValidateVolumeCapabilities(context.Background(), &req)
	assert.NotNil(t, resp)
	assert.Error(t, err)
}

func TestControllerGetCapabilities(t *testing.T) {
	d := NewFakeDriver()
	cs := NewDefaultControllerServer(d)

	// Test valid request
	req := csi.ControllerGetCapabilitiesRequest{}
	resp, err := cs.ControllerGetCapabilities(context.Background(), &req)
	assert.NoError(t, err)
	assert.Equal(t, resp.XXX_sizecache, int32(0))
}

func TestCreateVolume(t *testing.T) {
	d := NewFakeDriver()
	cs := NewDefaultControllerServer(d)
	req := csi.CreateVolumeRequest{}
	resp, err := cs.CreateVolume(context.Background(), &req)
	assert.Nil(t, resp)
	if !reflect.DeepEqual(err, status.Error(codes.Unimplemented, "")) {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestDeleteVolume(t *testing.T) {
	d := NewFakeDriver()
	cs := NewDefaultControllerServer(d)
	req := csi.DeleteVolumeRequest{}
	resp, err := cs.DeleteVolume(context.Background(), &req)
	assert.Nil(t, resp)
	if !reflect.DeepEqual(err, status.Error(codes.Unimplemented, "")) {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestControllerPublishVolume(t *testing.T) {
	d := NewFakeDriver()
	cs := NewDefaultControllerServer(d)
	req := csi.ControllerPublishVolumeRequest{}
	resp, err := cs.ControllerPublishVolume(context.Background(), &req)
	assert.Nil(t, resp)
	if !reflect.DeepEqual(err, status.Error(codes.Unimplemented, "")) {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestControllerUnpublishVolume(t *testing.T) {
	d := NewFakeDriver()
	cs := NewDefaultControllerServer(d)
	req := csi.ControllerUnpublishVolumeRequest{}
	resp, err := cs.ControllerUnpublishVolume(context.Background(), &req)
	assert.Nil(t, resp)
	if !reflect.DeepEqual(err, status.Error(codes.Unimplemented, "")) {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestGetCapacity(t *testing.T) {
	d := NewFakeDriver()
	cs := NewDefaultControllerServer(d)
	req := csi.GetCapacityRequest{}
	resp, err := cs.GetCapacity(context.Background(), &req)
	assert.Nil(t, resp)
	if !reflect.DeepEqual(err, status.Error(codes.Unimplemented, "")) {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestListVolumes(t *testing.T) {
	d := NewFakeDriver()
	cs := NewDefaultControllerServer(d)
	req := csi.ListVolumesRequest{}
	resp, err := cs.ListVolumes(context.Background(), &req)
	assert.Nil(t, resp)
	if !reflect.DeepEqual(err, status.Error(codes.Unimplemented, "")) {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestCreateSnapshot(t *testing.T) {
	d := NewFakeDriver()
	cs := NewDefaultControllerServer(d)
	req := csi.CreateSnapshotRequest{}
	resp, err := cs.CreateSnapshot(context.Background(), &req)
	assert.Nil(t, resp)
	if !reflect.DeepEqual(err, status.Error(codes.Unimplemented, "")) {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestDeleteSnapshot(t *testing.T) {
	d := NewFakeDriver()
	cs := NewDefaultControllerServer(d)
	req := csi.DeleteSnapshotRequest{}
	resp, err := cs.DeleteSnapshot(context.Background(), &req)
	assert.Nil(t, resp)
	if !reflect.DeepEqual(err, status.Error(codes.Unimplemented, "")) {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestListSnapshots(t *testing.T) {
	d := NewFakeDriver()
	cs := NewDefaultControllerServer(d)
	req := csi.ListSnapshotsRequest{}
	resp, err := cs.ListSnapshots(context.Background(), &req)
	assert.Nil(t, resp)
	if !reflect.DeepEqual(err, status.Error(codes.Unimplemented, "")) {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestControllerExpandVolume(t *testing.T) {
	d := NewFakeDriver()
	cs := NewDefaultControllerServer(d)
	req := csi.ControllerExpandVolumeRequest{}
	resp, err := cs.ControllerExpandVolume(context.Background(), &req)
	assert.Nil(t, resp)
	if !reflect.DeepEqual(err, status.Error(codes.Unimplemented, "")) {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestControllerGetVolume(t *testing.T) {
	d := NewFakeDriver()
	cs := NewDefaultControllerServer(d)
	req := csi.ControllerGetVolumeRequest{}
	resp, err := cs.ControllerGetVolume(context.Background(), &req)
	assert.Nil(t, resp)
	if !reflect.DeepEqual(err, status.Error(codes.Unimplemented, "")) {
		t.Errorf("Unexpected error: %v", err)
	}
}
