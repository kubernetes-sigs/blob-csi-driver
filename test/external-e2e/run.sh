#!/bin/bash

# Copyright 2021 The Kubernetes Authors.
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

set -xe

PROJECT_ROOT=$(git rev-parse --show-toplevel)

install_ginkgo () {
    apt update -y
    apt install -y golang-ginkgo-dev
}

setup_e2e_binaries() {
    # download k8s external e2e binary
    curl -sL https://storage.googleapis.com/kubernetes-release/release/v1.21.0/kubernetes-test-linux-amd64.tar.gz --output e2e-tests.tar.gz
    tar -xvf e2e-tests.tar.gz && rm e2e-tests.tar.gz

    if [ ! -z ${EXTERNAL_E2E_TEST_NFS} ]; then
        # enable fsGroupPolicy (only available from k8s 1.20)
        export EXTRA_HELM_OPTIONS="--set feature.enableFSGroupPolicy=true"
    fi

    # install csi driver
    make e2e-bootstrap
    make create-metrics-svc
}

print_logs() {
    echo "print out driver logs ..."
    bash ./test/utils/blob_log.sh
}

install_ginkgo
setup_e2e_binaries
trap print_logs EXIT

mkdir -p /tmp/csi

if [ ! -z ${EXTERNAL_E2E_TEST_BLOBFUSE} ]; then
    echo "begin to run blobfuse tests ...."
    cp deploy/example/storageclass-blobfuse.yaml /tmp/csi/storageclass.yaml
    ginkgo -p --progress --v -focus='External.Storage.*blob.csi.azure.com' \
        -skip='\[Disruptive\]|\[Slow\]|allow exec of files on the volume|unmount after the subpath directory is deleted' kubernetes/test/bin/e2e.test  -- \
        -storage.testdriver=$PROJECT_ROOT/test/external-e2e/testdriver-blobfuse.yaml \
        --kubeconfig=$KUBECONFIG
fi

if [ ! -z ${EXTERNAL_E2E_TEST_NFS} ]; then
    echo "begin to run NFSv3 tests ...."
    cp deploy/example/storageclass-blob-nfs.yaml /tmp/csi/storageclass.yaml
    ginkgo -p --progress --v -focus='External.Storage.*blob.csi.azure.com' \
        -skip='\[Disruptive\]|\[Slow\]' kubernetes/test/bin/e2e.test  -- \
        -storage.testdriver=$PROJECT_ROOT/test/external-e2e/testdriver-nfs.yaml \
        --kubeconfig=$KUBECONFIG
fi