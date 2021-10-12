#!/bin/bash

# Copyright 2020 The Kubernetes Authors.
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

set -euo pipefail

ver="master"
if [[ "$#" -gt 0 ]]; then
  ver="$1"
fi

repo="https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/$ver/deploy"
if [[ "$#" -gt 1 ]]; then
  if [[ "$2" == *"local"* ]]; then
    echo "use local deploy"
    repo="./deploy"
  fi
fi

if [ $ver != "master" ]; then
  repo="$repo/$ver"
fi

echo "Installing Azure Blob Storage CSI driver, version: $ver ..."
kubectl apply -f $repo/rbac-csi-blob-controller.yaml
kubectl apply -f $repo/rbac-csi-blob-node.yaml
kubectl apply -f $repo/csi-blob-driver.yaml
kubectl apply -f $repo/csi-blob-controller.yaml

if [[ "$#" -gt 1 ]]; then
  if [[ "$2" == *"blobfuse-proxy"* ]]; then
    echo "set enable-blobfuse-proxy as true ..."
    kubectl apply -f ./deploy/blobfuse-proxy/blobfuse-proxy.yaml
    if [[ "$2" == *"local"* ]]; then
      cat $repo/csi-blob-node.yaml | sed 's/enable-blobfuse-proxy=false/enable-blobfuse-proxy=true/g' | kubectl apply -f -
    else
      curl -s $repo/csi-blob-node.yaml | sed 's/enable-blobfuse-proxy=false/enable-blobfuse-proxy=true/g' | kubectl apply -f -
    fi
  else
    kubectl apply -f $repo/csi-blob-node.yaml
  fi
fi
echo 'Azure Blob Storage CSI driver installed successfully.'
