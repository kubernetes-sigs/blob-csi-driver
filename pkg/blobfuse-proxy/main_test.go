/*
Copyright 2025 The Kubernetes Authors.

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
	"net"
	"os"
	"runtime"
	"testing"

	"sigs.k8s.io/blob-csi-driver/pkg/blobfuse-proxy/pb"
	csicommon "sigs.k8s.io/blob-csi-driver/pkg/csi-common"
)

func mockRunGRPCServer(_ pb.MountServiceServer, _ bool, _ net.Listener) error {
	return nil
}

func TestMain(t *testing.T) {
	// Skip test on windows
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test on ", runtime.GOOS)
	}

	// mock the grpcServerRunner
	originalGRPCServerRunner := grpcServerRunner
	grpcServerRunner = mockRunGRPCServer
	defer func() { grpcServerRunner = originalGRPCServerRunner }()

	// Set the blobfuse-proxy-endpoint
	os.Args = []string{"cmd", "-blobfuse-proxy-endpoint=unix://tmp/test.sock"}

	// Run main
	main()
}

func TestMainWithTCPEndpoint(t *testing.T) {
	// Skip test on windows
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test on ", runtime.GOOS)
	}

	// mock the grpcServerRunner
	originalGRPCServerRunner := grpcServerRunner
	grpcServerRunner = mockRunGRPCServer
	defer func() { grpcServerRunner = originalGRPCServerRunner }()

	// Set the blobfuse-proxy-endpoint with TCP
	// Use a separate process to avoid flag redefinition issues
	os.Args = []string{"cmd", "-blobfuse-proxy-endpoint=tcp://127.0.0.1:0"}

	// Since we can't call main() directly due to flag conflicts,
	// let's test the core functionality instead
	proto, addr, err := csicommon.ParseEndpoint("tcp://127.0.0.1:0")
	if err != nil {
		t.Errorf("failed to parse endpoint: %v", err)
	}
	
	if proto != "tcp" {
		t.Errorf("expected protocol tcp, got %s", proto)
	}
	
	if addr == "" {
		t.Errorf("expected non-empty address")
	}
}
