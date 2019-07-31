#!/bin/bash

# Copyright 2019 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -uo pipefail

kubectl get daemonsets csi-blobfuse-node -n kube-system; checkBlobFuseDriver=$?;
if [ $checkBlobFuseDriver -ne 0 ]; then
    echo "BlobFuse csi driver daemonset not found";
    echo "Installing BlobFuse csi driver";
    deploy/install-driver.sh
    echo "BlobFuse csi driver installed";    
fi

# Fetching ginkgo for running the test
GO111MODULE=off go get -u github.com/onsi/ginkgo/ginkgo
# Exporting KUBECONFIG path
export KUBECONFIG=$HOME/.kube/config
# Running the e2e test
ginkgo test/e2e
# Checking for test status
TEST_PASS=$?
if [[ $TEST_PASS -ne 0 ]]; then
    exit 1
fi
