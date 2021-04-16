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

set -eo pipefail

readonly cloud="$1"

function install_csi_sanity_bin {
  echo 'Installing CSI sanity test binary...'
  mkdir -p $GOPATH/src/github.com/kubernetes-csi
  pushd $GOPATH/src/github.com/kubernetes-csi
  export GO111MODULE=off
  git clone https://github.com/kubernetes-csi/csi-test.git -b v4.0.2
  pushd csi-test/cmd/csi-sanity
  make install
  popd
  popd
}

function install_blobfuse_bin {
  echo 'Installing blobfuse...'
  apt-get update && apt install ca-certificates pkg-config libfuse-dev libfuse2 cmake libcurl4-gnutls-dev libgnutls28-dev uuid-dev libgcrypt20-dev libboost-all-dev gcc g++ -y
  mkdir -p $GOPATH/src/github.com/Azure
  pushd $GOPATH/src/github.com/Azure
  git clone https://github.com/Azure/azure-storage-fuse/
  pushd azure-storage-fuse
  ./build.sh
  cp build/blobfuse /usr/bin/blobfuse
  popd
  popd
}

install_blobfuse_bin

if [[ -z "$(command -v csi-sanity)" ]]; then
  install_csi_sanity_bin
fi
test/sanity/run-test.sh "$nodeid"
