#!/bin/bash

# Copyright 2018 The Kubernetes Authors.
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

echo "verifying .deb package"
PKG_ROOT=$(git rev-parse --show-toplevel)

INITIAL_MD5SUM=$(md5sum ${PKG_ROOT}/deploy/blobfuse-proxy/v0.1.0/blobfuse-proxy-v0.1.0.deb | awk '{print $1}')
INITIAL_MD5SUM=
echo "creating a new .deb package"
mkdir -p ${PKG_ROOT}/pkg/blobfuse-proxy/debpackage/usr/bin
cp ${PKG_ROOT}/_output/blobfuse-proxy ${PKG_ROOT}/pkg/blobfuse-proxy/debpackage/usr/bin/blobfuse-proxy
dpkg-deb --build ${PKG_ROOT}/pkg/blobfuse-proxy/debpackage
FINAL_MD5SUM=$(md5sum ${PKG_ROOT}/pkg/blobfuse-proxy/debpackage.deb | awk '{print $1}')

if [[ "$FINAL_MD5SUM" == "$INITIAL_MD5SUM" ]]; then
    echo "md5sum matches"
else
    echo "md5sum doesnt match. Please update with the latest .deb package"
    exit 1
fi
