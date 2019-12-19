/*
Copyright 2019 The Kubernetes Authors.

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

package e2e

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	"k8s.io/kubernetes/test/e2e/framework"
)

const kubeconfigEnvVar = "KUBECONFIG"

var _ = ginkgo.BeforeSuite(func() {
	var err error
	// k8s.io/kubernetes/test/e2e/framework requires env KUBECONFIG to be set
	// it does not fall back to defaults
	if os.Getenv(kubeconfigEnvVar) == "" {
		kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
		os.Setenv(kubeconfigEnvVar, kubeconfig)
	}
	framework.HandleFlags()
	framework.AfterReadingAllFlags(&framework.TestContext)

	// Install BlobFuse CSI Driver on cluster from project root
	err = os.Chdir("../..")
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	defer func() {
		err := os.Chdir("test/e2e")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	}()

	projectRoot, err := os.Getwd()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(strings.HasSuffix(projectRoot, "blobfuse-csi-driver")).To(gomega.Equal(true))

	log.Println("Installing BlobFuse CSI Driver...")
	cmd := exec.Command("make", "e2e-bootstrap")
	cmd.Dir = projectRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	log.Println("BlobFuse CSI Driver installed")
})

var _ = ginkgo.AfterSuite(func() {
	var err error
	if os.Getenv(kubeconfigEnvVar) == "" {
		kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
		os.Setenv(kubeconfigEnvVar, kubeconfig)
	}

	// Uninstall BlobFuse CSI Driver on cluster from project root
	err = os.Chdir("../..")
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	defer func() {
		err := os.Chdir("test/e2e")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	}()

	projectRoot, err := os.Getwd()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(strings.HasSuffix(projectRoot, "blobfuse-csi-driver")).To(gomega.Equal(true))

	log.Println("Uninstalling BlobFuse CSI Driver...")
	cmd := exec.Command("make", "e2e-teardown")
	cmd.Dir = projectRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	log.Println("BlobFuse CSI Driver uninstalled")
})

func TestE2E(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "BlobFuse CSI Driver End-to-End Tests")
}
