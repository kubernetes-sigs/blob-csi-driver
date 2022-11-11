/*
Copyright 2021 The Kubernetes Authors.

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
	"flag"
	"net"
	"os"

	"k8s.io/klog/v2"

	"github.com/go-ini/ini"
	"github.com/pkg/errors"
	server "sigs.k8s.io/blob-csi-driver/pkg/blobfuse-proxy/server"
	csicommon "sigs.k8s.io/blob-csi-driver/pkg/csi-common"
)

func init() {
	_ = flag.Set("logtostderr", "true")
}

var (
	blobfuseProxyEndpoint = flag.String("blobfuse-proxy-endpoint", "unix://tmp/blobfuse-proxy.sock", "blobfuse-proxy endpoint")
)

func main() {
	klog.InitFlags(nil)
	flag.Parse()
	proto, addr, err := csicommon.ParseEndpoint(*blobfuseProxyEndpoint)
	if err != nil {
		klog.Fatalf("failed to  parse endpoint %v", err.Error())
	}

	if proto == "unix" {
		addr = "/" + addr
		if err := os.Remove(addr); err != nil && !os.IsNotExist(err) {
			klog.Fatalf("Failed to remove %s, error: %s", addr, err.Error())
		}
	}

	listener, err := net.Listen(proto, addr)
	if err != nil {
		klog.Fatal("cannot start server:", err)
	}

	mountServer := server.NewMountServiceServer(getBlobfuseVersion())

	klog.V(2).Info("Listening for connections on address: %v\n", listener.Addr())
	if err = server.RunGRPCServer(mountServer, false, listener); err != nil {
		klog.Fatalf("Error running grpc server. Error: %v", listener.Addr(), err)
	}
}

func getOSInfo() (map[string]string, error) {
	f := "/etc/lsb-release"
	cfg, err := ini.Load(f)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read %q", f)
	}

	OSInfo := make(map[string]string)
	OSInfo["DISTRIB_ID"] = cfg.Section("").Key("DISTRIB_ID").String()
	OSInfo["DISTRIB_RELEASE"] = cfg.Section("").Key("DISTRIB_RELEASE").String()

	return OSInfo, nil
}

func getBlobfuseVersion() server.BlobfuseVersion {
	OSInfo, err := getOSInfo()
	if err != nil {
		klog.Errorf("failed to get OS info: %v, default using blobfuse v1", err)
		return server.BlobfuseV1
	}
	klog.Infof("OS info: %v", OSInfo)

	if OSInfo["DISTRIB_ID"] == "Ubuntu" && OSInfo["DISTRIB_RELEASE"] >= "22.04" {
		return server.BlobfuseV2
	}
	return server.BlobfuseV1
}
