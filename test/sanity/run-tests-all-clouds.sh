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
  git clone https://github.com/kubernetes-csi/csi-test.git -b v4.3.0
  pushd csi-test/cmd/csi-sanity
  make install
  popd
  popd
}

function install_blobfuse_bin {
  echo 'Installing blobfuse...'
  apt-get update && apt install ca-certificates pkg-config libfuse-dev cmake libcurl4-gnutls-dev libgnutls28-dev uuid-dev libgcrypt20-dev wget -y
  wget https://packages.microsoft.com/config/ubuntu/16.04/packages-microsoft-prod.deb
  dpkg -i packages-microsoft-prod.deb
  apt-get update && apt install blobfuse fuse -y
  rm -f packages-microsoft-prod.deb
}

# copy blobfuse binary
if [[ "$cloud" == "AzureStackCloud" ]]; then
  install_blobfuse_bin
else
  mkdir -p /usr/blob
  cp test/artifacts/blobfuse /usr/bin/blobfuse
fi

apt update && apt install libfuse2 -y
if [[ -z "$(command -v csi-sanity)" ]]; then
	install_csi_sanity_bin
fi
test/sanity/run-test.sh "$nodeid"
