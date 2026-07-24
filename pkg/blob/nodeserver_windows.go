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

//go:build windows
// +build windows

package blob

import "os"

// probeMount performs a lightweight mount-liveness check on target.
//
// blobfuse itself is Linux-only; on Windows the driver runs against SMB via
// csi-proxy and there is no FUSE mount to probe. Fall back to os.Lstat, which
// is a cheap kernel-level check that still detects a missing or broken
// target path.
func probeMount(target string) error {
	_, err := os.Lstat(target)
	return err
}
