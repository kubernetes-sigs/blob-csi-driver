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


set -euo pipefail

function cleanup {
  echo "pkill -f blobplugin"
  pkill -f blobplugin
}

readonly volname="citest-$(date +%s)"
readonly volsize="2147483648"
readonly expanded_vol_size="2147483650"
readonly endpoint="$1"
readonly staging_target_path="$2"
readonly target_path="$3"
readonly resource_group="$4"
readonly cloud="$5"

echo "Begin to run integration test on $cloud..."

# Run CSI driver as a background service
_output/blobplugin --endpoint "$endpoint" --nodeid CSINode --enable-blob-mock-mount -v=5 &
trap cleanup EXIT

if [[ "$cloud" == "AzureChinaCloud" ]]; then
  sleep 25
else
  sleep 5
fi

# Begin to run CSI functions one by one
echo "Create volume test:"
value="$(csc controller new --endpoint "$endpoint" --cap 1,block "$volname" --req-bytes "$volsize" --params skuname=Standard_LRS)"
sleep 15

volumeid="$(echo "$value" | awk '{print $1}' | sed 's/"//g')"
echo "Got volume id: $volumeid"
storage_account_name="$(echo "$volumeid" | awk -F# '{print $2}')"

csc controller validate-volume-capabilities --endpoint "$endpoint" --cap 1,block "$volumeid"

if [[ "$cloud" != "AzureChinaCloud" ]]; then
  echo "stage volume test:"
  csc node stage --endpoint "$endpoint" --cap 1,block --staging-target-path "$staging_target_path" "$volumeid"

  echo "publish volume test:"
  csc node publish --endpoint "$endpoint" --cap 1,block --staging-target-path "$staging_target_path" --target-path "$target_path" "$volumeid"
  sleep 2

  echo "node stats test:"
  csc node stats --endpoint "$endpoint" "$volumeid:$target_path:$staging_target_path"
  sleep 2

  echo 'expand volume test'
  csc controller expand-volume --endpoint "$endpoint" --req-bytes "$expanded_vol_size" "$volumeid"

  echo "unpublish volume test:"
  csc node unpublish --endpoint "$endpoint" --target-path "$target_path" "$volumeid"
  sleep 2

  echo "unstage volume test:"
  csc node unstage --endpoint "$endpoint" --staging-target-path "$staging_target_path" "$volumeid"
  sleep 2
fi

echo "Delete volume test:"
csc controller del --endpoint "$endpoint" "$volumeid"
sleep 15

echo "Create volume in storage account($storage_account_name) under resource group($resource_group):"
value="$(csc controller new --endpoint "$endpoint" --cap 1,block "$volname" --req-bytes "$volsize" --params skuname=Standard_LRS,storageAccount=$storage_account_name,resourceGroup=$resource_group)"
sleep 15

volumeid="$(echo "$value" | awk '{print $1}' | sed 's/"//g')"
echo "got volume id: $volumeid"

echo "Delete volume test:"
csc controller del --endpoint "$endpoint" "$volumeid"
sleep 15

csc identity plugin-info --endpoint "$endpoint"
csc node get-info --endpoint "$endpoint"

echo "Integration test on $cloud is completed."
