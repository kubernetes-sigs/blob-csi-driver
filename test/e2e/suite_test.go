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
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/reporters"
	"github.com/onsi/gomega"
	"github.com/pborman/uuid"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/framework/config"
	_ "k8s.io/kubernetes/test/e2e/framework/debug/init"
	"sigs.k8s.io/blob-csi-driver/pkg/blob"
	"sigs.k8s.io/blob-csi-driver/test/utils/azure"
	"sigs.k8s.io/blob-csi-driver/test/utils/credentials"
	"sigs.k8s.io/blob-csi-driver/test/utils/testutil"
)

const (
	kubeconfigEnvVar = "KUBECONFIG"
	reportDirEnv     = "ARTIFACTS"
	defaultReportDir = "/workspace/_artifacts"
)

var isAzureStackCloud = strings.EqualFold(os.Getenv("AZURE_CLOUD_NAME"), "AZURESTACKCLOUD")
var blobDriver *blob.Driver
var projectRoot string

type testCmd struct {
	command  string
	args     []string
	startLog string
	endLog   string
}

func TestMain(m *testing.M) {
	handleFlags()

	os.Exit(m.Run())
}

func TestE2E(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	reportDir := os.Getenv(reportDirEnv)
	if reportDir == "" {
		reportDir = defaultReportDir
	}
	r := []ginkgo.Reporter{reporters.NewJUnitReporter(path.Join(reportDir, "junit_01.xml"))}
	ginkgo.RunSpecsWithDefaultAndCustomReporters(t, "Azure Blob Storage CSI driver End-to-End Tests", r)
}

var _ = ginkgo.SynchronizedBeforeSuite(func() []byte {
	creds, err := credentials.CreateAzureCredentialFile()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	azureClient, err := azure.GetClient(creds.Cloud, creds.SubscriptionID, creds.AADClientID, creds.TenantID, creds.AADClientSecret)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	_, err = azureClient.EnsureResourceGroup(context.Background(), creds.ResourceGroup, creds.Location, nil)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	if testutil.IsRunningInProw() {
		// Need to login to ACR using SP credential if we are running in Prow so we can push test images.
		// If running locally, user should run 'docker login' before running E2E tests
		registry := os.Getenv("REGISTRY")
		gomega.Expect(registry).NotTo(gomega.Equal(""))

		log.Println("Attempting docker login with Azure service principal")
		cmd := exec.Command("docker", "login", fmt.Sprintf("--username=%s", creds.AADClientID), fmt.Sprintf("--password=%s", creds.AADClientSecret), registry)
		err = cmd.Run()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		log.Println("docker login is successful")
	}

	// Install Azure Blob Storage CSI driver on cluster from project root
	e2eBootstrap := testCmd{
		command:  "make",
		args:     []string{"e2e-bootstrap"},
		startLog: "Installing Azure Blob Storage CSI driver ...",
		endLog:   "Azure Blob Storage CSI driver installed",
	}

	createMetricsSVC := testCmd{
		command:  "make",
		args:     []string{"create-metrics-svc"},
		startLog: "create metrics service ...",
		endLog:   "metrics service created",
	}
	execTestCmd([]testCmd{e2eBootstrap, createMetricsSVC})

	if testutil.IsRunningInProw() {
		data, err := json.Marshal(creds)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		return data
	}

	return nil
}, func(data []byte) {
	if testutil.IsRunningInProw() {
		creds := &credentials.Credentials{}
		err := json.Unmarshal(data, creds)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		// set env for azidentity.EnvironmentCredential
		os.Setenv("AZURE_TENANT_ID", creds.TenantID)
		os.Setenv("AZURE_CLIENT_ID", creds.AADClientID)
		os.Setenv("AZURE_CLIENT_SECRET", creds.AADClientSecret)
	}

	// k8s.io/kubernetes/test/e2e/framework requires env KUBECONFIG to be set
	// it does not fall back to defaults
	if os.Getenv(kubeconfigEnvVar) == "" {
		kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
		os.Setenv(kubeconfigEnvVar, kubeconfig)
	}

	// spin up a blob driver locally to make use of the azure client and controller service
	kubeconfig := os.Getenv(kubeconfigEnvVar)
	_, useBlobfuseProxy := os.LookupEnv("ENABLE_BLOBFUSE_PROXY")
	driverOptions := blob.DriverOptions{
		NodeID:                  os.Getenv("nodeid"),
		DriverName:              blob.DefaultDriverName,
		BlobfuseProxyEndpoint:   "",
		EnableBlobfuseProxy:     useBlobfuseProxy,
		BlobfuseProxyConnTimout: 5,
		EnableBlobMockMount:     false,
	}
	blobDriver = blob.NewDriver(&driverOptions)
	go func() {
		os.Setenv("AZURE_CREDENTIAL_FILE", credentials.TempAzureCredentialFilePath)
		blobDriver.Run(fmt.Sprintf("unix:///tmp/csi-%s.sock", uuid.NewUUID().String()), kubeconfig, false)
	}()
})

var _ = ginkgo.SynchronizedAfterSuite(func(ctx ginkgo.SpecContext) {},
	func(ctx ginkgo.SpecContext) {
		blobLog := testCmd{
			command:  "bash",
			args:     []string{"test/utils/blob_log.sh"},
			startLog: "==============start blob log(after suite)===================",
			endLog:   "==============end blob log(after suite)===================",
		}
		e2eTeardown := testCmd{
			command:  "make",
			args:     []string{"e2e-teardown"},
			startLog: "Uninstalling Azure Blob Storage CSI driver...",
			endLog:   "Azure Blob Storage CSI driver uninstalled",
		}
		execTestCmd([]testCmd{blobLog, e2eTeardown})

		// install/uninstall CSI Driver deployment scripts test
		installDriver := testCmd{
			command:  "bash",
			args:     []string{"deploy/install-driver.sh", "master", "local,enable-blobfuse-proxy"},
			startLog: "===================install CSI Driver deployment scripts test===================",
			endLog:   "===================================================",
		}
		uninstallDriver := testCmd{
			command:  "bash",
			args:     []string{"deploy/uninstall-driver.sh", "master", "local,enable-blobfuse-proxy"},
			startLog: "===================uninstall CSI Driver deployment scripts test===================",
			endLog:   "===================================================",
		}
		execTestCmd([]testCmd{installDriver, uninstallDriver})

		checkAccountCreationLeak()

		err := credentials.DeleteAzureCredentialFile()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	}, ginkgo.NodeTimeout(10*time.Minute))

func execTestCmd(cmds []testCmd) {
	ginkgo.GinkgoHelper()

	for _, cmd := range cmds {
		log.Println(cmd.startLog)
		cmdSh := exec.Command(cmd.command, cmd.args...)
		cmdSh.Dir = projectRoot
		cmdSh.Stdout = os.Stdout
		cmdSh.Stderr = os.Stderr
		err := cmdSh.Run()
		if err != nil {
			log.Printf("Failed to run command: %s %s, Error: %s\n", cmd.command, strings.Join(cmd.args, " "), err.Error())
		}
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		log.Println(cmd.endLog)
	}
}

func checkAccountCreationLeak() {
	creds, err := credentials.CreateAzureCredentialFile()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	azureClient, err := azure.GetClient(creds.Cloud, creds.SubscriptionID, creds.AADClientID, creds.TenantID, creds.AADClientSecret)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	accountNum, err := azureClient.GetAccountNumByResourceGroup(context.TODO(), creds.ResourceGroup)
	framework.ExpectNoError(err, fmt.Sprintf("failed to GetAccountNumByResourceGroup(%s): %v", creds.ResourceGroup, err))
	ginkgo.By(fmt.Sprintf("GetAccountNumByResourceGroup(%s) returns %d accounts", creds.ResourceGroup, accountNum))

	accountLimitInTest := 20
	framework.ExpectEqual(accountNum >= accountLimitInTest, false, fmt.Sprintf("current account num %d should not exceed %d", accountNum, accountLimitInTest))
}

// handleFlags sets up all flags and parses the command line.
func handleFlags() {
	registerFlags()
	handleFramworkFlags()
}

func registerFlags() {
	flag.StringVar(&projectRoot, "project-root", "", "path to the blob csi driver project root, used for script execution")
	flag.Parse()
	if projectRoot == "" {
		klog.Fatal("project-root must be set")
	}
}

func handleFramworkFlags() {
	config.CopyFlags(config.Flags, flag.CommandLine)
	framework.RegisterCommonFlags(flag.CommandLine)
	framework.RegisterClusterFlags(flag.CommandLine)
	flag.Parse()
	framework.AfterReadingAllFlags(&framework.TestContext)
}
