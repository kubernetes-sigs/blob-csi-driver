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
DRIVER="test"

setup_e2e_binaries() {
    # download k8s external e2e binary
    curl -sL https://storage.googleapis.com/kubernetes-release/release/v1.26.0/kubernetes-test-linux-amd64.tar.gz --output e2e-tests.tar.gz
    tar -xvf e2e-tests.tar.gz && rm e2e-tests.tar.gz

    export EXTRA_HELM_OPTIONS="--set driver.name=$DRIVER.csi.azure.com --set controller.name=csi-$DRIVER-controller --set node.name=csi-$DRIVER-node --set driver.azureGoSDKLogLevel=INFO"
    if [ ! -z ${EXTERNAL_E2E_TEST_NFS} ]; then
        # enable fsGroupPolicy (only available from k8s 1.20)
        export EXTRA_HELM_OPTIONS=$EXTRA_HELM_OPTIONS" --set feature.enableFSGroupPolicy=true"
    fi

     # test on alternative driver name
    sed -i "s/blob.csi.azure.com/$DRIVER.csi.azure.com/g" deploy/example/storageclass-blobfuse.yaml
    sed -i "s/blob.csi.azure.com/$DRIVER.csi.azure.com/g" deploy/example/storageclass-blobfuse2.yaml
    sed -i "s/blob.csi.azure.com/$DRIVER.csi.azure.com/g" deploy/example/storageclass-blob-nfs.yaml
    make e2e-bootstrap
    sed -i "s/csi-blob-controller/csi-$DRIVER-controller/g" deploy/example/metrics/csi-blob-controller-svc.yaml
    make create-metrics-svc
}

print_logs() {
    bash ./hack/verify-examples.sh
    echo "print out driver logs ..."
    bash ./test/utils/blob_log.sh $DRIVER
}

setup_e2e_binaries
trap print_logs EXIT

mkdir -p /tmp/csi

if [ ! -z ${EXTERNAL_E2E_TEST_BLOBFUSE} ]; then
    echo "begin to run blobfuse v1 tests ...."
    cp deploy/example/storageclass-blobfuse.yaml /tmp/csi/storageclass.yaml
    # achieve close-to-open cache consistency like in NFSv3
    sed -i 's/file-cache-timeout-in-seconds=120/file-cache-timeout-in-seconds=0/g' /tmp/csi/storageclass.yaml
    ginkgo -p -v --fail-fast --flake-attempts 2 -focus="External.Storage.*$DRIVER.csi.azure.com" \
        -skip='\[Disruptive\]|allow exec of files on the volume|unmount after the subpath directory is deleted|should concurrently access the single volume from pods on different node|pod created with an initial fsgroup, volume contents ownership changed via chgrp in first pod, new pod with same fsgroup skips ownership changes to the volume contents|should provision storage with any volume data source|should mount multiple PV pointing to the same storage on the same node' kubernetes/test/bin/e2e.test  -- \
        -storage.testdriver=$PROJECT_ROOT/test/external-e2e/testdriver-blobfuse.yaml \
        --kubeconfig=$KUBECONFIG
fi

if [ ! -z ${EXTERNAL_E2E_TEST_BLOBFUSE_v2} ]; then
    echo "begin to run blobfuse v2 tests ...."
    cp deploy/example/storageclass-blobfuse2.yaml /tmp/csi/storageclass.yaml
    # achieve close-to-open cache consistency like in NFSv3
    sed -i 's/file-cache-timeout-in-seconds=120/file-cache-timeout-in-seconds=0/g' /tmp/csi/storageclass.yaml
    ginkgo -p -v --fail-fast --flake-attempts 2 -focus="External.Storage.*$DRIVER.csi.azure.com" \
        -skip='\[Disruptive\]|allow exec of files on the volume|unmount after the subpath directory is deleted|should concurrently access the single volume from pods on different node|pod created with an initial fsgroup, volume contents ownership changed via chgrp in first pod, new pod with same fsgroup skips ownership changes to the volume contents|should provision storage with any volume data source|should mount multiple PV pointing to the same storage on the same node' kubernetes/test/bin/e2e.test  -- \
        -storage.testdriver=$PROJECT_ROOT/test/external-e2e/testdriver-blobfuse.yaml \
        --kubeconfig=$KUBECONFIG
fi

if [ ! -z ${EXTERNAL_E2E_TEST_NFS} ]; then
    echo "begin to run NFSv3 tests ...."
    cp deploy/example/storageclass-blob-nfs.yaml /tmp/csi/storageclass.yaml
    ginkgo -p -v --fail-fast --flake-attempts 2 -focus="External.Storage.*$DRIVER.csi.azure.com" \
        -skip='\[Disruptive\]|should concurrently access the single volume from pods on different node|pod created with an initial fsgroup, volume contents ownership changed via chgrp in first pod, new pod with same fsgroup skips ownership changes to the volume contents|should provision storage with any volume data source|should mount multiple PV pointing to the same storage on the same node|should access to two volumes with the same volume mode and retain data across pod recreation on different node' kubernetes/test/bin/e2e.test  -- \
        -storage.testdriver=$PROJECT_ROOT/test/external-e2e/testdriver-nfs.yaml \
        --kubeconfig=$KUBECONFIG
fi
