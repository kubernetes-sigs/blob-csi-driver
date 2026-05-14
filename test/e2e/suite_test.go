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
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/reporters"
	"github.com/onsi/gomega"
	"github.com/pborman/uuid"
	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/framework/config"
	_ "k8s.io/kubernetes/test/e2e/framework/debug/init"
	"sigs.k8s.io/blob-csi-driver/pkg/blob"
	"sigs.k8s.io/blob-csi-driver/pkg/util"
	"sigs.k8s.io/blob-csi-driver/test/utils/azure"
	"sigs.k8s.io/blob-csi-driver/test/utils/credentials"
)

const (
	kubeconfigEnvVar = "KUBECONFIG"
	reportDirEnv     = "ARTIFACTS"
	defaultReportDir = "/workspace/_artifacts"
)

var isAzureStackCloud = strings.EqualFold(os.Getenv("AZURE_CLOUD_NAME"), "AZURESTACKCLOUD")
var isCapzTest = os.Getenv("NODE_MACHINE_TYPE") != "" || os.Getenv("AZURE_NODE_MACHINE_TYPE") != ""
var blobDriver *blob.Driver
var projectRoot string

type testCmd struct {
	command  string
	args     []string
	startLog string
	endLog   string
}

func TestMain(m *testing.M) {
	flag.StringVar(&projectRoot, "project-root", "", "path to the blob csi driver project root, used for script execution")
	flag.Parse()
	if projectRoot == "" {
		klog.Fatal("project-root must be set")
	}

	config.CopyFlags(config.Flags, flag.CommandLine)
	framework.RegisterCommonFlags(flag.CommandLine)
	framework.RegisterClusterFlags(flag.CommandLine)
	flag.Parse()
	framework.AfterReadingAllFlags(&framework.TestContext)
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

var _ = ginkgo.SynchronizedBeforeSuite(func(ctx ginkgo.SpecContext) []byte {
	creds, err := credentials.CreateAzureCredentialFile()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	azureClient, err := azure.GetClient(creds.Cloud, creds.SubscriptionID, creds.AADClientID, creds.TenantID, creds.AADClientSecret, creds.AADFederatedTokenFile)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	_, err = azureClient.EnsureResourceGroup(ctx, creds.ResourceGroup, creds.Location, nil)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

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

	return nil
}, func(_ ginkgo.SpecContext, _ []byte) {
	// k8s.io/kubernetes/test/e2e/framework requires env KUBECONFIG to be set
	// it does not fall back to defaults
	if os.Getenv(kubeconfigEnvVar) == "" {
		kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
		os.Setenv(kubeconfigEnvVar, kubeconfig)
	}

	// spin up a blob driver locally to make use of the azure client and controller service
	kubeconfig := os.Getenv(kubeconfigEnvVar)
	_, useBlobfuseProxy := os.LookupEnv("ENABLE_BLOBFUSE_PROXY")
	os.Setenv("AZURE_CREDENTIAL_FILE", credentials.TempAzureCredentialFilePath)
	driverOptions := blob.DriverOptions{
		NodeID:                   os.Getenv("nodeid"),
		DriverName:               blob.DefaultDriverName,
		BlobfuseProxyEndpoint:    "",
		EnableBlobfuseProxy:      useBlobfuseProxy,
		BlobfuseProxyConnTimeout: 5,
		EnableBlobMockMount:      false,
	}
	kubeClient, err := util.GetKubeClient(kubeconfig, 25.0, 50, "")
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	cloud, err := blob.GetCloudProvider(context.Background(), kubeClient, driverOptions.NodeID, "", "", "", false)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	blobDriver = blob.NewDriver(&driverOptions, kubeClient, cloud)
	go func() {
		err := blobDriver.Run(context.Background(), fmt.Sprintf("unix:///tmp/csi-%s.sock", uuid.NewUUID().String()))
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	}()
})

var _ = ginkgo.SynchronizedAfterSuite(func(_ ginkgo.SpecContext) {},
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

		checkAccountCreationLeak(ctx)

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
			log.Printf("Failed to run command: %s %s, Error: %v\n", cmd.command, strings.Join(cmd.args, " "), err)
		}
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		log.Println(cmd.endLog)
	}
}

func checkAccountCreationLeak(ctx context.Context) {
	creds, err := credentials.CreateAzureCredentialFile()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	azureClient, err := azure.GetClient(creds.Cloud, creds.SubscriptionID, creds.AADClientID, creds.TenantID, creds.AADClientSecret, creds.AADFederatedTokenFile)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	accountNum, err := azureClient.GetAccountNumByResourceGroup(ctx, creds.ResourceGroup)
	framework.ExpectNoError(err, fmt.Sprintf("failed to GetAccountNumByResourceGroup(%s): %v", creds.ResourceGroup, err))
	ginkgo.By(fmt.Sprintf("GetAccountNumByResourceGroup(%s) returns %d accounts", creds.ResourceGroup, accountNum))

	accountLimitInTest := 22
	gomega.Expect(accountNum >= accountLimitInTest).To(gomega.BeFalse())
}

const (
	wiServiceAccountName         = "blob-wi-test-sa"
	wiServiceAccountNamespace    = "default"
	wiStorageBlobDataContributor = "ba92f5b4-2d11-453d-a403-e96b0029c9fe" // Storage Blob Data Contributor role GUID
	// wiPropagationWait is the time to wait for ARM FIC + RBAC propagation.
	// AAD OIDC metadata cache is handled separately by waitForAADTokenExchange.
	wiPropagationWait = 2 * time.Minute
)

// setupWorkloadIdentity configures workload identity for e2e tests:
// 1. Discovers the OIDC issuer URL from kube-apiserver
// 2. Gets the node identity client ID and principal ID
// 3. Creates a federated identity credential
// 4. Creates a Kubernetes service account with WI annotation
// 5. Assigns Storage Blob Data Contributor role to the identity
// Returns the identity client ID for use in StorageClass parameters.
func setupWorkloadIdentity(ctx context.Context, cs clientset.Interface, azureClient *azure.Client, creds *credentials.Credentials) (string, error) {
	log.Println("Setting up workload identity for e2e tests...")

	// Step 1: Discover OIDC issuer URL from kube-apiserver
	oidcIssuerURL, err := discoverOIDCIssuer(ctx, cs)
	if err != nil {
		return "", fmt.Errorf("failed to discover OIDC issuer: %v", err)
	}
	log.Printf("Discovered OIDC issuer URL: %s", oidcIssuerURL)

	// Step 2: Get node identity info
	identityInfo, err := azureClient.GetFirstUserAssignedIdentity(ctx, creds.ResourceGroup)
	if err != nil {
		return "", fmt.Errorf("failed to get node identity: %v", err)
	}
	log.Printf("Node identity clientID: %s, principalID: %s", identityInfo.ClientID, identityInfo.PrincipalID)

	// Parse identity name and resource group from resource ID
	parts := strings.Split(identityInfo.ResourceID, "/")
	if len(parts) < 9 || !strings.EqualFold(parts[3], "resourceGroups") || !strings.EqualFold(parts[7], "userAssignedIdentities") {
		return "", fmt.Errorf("invalid identity resource ID format: %s", identityInfo.ResourceID)
	}
	identityRG := parts[4]
	identityName := parts[8]
	log.Printf("Identity resource group: %s, name: %s", identityRG, identityName)

	// Step 3: Create federated identity credential with deterministic name.
	// Use a fixed name so that concurrent/repeated runs reuse the same FIC
	// instead of leaking new ones on every run. Handle 409 Conflict gracefully
	// when the issuer+subject combination already exists.
	ficName := "blob-e2e-wi-fic"
	subject := fmt.Sprintf("system:serviceaccount:%s:%s", wiServiceAccountNamespace, wiServiceAccountName)
	err = azureClient.CreateFederatedIdentityCredential(ctx, identityRG, identityName, ficName, oidcIssuerURL, subject)
	if err != nil {
		// If the issuer+subject already exists (409 Conflict), it's safe to continue
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) && respErr.StatusCode == http.StatusConflict {
			log.Printf("Federated identity credential already exists (issuer+subject match), reusing")
		} else {
			return "", fmt.Errorf("failed to create federated identity credential: %v", err)
		}
	} else {
		log.Printf("Federated identity credential created: %s", ficName)
	}

	// Step 4: Create or update Kubernetes service account with WI annotation
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      wiServiceAccountName,
			Namespace: wiServiceAccountNamespace,
			Labels: map[string]string{
				"azure.workload.identity/use": "true",
			},
			Annotations: map[string]string{
				"azure.workload.identity/client-id": identityInfo.ClientID,
				"azure.workload.identity/tenant-id": creds.TenantID,
			},
		},
	}
	_, err = cs.CoreV1().ServiceAccounts(wiServiceAccountNamespace).Create(ctx, sa, metav1.CreateOptions{})
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			log.Printf("Service account %s already exists, updating...", wiServiceAccountName)
			existing, getErr := cs.CoreV1().ServiceAccounts(wiServiceAccountNamespace).Get(ctx, wiServiceAccountName, metav1.GetOptions{})
			if getErr != nil {
				return "", fmt.Errorf("failed to get existing service account: %v", getErr)
			}
			existing.Labels = sa.Labels
			existing.Annotations = sa.Annotations
			_, err = cs.CoreV1().ServiceAccounts(wiServiceAccountNamespace).Update(ctx, existing, metav1.UpdateOptions{})
			if err != nil {
				return "", fmt.Errorf("failed to update service account: %v", err)
			}
		} else {
			return "", fmt.Errorf("failed to create service account: %v", err)
		}
	}
	log.Printf("Service account %s created/updated in namespace %s", wiServiceAccountName, wiServiceAccountNamespace)

	// Step 5: Assign Storage Blob Data Contributor role to identity
	err = azureClient.AssignRoleToIdentity(ctx, creds.ResourceGroup, identityInfo.PrincipalID, wiStorageBlobDataContributor)
	if err != nil {
		return "", fmt.Errorf("failed to assign Storage Blob Data Contributor role: %v", err)
	}
	log.Printf("Assigned Storage Blob Data Contributor role to identity")

	// Wait for ARM eventual consistency (FIC + RBAC propagation)
	log.Printf("Waiting %v for FIC and RBAC propagation...", wiPropagationWait)
	time.Sleep(wiPropagationWait)

	// Verify OIDC JWKS endpoint is accessible and contains signing keys.
	// AAD validates SA tokens by fetching the JWKS from the OIDC issuer.
	// In CAPZ clusters the JWKS is served from Azure Blob Storage and may
	// not be immediately available after cluster creation, leading to
	// AADSTS7000272 errors during token exchange.
	if err := waitForOIDCJWKS(oidcIssuerURL, 5*time.Minute); err != nil {
		return "", fmt.Errorf("OIDC JWKS not ready: %v", err)
	}

	// Wait for AAD to accept token exchange. AAD caches OIDC metadata
	// independently of endpoint availability, so even after JWKS is
	// accessible, token exchanges may fail with AADSTS7000272 until
	// AAD refreshes its internal cache.
	if err := waitForAADTokenExchange(ctx, cs, identityInfo.ClientID, 20*time.Minute); err != nil {
		return "", fmt.Errorf("AAD token exchange not ready: %v", err)
	}

	return identityInfo.ClientID, nil
}

// discoverOIDCIssuer retrieves the OIDC issuer URL from the kube-apiserver's
// well-known OpenID configuration endpoint.
func discoverOIDCIssuer(ctx context.Context, cs clientset.Interface) (string, error) {
	result := cs.Discovery().RESTClient().Get().AbsPath("/.well-known/openid-configuration").Do(ctx)
	body, err := result.Raw()
	if err != nil {
		return "", fmt.Errorf("failed to get OIDC configuration: %v", err)
	}

	var oidcConfig struct {
		Issuer string `json:"issuer"`
	}
	if err := json.Unmarshal(body, &oidcConfig); err != nil {
		return "", fmt.Errorf("failed to parse OIDC configuration: %v", err)
	}
	if oidcConfig.Issuer == "" {
		return "", fmt.Errorf("OIDC issuer URL is empty in cluster configuration")
	}
	return oidcConfig.Issuer, nil
}

// waitForOIDCJWKS polls the OIDC issuer's JWKS endpoint until it returns a
// valid response containing at least one signing key. This guards against the
// race where AAD tries to validate a SA token before the JWKS document is
// published (AADSTS7000272).
func waitForOIDCJWKS(issuerURL string, timeout time.Duration) error {
	jwksURL := strings.TrimSuffix(issuerURL, "/") + "/openid/v1/jwks"
	log.Printf("Waiting up to %v for OIDC JWKS to be available at %s", timeout, jwksURL)

	httpClient := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		},
	}

	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := httpClient.Get(jwksURL) //nolint:gosec // URL is constructed from cluster OIDC issuer, not user input
		if err != nil {
			lastErr = fmt.Errorf("GET %s: %v", jwksURL, err)
			log.Printf("JWKS not ready: %v, retrying...", lastErr)
			time.Sleep(10 * time.Second)
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("reading JWKS response: %v", err)
			time.Sleep(10 * time.Second)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("JWKS returned status %d", resp.StatusCode)
			log.Printf("JWKS not ready: %v, retrying...", lastErr)
			time.Sleep(10 * time.Second)
			continue
		}

		var jwks struct {
			Keys []json.RawMessage `json:"keys"`
		}
		if err := json.Unmarshal(body, &jwks); err != nil {
			lastErr = fmt.Errorf("parsing JWKS: %v", err)
			time.Sleep(10 * time.Second)
			continue
		}
		if len(jwks.Keys) == 0 {
			lastErr = fmt.Errorf("JWKS contains no keys")
			log.Printf("JWKS not ready: %v, retrying...", lastErr)
			time.Sleep(10 * time.Second)
			continue
		}

		log.Printf("OIDC JWKS is ready with %d key(s)", len(jwks.Keys))
		return nil
	}
	return fmt.Errorf("timed out waiting for OIDC JWKS: %v", lastErr)
}

// waitForAADTokenExchange polls AAD token exchange until it succeeds.
// AAD independently caches OIDC metadata; even after the JWKS endpoint
// returns keys, AAD may still reject token exchanges with AADSTS7000272
// until its internal cache refreshes. This function creates a short-lived
// service account token and attempts a client_credentials + client_assertion
// exchange against the AAD token endpoint, retrying until success or timeout.
func waitForAADTokenExchange(ctx context.Context, cs clientset.Interface, clientID string, timeout time.Duration) error {
	log.Printf("Waiting up to %v for AAD to accept token exchange for client %s", timeout, clientID)

	httpClient := &http.Client{Timeout: 10 * time.Second}
	deadline := time.Now().Add(timeout)
	var lastErr error

	for time.Now().Before(deadline) {
		// Create a short-lived SA token via TokenRequest API
		tokenReq := &authv1.TokenRequest{
			Spec: authv1.TokenRequestSpec{
				Audiences:         []string{"api://AzureADTokenExchange"},
				ExpirationSeconds: int64Ptr(600),
			},
		}
		tokenResp, err := cs.CoreV1().ServiceAccounts(wiServiceAccountNamespace).
			CreateToken(ctx, wiServiceAccountName, tokenReq, metav1.CreateOptions{})
		if err != nil {
			lastErr = fmt.Errorf("creating SA token: %v", err)
			log.Printf("Token exchange check: %v, retrying...", lastErr)
			time.Sleep(15 * time.Second)
			continue
		}

		saToken := tokenResp.Status.Token

		// Try exchanging with AAD
		// Use the well-known Azure AD v2 token endpoint
		tenantID := os.Getenv("AZURE_TENANT_ID")
		if tenantID == "" {
			// Try to get from credentials file
			creds, credErr := credentials.CreateAzureCredentialFile()
			if credErr == nil {
				tenantID = creds.TenantID
			}
		}
		if tenantID == "" {
			return fmt.Errorf("AZURE_TENANT_ID not set and not available from credentials")
		}

		tokenURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", tenantID)
		data := fmt.Sprintf(
			"grant_type=client_credentials&client_id=%s&scope=https://storage.azure.com/.default"+
				"&client_assertion_type=urn:ietf:params:oauth:client-assertion-type:jwt-bearer"+
				"&client_assertion=%s",
			clientID, saToken)

		resp, err := httpClient.Post(tokenURL, "application/x-www-form-urlencoded", strings.NewReader(data)) //nolint:gosec,noctx
		if err != nil {
			lastErr = fmt.Errorf("AAD token request: %v", err)
			log.Printf("Token exchange check: %v, retrying...", lastErr)
			time.Sleep(15 * time.Second)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			log.Printf("AAD token exchange succeeded")
			return nil
		}

		// Check if it's the expected AADSTS7000272 (OIDC not yet cached)
		bodyStr := string(body)
		if strings.Contains(bodyStr, "AADSTS7000272") {
			lastErr = fmt.Errorf("AAD returned AADSTS7000272 (OIDC metadata not yet cached)")
			log.Printf("Token exchange check: %v, retrying in 15s...", lastErr)
			time.Sleep(15 * time.Second)
			continue
		}

		// Other AAD error — log but keep retrying (could be transient)
		lastErr = fmt.Errorf("AAD returned status %d: %s", resp.StatusCode, bodyStr)
		log.Printf("Token exchange check: %v, retrying...", lastErr)
		time.Sleep(15 * time.Second)
	}
	return fmt.Errorf("timed out waiting for AAD token exchange: %v", lastErr)
}

func int64Ptr(i int64) *int64 { return &i }
