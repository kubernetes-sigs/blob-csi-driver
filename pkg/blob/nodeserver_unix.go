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

//go:build !windows
// +build !windows

package blob

import "syscall"

// probeMount performs a lightweight mount-liveness check on target.
//
// It uses syscall.Statfs, which is served by the kernel FUSE layer without
// issuing any data-plane request to Azure Blob Storage, while still surfacing
// the failure modes we care about (ENOTCONN when the blobfuse process died,
// ESTALE, EIO on a broken mount).
func probeMount(target string) error {
	var stat syscall.Statfs_t
	return syscall.Statfs(target, &stat)
}
