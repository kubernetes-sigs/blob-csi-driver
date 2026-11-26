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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/require"
	mount "k8s.io/mount-utils"
)

func TestWorkloadIdentityMountIsolation(t *testing.T) {
	d := NewFakeDriver()
	root := t.TempDir()
	mountToken := func(podUID, volumeID, mountName, token string) string {
		t.Helper()
		target := filepath.Join(root, "pods", podUID, "volumes", "kubernetes.io~csi", mountName, "mount")
		require.NoError(t, os.MkdirAll(target, 0750))
		attrib := map[string]string{
			clientIDField:            "shared-client",
			storageAccountField:      "sharedaccount",
			containerNameField:       "sharedcontainer",
			mountWithWITokenField:    "true",
			podUIDField:              podUID,
			serviceAccountTokenField: fmt.Sprintf(`{"api://AzureADTokenExchange":{"token":%q}}`, token),
		}
		_, _, _, _, env, err := d.getAuthEnv(context.Background(), volumeID, Fuse2, attrib, nil, target)
		require.NoError(t, err)
		for _, value := range env {
			if path, ok := strings.CutPrefix(value, "AZURE_OAUTH_TOKEN_FILE="); ok {
				return path
			}
		}
		t.Fatal("missing token-file environment variable")
		return ""
	}

	a := mountToken("pod-a", "shared-volume", "shared", "token-a")
	b := mountToken("pod-b", "shared-volume", "shared", "token-b")
	otherMount := mountToken("pod-a", "other-volume", "other", "token-other")
	require.NotEqual(t, a, b)
	require.NotEqual(t, a, otherMount)
	require.Equal(t, a, mountToken("pod-a", "shared-volume", "shared", "refreshed-a"))
	for path, expected := range map[string]string{a: "refreshed-a", b: "token-b", otherMount: "token-other"} {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, expected, string(data))
		info, err := os.Stat(path)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0600), info.Mode().Perm())
	}

	d.mounter = &mount.SafeFormatAndMount{Interface: mount.NewFakeMounter(nil)}
	_, err := d.NodeUnpublishVolume(context.Background(), &csi.NodeUnpublishVolumeRequest{
		VolumeId:   "shared-volume",
		TargetPath: filepath.Join(root, "pods", "pod-a", "volumes", "kubernetes.io~csi", "shared", "mount"),
	})
	require.NoError(t, err)
	_, err = os.Stat(a)
	require.ErrorIs(t, err, os.ErrNotExist)
	for _, path := range []string{b, otherMount} {
		_, err := os.Stat(path)
		require.NoError(t, err)
	}
	require.NoError(t, removeWorkloadIdentityToken("shared-volume",
		filepath.Join(root, "pods", "pod-a", "volumes", "kubernetes.io~csi", "shared", "mount")))
}

func TestWorkloadIdentityRequiresMatchingPodMount(t *testing.T) {
	target := filepath.Join(t.TempDir(), "pods", "pod-a", "volumes", "kubernetes.io~csi", "volume", "mount")
	for _, test := range []struct {
		name   string
		podUID string
		target string
	}{
		{name: "missing UID", target: target},
		{name: "different UID", podUID: "pod-b", target: target},
		{name: "path traversal UID", podUID: "../pod-a", target: target},
		{name: "relative target", podUID: "pod-a", target: "pods/pod-a/volumes/volume/mount"},
		{name: "staging target", podUID: "pod-a", target: filepath.Join(t.TempDir(), "globalmount")},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := podWorkloadIdentityTokenPath("volume", test.target, test.podUID)
			require.Error(t, err)
		})
	}
}

func TestWorkloadIdentityRepublish(t *testing.T) {
	target := filepath.Join(t.TempDir(), "pods", "pod-a", "volumes", "kubernetes.io~csi", "volume", "mount")
	require.NoError(t, os.MkdirAll(target, 0750))
	d := NewFakeDriver()
	mounter := mount.NewFakeMounter([]mount.MountPoint{{Device: "blobfuse2", Path: target, Type: "fuse"}})
	d.mounter = &mount.SafeFormatAndMount{Interface: mounter}
	req := &csi.NodeStageVolumeRequest{
		VolumeId:          "shared-volume",
		StagingTargetPath: target,
		VolumeCapability: &csi.VolumeCapability{AccessMode: &csi.VolumeCapability_AccessMode{
			Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
		}},
		VolumeContext: map[string]string{
			mountWithWITokenField:    "true",
			clientIDField:            "shared-client",
			storageAccountField:      "sharedaccount",
			containerNameField:       "sharedcontainer",
			serviceAccountTokenField: `{"api://AzureADTokenExchange":{"token":"refreshed-token"}}`,
		},
	}
	_, err := d.NodeStageVolume(context.Background(), req)
	require.ErrorContains(t, err, "Pod UID")
	req.VolumeContext[podUIDField] = "pod-a"
	_, err = d.NodeStageVolume(context.Background(), req)
	require.NoError(t, err)
	token, err := os.ReadFile(workloadIdentityTokenPath(req.VolumeId, target))
	require.NoError(t, err)
	require.Equal(t, "refreshed-token", string(token))
	require.Empty(t, mounter.GetLog(), "republish must not remount an existing filesystem")
}

func TestWorkloadIdentityTokenRejectsSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "outside")
	require.NoError(t, os.WriteFile(outside, []byte("unchanged"), 0600))
	token := filepath.Join(root, "token")
	require.NoError(t, os.Symlink(outside, token))
	require.Error(t, writeWorkloadIdentityToken(token, "replacement"))
	data, err := os.ReadFile(outside)
	require.NoError(t, err)
	require.Equal(t, "unchanged", string(data))

	parentLink := filepath.Join(root, "linked-directory")
	require.NoError(t, os.Symlink(root, parentLink))
	require.Error(t, writeWorkloadIdentityToken(filepath.Join(parentLink, "another-token"), "replacement"))
}

func TestWorkloadIdentityTokenAtomicRefresh(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	token := strings.Repeat("assertion", 1024)
	require.NoError(t, writeWorkloadIdentityToken(path, token))
	var readers sync.WaitGroup
	done := make(chan struct{})
	readers.Go(func() {
		for {
			select {
			case <-done:
				return
			default:
				data, err := os.ReadFile(path)
				if err != nil || string(data) != token {
					t.Errorf("observed incomplete assertion: bytes=%d error=%v", len(data), err)
					return
				}
			}
		}
	})
	for i := 0; i < 20; i++ {
		require.NoError(t, writeWorkloadIdentityToken(path, token))
	}
	close(done)
	readers.Wait()
	files, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	require.Len(t, files, 1)
}
