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
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
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

// wiClientID is pre-populated during SynchronizedBeforeSuite so that the
// AAD OIDC cache warm-up happens in parallel with the CSI driver bootstrap,
// avoiding a ~20 min delay when the WI test actually runs.
var wiClientID string

// wiReady is closed when the background AAD OIDC cache warm-up finishes
// (successfully or with an error stored in errWISetup).
var wiReady = make(chan struct{})

// errWISetup holds any error from the background WI warm-up goroutine.
var errWISetup error

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

	// Pre-warm AAD OIDC cache for workload identity test.
	// AAD independently caches OIDC metadata and can take 20-30+ min to
	// accept token exchanges for a new OIDC issuer (AADSTS7000272).
	// We run the fast configuration steps (FIC, SA, RBAC) synchronously,
	// then kick off the slow AAD token exchange wait in a background
	// goroutine so that other tests can run in parallel. The WI test
	// blocks on wiReady channel when it needs the client ID.
	var clientID string
	if isCapzTest {
		kubeconfig := os.Getenv(kubeconfigEnvVar)
		if kubeconfig == "" {
			kubeconfig = filepath.Join(os.Getenv("HOME"), ".kube", "config")
		}
		cs, csErr := util.GetKubeClient(kubeconfig, 25.0, 50, "")
		gomega.Expect(csErr).NotTo(gomega.HaveOccurred())
		clientID, err = configureWorkloadIdentity(ctx, cs, azureClient, creds)
		if err != nil {
			log.Printf("WARNING: WI configuration failed: %v", err)
			clientID = ""
		}
	}

	return []byte(clientID)
}, func(_ ginkgo.SpecContext, data []byte) {
	// Store pre-configured WI client ID from node 1 and start background
	// AAD OIDC cache warm-up. The WI test will block on wiReady channel.
	if len(data) > 0 {
		wiClientID = string(data)
		log.Printf("Received WI clientID: %s, starting background AAD warm-up", wiClientID)
		go func() {
			defer close(wiReady)
			kubeconfig := os.Getenv(kubeconfigEnvVar)
			if kubeconfig == "" {
				kubeconfig = filepath.Join(os.Getenv("HOME"), ".kube", "config")
			}
			bgCS, err := util.GetKubeClient(kubeconfig, 25.0, 50, "")
			if err != nil {
				errWISetup = fmt.Errorf("failed to create kube client for WI warm-up: %v", err)
				log.Printf("WARNING: %v", errWISetup)
				return
			}
			if err := waitForAADTokenExchange(context.Background(), bgCS, wiClientID, 45*time.Minute); err != nil {
				errWISetup = fmt.Errorf("AAD token exchange not ready: %v", err)
				log.Printf("WARNING: WI background warm-up failed: %v", errWISetup)
			} else {
				log.Printf("AAD token exchange warm-up succeeded in background")
			}
		}()
	} else {
		close(wiReady) // No WI setup needed, unblock immediately
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
)

// configureWorkloadIdentity sets up workload identity resources (fast, sync):
// 1. Discovers the OIDC issuer URL from kube-apiserver
// 2. Gets the node identity client ID and principal ID
// 3. Creates a federated identity credential
// 4. Creates a Kubernetes service account with WI annotation
// 5. Assigns Storage Blob Data Contributor role to the identity
// 6. Waits for OIDC JWKS endpoint to be accessible
// Returns the identity client ID. Does NOT wait for AAD token exchange
// (that happens in a background goroutine to avoid blocking BeforeSuite).
func configureWorkloadIdentity(ctx context.Context, cs clientset.Interface, azureClient *azure.Client, creds *credentials.Credentials) (string, error) {
	log.Println("Configuring workload identity for e2e tests...")

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
	log.Printf("Creating FIC: identity=%s/%s, issuer=%s, subject=%s", identityRG, identityName, oidcIssuerURL, subject)
	err = azureClient.CreateFederatedIdentityCredential(ctx, identityRG, identityName, ficName, oidcIssuerURL, subject)
	if err != nil {
		// A 409 with "issuer/subject combination already exists" means a matching FIC is present; safe to reuse.
		// Any other 409 (concurrent update, quota, etc.) should be treated as a real failure.
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) && respErr.StatusCode == http.StatusConflict &&
			strings.Contains(respErr.Error(), "already exists") {
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
			if existing.Labels == nil {
				existing.Labels = make(map[string]string)
			}
			for k, v := range sa.Labels {
				existing.Labels[k] = v
			}
			if existing.Annotations == nil {
				existing.Annotations = make(map[string]string)
			}
			for k, v := range sa.Annotations {
				existing.Annotations[k] = v
			}
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

	// Skip static sleep for FIC/RBAC propagation — the token exchange retry
	// loop (waitForAADTokenExchange) naturally handles FIC propagation delay,
	// and RBAC propagation completes well before the actual CSI mount runs.

	// Verify OIDC JWKS endpoint is accessible and contains signing keys.
	// AAD validates SA tokens by fetching the JWKS from the OIDC issuer.
	// In CAPZ clusters the JWKS is served from Azure Blob Storage and may
	// not be immediately available after cluster creation, leading to
	// AADSTS7000272 errors during token exchange.
	if err := waitForOIDCJWKS(oidcIssuerURL, 5*time.Minute); err != nil {
		return "", fmt.Errorf("OIDC JWKS not ready: %v", err)
	}

	// Verify the OIDC issuer's JWKS contains the key used by kube-apiserver
	// to sign ServiceAccount tokens. In CAPZ clusters, the JWKS is uploaded
	// to Azure Blob Storage during cluster creation, but the apiserver may
	// use a different signing key if the key pair was rotated or if there was
	// a race during bootstrap. If the keys don't match, AAD will permanently
	// reject token exchanges with AADSTS7000272.
	if err := verifyJWKSKeyMatch(ctx, cs, oidcIssuerURL, creds); err != nil {
		return "", fmt.Errorf("OIDC JWKS key mismatch: %v", err)
	}

	// NOTE: AAD OIDC cache warm-up (waitForAADTokenExchange) runs in a
	// background goroutine started from SynchronizedBeforeSuite's second
	// func, so it does not block other tests from starting.

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

// verifyJWKSKeyMatch ensures the OIDC issuer's JWKS (hosted on Azure Blob
// Storage for CAPZ clusters) contains the signing key actually used by
// kube-apiserver. CAPZ uploads the JWKS during cluster creation, but a race
// condition can cause the blob to contain a stale key while the apiserver
// uses a different one. When this happens, AAD permanently rejects token
// exchanges with AADSTS7000272 ("certificate ... could not be found in the
// metadata of identity provider"). This function detects the mismatch and
// fixes it by re-uploading the correct JWKS to the blob.
func verifyJWKSKeyMatch(ctx context.Context, cs clientset.Interface, issuerURL string, creds *credentials.Credentials) error {
	// 1. Create a SA token and extract the kid from its JWT header
	tokenReq := &authv1.TokenRequest{
		Spec: authv1.TokenRequestSpec{
			Audiences:         []string{"api://AzureADTokenExchange"},
			ExpirationSeconds: int64Ptr(600),
		},
	}
	tokenResp, err := cs.CoreV1().ServiceAccounts("default").
		CreateToken(ctx, "default", tokenReq, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("creating SA token: %v", err)
	}

	tokenKid, err := extractJWTKid(tokenResp.Status.Token)
	if err != nil {
		return fmt.Errorf("extracting kid from SA token: %v", err)
	}
	log.Printf("SA token kid: %s", tokenKid)

	// 2. Fetch JWKS from the OIDC issuer blob URL and extract kids
	jwksURL := strings.TrimSuffix(issuerURL, "/") + "/openid/v1/jwks"
	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Get(jwksURL) //nolint:gosec
	if err != nil {
		return fmt.Errorf("fetching JWKS from %s: %v", jwksURL, err)
	}
	blobJWKSBody, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return fmt.Errorf("reading JWKS response: %v", err)
	}

	blobKids, err := extractJWKSKids(blobJWKSBody)
	if err != nil {
		return fmt.Errorf("parsing blob JWKS: %v", err)
	}
	log.Printf("Blob JWKS kids: %v", blobKids)

	// 3. Check if the SA token kid is present in the blob JWKS
	found := false
	for _, kid := range blobKids {
		if kid == tokenKid {
			found = true
			break
		}
	}
	if found {
		log.Printf("JWKS key match verified: SA token kid %s found in blob JWKS", tokenKid)
		return nil
	}

	log.Printf("WARNING: JWKS key mismatch detected! SA token kid %s not in blob JWKS %v", tokenKid, blobKids)
	log.Printf("Fetching correct JWKS from kube-apiserver and re-uploading to blob storage...")

	// 4. Fetch the correct JWKS from kube-apiserver
	result := cs.Discovery().RESTClient().Get().AbsPath("/openid/v1/jwks").Do(ctx)
	apiJWKS, err := result.Raw()
	if err != nil {
		return fmt.Errorf("fetching JWKS from apiserver: %v", err)
	}

	// Verify the apiserver JWKS contains our kid
	apiKids, err := extractJWKSKids(apiJWKS)
	if err != nil {
		return fmt.Errorf("parsing apiserver JWKS: %v", err)
	}
	log.Printf("Apiserver JWKS kids: %v", apiKids)

	// Fail fast if the apiserver JWKS also lacks our token's kid
	apiHasKid := false
	for _, kid := range apiKids {
		if kid == tokenKid {
			apiHasKid = true
			break
		}
	}
	if !apiHasKid {
		return fmt.Errorf("apiserver JWKS does not contain token kid %s (has %v); cannot repair", tokenKid, apiKids)
	}

	// 5. Extract storage account and blob service URL from OIDC issuer URL
	// Format: https://<storageaccount>.z1.web.core.windows.net/
	storageAccount, blobServiceURL, err := extractStorageInfoFromIssuer(issuerURL)
	if err != nil {
		return fmt.Errorf("extracting storage info from issuer URL: %v", err)
	}
	log.Printf("OIDC storage account: %s, blob service: %s", storageAccount, blobServiceURL)

	// 6. Upload corrected JWKS to the $web container
	if err := uploadJWKSToBlob(blobServiceURL, apiJWKS, creds); err != nil {
		return fmt.Errorf("uploading corrected JWKS: %v", err)
	}

	// 7. Also upload the openid-configuration if needed
	// Preserve the original issuer URL (including trailing slash if present)
	// to match the FIC issuer configuration exactly.
	oidcConfig := fmt.Sprintf(`{"issuer":"%s","jwks_uri":"%s","response_types_supported":["id_token"],"subject_types_supported":["public"],"id_token_signing_alg_values_supported":["RS256"]}`,
		issuerURL, jwksURL)
	if err := uploadOIDCConfigToBlob(blobServiceURL, []byte(oidcConfig), creds); err != nil {
		log.Printf("WARNING: failed to upload openid-configuration: %v (non-fatal)", err)
	}

	log.Printf("Successfully re-uploaded JWKS to blob storage, waiting for AAD cache refresh...")
	return nil
}

// extractJWTKid parses a JWT token (without validation) and returns the "kid"
// from its header.
func extractJWTKid(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid JWT: expected 3 parts, got %d", len(parts))
	}
	// Pad base64url if needed
	headerB64 := parts[0]
	if m := len(headerB64) % 4; m != 0 {
		headerB64 += strings.Repeat("=", 4-m)
	}
	headerBytes, err := base64.URLEncoding.DecodeString(headerB64)
	if err != nil {
		return "", fmt.Errorf("decoding JWT header: %v", err)
	}
	var header struct {
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return "", fmt.Errorf("parsing JWT header: %v", err)
	}
	if header.Kid == "" {
		return "", fmt.Errorf("JWT header has no kid")
	}
	return header.Kid, nil
}

// extractJWKSKids extracts all "kid" values from a JWKS document.
func extractJWKSKids(jwksBody []byte) ([]string, error) {
	var jwks struct {
		Keys []struct {
			Kid string `json:"kid"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(jwksBody, &jwks); err != nil {
		return nil, fmt.Errorf("parsing JWKS: %v", err)
	}
	var kids []string
	for _, k := range jwks.Keys {
		if k.Kid != "" {
			kids = append(kids, k.Kid)
		}
	}
	return kids, nil
}

// extractStorageInfoFromIssuer extracts the storage account name and blob
// service URL from a CAPZ OIDC issuer URL.
// e.g., https://capzoidcXXX.z1.web.core.windows.net/ →
//
//	account: capzoidcXXX, blobURL: https://capzoidcXXX.blob.core.windows.net
func extractStorageInfoFromIssuer(issuerURL string) (account string, blobServiceURL string, err error) {
	u, err := url.Parse(issuerURL)
	if err != nil {
		return "", "", err
	}
	// hostname is like capzoidcXXX.z1.web.core.windows.net
	parts := strings.SplitN(u.Hostname(), ".", 2)
	if len(parts) < 2 {
		return "", "", fmt.Errorf("unexpected issuer hostname: %s", u.Hostname())
	}
	account = parts[0]
	// parts[1] is like "z1.web.core.windows.net"; extract storage suffix
	// Blob endpoints don't include the zone prefix, so strip "z1." and replace "web." with "blob."
	// z1.web.core.windows.net → blob.core.windows.net
	hostSuffix := parts[1]
	webIdx := strings.Index(hostSuffix, "web.")
	if webIdx < 0 {
		return "", "", fmt.Errorf("unexpected issuer hostname (no 'web.' segment): %s", u.Hostname())
	}
	// Everything after "web." is the storage suffix (e.g., "core.windows.net")
	storageSuffix := hostSuffix[webIdx+4:]
	blobServiceURL = fmt.Sprintf("https://%s.blob.%s", account, storageSuffix)
	return account, blobServiceURL, nil
}

// uploadJWKSToBlob uploads the JWKS document to the $web container of the
// OIDC storage account at path openid/v1/jwks.
// getAzureCredential creates an Azure credential from test credentials,
// supporting both client secret and federated token authentication.
func getAzureCredential(creds *credentials.Credentials) (azcore.TokenCredential, error) {
	// Derive cloud configuration for sovereign cloud support.
	var cloudCfg cloud.Configuration
	switch strings.ToUpper(os.Getenv("AZURE_CLOUD_NAME")) {
	case "AZURECHINACLOUD":
		cloudCfg = cloud.AzureChina
	case "AZUREUSGOVERNMENTCLOUD":
		cloudCfg = cloud.AzureGovernment
	default:
		cloudCfg = cloud.AzurePublic
	}
	if host := os.Getenv("AZURE_AUTHORITY_HOST"); host != "" {
		cloudCfg.ActiveDirectoryAuthorityHost = host
	}

	if creds.AADFederatedTokenFile != "" {
		return azidentity.NewWorkloadIdentityCredential(&azidentity.WorkloadIdentityCredentialOptions{
			ClientOptions: azcore.ClientOptions{Cloud: cloudCfg},
			TenantID:      creds.TenantID,
			ClientID:      creds.AADClientID,
			TokenFilePath: creds.AADFederatedTokenFile,
		})
	}
	return azidentity.NewClientSecretCredential(creds.TenantID, creds.AADClientID, creds.AADClientSecret,
		&azidentity.ClientSecretCredentialOptions{
			ClientOptions: azcore.ClientOptions{Cloud: cloudCfg},
		})
}

func uploadJWKSToBlob(blobServiceURL string, jwksBody []byte, creds *credentials.Credentials) error {
	azCred, err := getAzureCredential(creds)
	if err != nil {
		return fmt.Errorf("creating Azure credential: %v", err)
	}
	client, err := azblob.NewClient(blobServiceURL, azCred, nil)
	if err != nil {
		return fmt.Errorf("creating blob client: %v", err)
	}
	_, err = client.UploadBuffer(context.Background(), "$web", "openid/v1/jwks", jwksBody, nil)
	if err != nil {
		return fmt.Errorf("uploading JWKS blob: %v", err)
	}
	log.Printf("Uploaded corrected JWKS (%d bytes) to %s/$web/openid/v1/jwks", len(jwksBody), blobServiceURL)
	return nil
}

// uploadOIDCConfigToBlob uploads the openid-configuration to the $web
// container at .well-known/openid-configuration.
func uploadOIDCConfigToBlob(blobServiceURL string, configBody []byte, creds *credentials.Credentials) error {
	azCred, err := getAzureCredential(creds)
	if err != nil {
		return fmt.Errorf("creating Azure credential: %v", err)
	}
	client, err := azblob.NewClient(blobServiceURL, azCred, nil)
	if err != nil {
		return fmt.Errorf("creating blob client: %v", err)
	}
	_, err = client.UploadBuffer(context.Background(), "$web", ".well-known/openid-configuration", configBody, nil)
	return err
}

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

		authorityHost := os.Getenv("AZURE_AUTHORITY_HOST")
		if authorityHost == "" {
			cloudName := os.Getenv("AZURE_CLOUD_NAME")
			switch strings.ToUpper(cloudName) {
			case "AZURECHINACLOUD":
				authorityHost = "https://login.chinacloudapi.cn"
			case "AZUREUSGOVERNMENTCLOUD":
				authorityHost = "https://login.microsoftonline.us"
			default:
				authorityHost = "https://login.microsoftonline.com"
			}
		}

		tokenURL := fmt.Sprintf("%s/%s/oauth2/v2.0/token", strings.TrimSuffix(authorityHost, "/"), tenantID)
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
			lastErr = fmt.Errorf("AAD returned AADSTS7000272: %s", bodyStr)
			log.Printf("Token exchange check: AADSTS7000272 response: %s, retrying in 15s...", bodyStr)
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
