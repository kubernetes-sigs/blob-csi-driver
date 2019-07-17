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

if [ -v GOPATH ]; then
		mkdir $GOPATH/src/github.com/kubernetes-csi
		pushd $GOPATH/src/github.com/kubernetes-csi
		git clone https://github.com/kubernetes-csi/csi-test.git -b v1.1.0
        pushd $GOPATH/src/github.com/kubernetes-csi/csi-test/cmd/csi-sanity
		make && make install 
		popd
		popd
fi

endpoint="unix:///tmp/csi.sock"

echo "being to run sanity test ..."

sudo _output/blobfuseplugin --endpoint $endpoint --nodeid CSINode -v=5 &

sudo $GOPATH/src/github.com/kubernetes-csi/csi-test/cmd/csi-sanity/csi-sanity --ginkgo.v --csi.endpoint=$endpoint -ginkgo.skip='should fail when requesting to create a volume with already existing name and different capacity'

retcode=$?

if [ $retcode -ne 0 ]; then
	exit $retcode
fi

# kill blobfuseplugin first
echo "pkill -f blobfuseplugin"
sudo /usr/bin/pkill -f blobfuseplugin

echo "sanity test is completed."
