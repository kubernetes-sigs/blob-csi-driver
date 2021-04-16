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

GO111MODULE=off go get github.com/rexray/gocsi/csc

function install_blobfuse_bin {
  echo 'Installing blobfuse...'
  apt-get update && apt install ca-certificates pkg-config libfuse-dev cmake libcurl4-gnutls-dev libgnutls28-dev uuid-dev libgcrypt20-dev wget -y
  wget https://packages.microsoft.com/config/ubuntu/16.04/packages-microsoft-prod.deb
  dpkg -i packages-microsoft-prod.deb
  apt-get update && apt install blobfuse fuse -y
  rm -f packages-microsoft-prod.deb
}

readonly resource_group="$1"
readonly cloud="$2"

install_blobfuse_bin

apt update && apt install libfuse2 -y
test/integration/run-test.sh "tcp://127.0.0.1:10000" "/tmp/stagingtargetpath" "/tmp/targetpath" "$resource_group" "$cloud"
