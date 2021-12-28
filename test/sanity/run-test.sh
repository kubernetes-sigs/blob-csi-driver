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

function cleanup {
  echo 'pkill -f blobplugin'
  pkill -f blobplugin
  echo 'Deleting CSI sanity test binary'
  rm -rf csi-test
}

trap cleanup EXIT

readonly controllerendpoint="unix:///tmp/csi-controller.sock"
readonly nodeendpoint="unix:///tmp/csi-node.sock"
nodeid="CSINode"
if [[ "$#" -gt 0 ]] && [[ -n "$1" ]]; then
  nodeid="$1"
fi

_output/amd64/blobplugin --endpoint "$controllerendpoint" -v=5 &
_output/amd64/blobplugin --endpoint "$nodeendpoint" --nodeid "$nodeid" --enable-blob-mock-mount -v=5 &

echo "Begin to run sanity test..."
readonly CSI_SANITY_BIN='csi-sanity'
"$CSI_SANITY_BIN" --ginkgo.v --csi.endpoint=$nodeendpoint --csi.controllerendpoint=$controllerendpoint -ginkgo.skip="should fail when requesting to create a volume with already existing name and different capacity|should be idempotent|should return appropriate capabilities"
