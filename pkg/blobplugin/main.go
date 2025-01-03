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

package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"

	"sigs.k8s.io/blob-csi-driver/pkg/blob"
	"sigs.k8s.io/blob-csi-driver/pkg/util"

	"k8s.io/component-base/metrics/legacyregistry"
	"k8s.io/klog/v2"
)

var driverOptions blob.DriverOptions
var (
	metricsAddress             = flag.String("metrics-address", "", "export the metrics")
	version                    = flag.Bool("version", false, "Print the version and exit.")
	endpoint                   = flag.String("endpoint", "unix://tmp/csi.sock", "CSI endpoint")
	kubeconfig                 = flag.String("kubeconfig", "", "Absolute path to the kubeconfig file. Required only when running out of cluster.")
	cloudConfigSecretName      = flag.String("cloud-config-secret-name", "azure-cloud-provider", "secret name of cloud config")
	cloudConfigSecretNamespace = flag.String("cloud-config-secret-namespace", "kube-system", "secret namespace of cloud config")
	customUserAgent            = flag.String("custom-user-agent", "", "custom userAgent")
	userAgentSuffix            = flag.String("user-agent-suffix", "", "userAgent suffix")
	allowEmptyCloudConfig      = flag.Bool("allow-empty-cloud-config", true, "allow running driver without cloud config")
	kubeAPIQPS                 = flag.Float64("kube-api-qps", 25.0, "QPS to use while communicating with the kubernetes apiserver.")
	kubeAPIBurst               = flag.Int("kube-api-burst", 50, "Burst to use while communicating with the kubernetes apiserver.")
)

func init() {
	klog.InitFlags(nil)
	driverOptions.AddFlags()
}

func main() {
	flag.Parse()
	if *version {
		info, err := blob.GetVersionYAML(driverOptions.DriverName)
		if err != nil {
			klog.Fatalln(err)
		}
		fmt.Println(info) // nolint
		os.Exit(0)
	}

	exportMetrics()
	handle()
	os.Exit(0)
}

func handle() {
	userAgent := blob.GetUserAgent(driverOptions.DriverName, *customUserAgent, *userAgentSuffix)
	klog.V(2).Infof("driver userAgent: %s", userAgent)

	kubeClient, err := util.GetKubeClient(*kubeconfig, *kubeAPIQPS, *kubeAPIBurst, userAgent)
	if err != nil {
		klog.Warningf("failed to get kubeClient, error: %v", err)
	}

	cloud, err := blob.GetCloudProvider(context.Background(), kubeClient, driverOptions.NodeID, *cloudConfigSecretName, *cloudConfigSecretNamespace, userAgent, *allowEmptyCloudConfig)
	if err != nil {
		klog.Fatalf("failed to get Azure Cloud Provider, error: %v", err)
	}
	klog.V(2).Infof("cloud: %s, location: %s, rg: %s, VnetName: %s, VnetResourceGroup: %s, SubnetName: %s", cloud.Cloud, cloud.Location, cloud.ResourceGroup, cloud.VnetName, cloud.VnetResourceGroup, cloud.SubnetName)

	driver := blob.NewDriver(&driverOptions, kubeClient, cloud)
	if driver == nil {
		klog.Fatalln("Failed to initialize Azure Blob Storage CSI driver")
	}
	if err := driver.Run(context.Background(), *endpoint); err != nil {
		klog.Fatalf("Failed to run Azure Blob Storage CSI driver: %v", err)
	}
}

func exportMetrics() {
	if *metricsAddress == "" {
		return
	}
	l, err := net.Listen("tcp", *metricsAddress)
	if err != nil {
		klog.Warningf("failed to get listener for metrics endpoint: %v", err)
		return
	}
	serve(context.Background(), l, serveMetrics)
}

func serve(_ context.Context, l net.Listener, serveFunc func(net.Listener) error) {
	path := l.Addr().String()
	klog.V(2).Infof("set up prometheus server on %v", path)
	go func() {
		defer l.Close()
		if err := serveFunc(l); err != nil {
			klog.Fatalf("serve failure(%v), address(%v)", err, path)
		}
	}()
}

func serveMetrics(l net.Listener) error {
	m := http.NewServeMux()
	m.Handle("/metrics", legacyregistry.Handler()) //nolint, because azure cloud provider uses legacyregistry currently
	return trapClosedConnErr(http.Serve(l, m))
}

func trapClosedConnErr(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "use of closed network connection") {
		return nil
	}
	return err
}
