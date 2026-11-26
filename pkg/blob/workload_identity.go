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
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func workloadIdentityTokenPath(volumeID, targetPath string) string {
	scope := sha256.Sum256([]byte(volumeID + "\x00" + filepath.Clean(targetPath)))
	return filepath.Join(filepath.Dir(targetPath), fmt.Sprintf(".blob-csi-wi-%x.token", scope))
}

func podWorkloadIdentityTokenPath(volumeID, targetPath, podUID string) (string, error) {
	if podUID == "" || podUID == "." || podUID == ".." || strings.ContainsAny(podUID, `/\`) {
		return "", fmt.Errorf("token-only workload identity requires a valid Pod UID")
	}
	podVolumePath := string(filepath.Separator) + filepath.Join("pods", podUID, "volumes") + string(filepath.Separator)
	if !filepath.IsAbs(targetPath) || !strings.Contains(filepath.Clean(targetPath), podVolumePath) {
		return "", fmt.Errorf("token-only workload identity mount target does not belong to the supplied Pod UID")
	}
	// The kubelet target contains the immutable Pod UID and volume mount identity.
	// Keep credentials beside, not inside, the filesystem exposed to the Pod.
	return workloadIdentityTokenPath(volumeID, targetPath), nil
}

func writeWorkloadIdentityToken(path, token string) error {
	parent := filepath.Dir(path)
	info, err := os.Lstat(parent)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("workload identity token directory must not be a symlink")
	}
	root, err := os.OpenRoot(parent)
	if err != nil {
		return err
	}
	defer root.Close()

	name := filepath.Base(path)
	if info, err := root.Lstat(name); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("workload identity token path must be a regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	tempName := ".blob-csi-wi-" + rand.Text()
	file, err := root.OpenFile(tempName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(tempName)
	if _, err := file.WriteString(token); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return root.Rename(tempName, name)
}

func removeWorkloadIdentityToken(volumeID, targetPath string) error {
	err := os.Remove(workloadIdentityTokenPath(volumeID, targetPath))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
