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

kustomize_path="blob-csi-driver"
if [[ "$#" -gt 1 ]]; then
  if [[ "$2" == *"blobfuse-proxy"* ]]; then
    echo "enable blobfuse-proxy ..."
    kustomize_path="with-blobfuse-proxy"
  fi
fi

repo="https://github.com/kubernetes-sigs/blob-csi-driver/deploy/${kustomize_path}?ref=$ver"
if [[ "$#" -gt 1 ]]; then
  if [[ "$2" == *"local"* ]]; then
    echo "use local deploy"
    repo="./deploy/${kustomize_path}"
    if [ "$ver" != "master" ]; then
       repo="./deploy/$ver/${kustomize_path}"
    fi
  fi
fi


echo "Installing Azure Blob Storage CSI driver, version: $ver ..."

kubectl apply -k "$repo"

echo 'Azure Blob Storage CSI driver installed successfully.'
