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

FROM k8s.gcr.io/build-image/debian-base:bullseye-v1.0.0
COPY ./_output/blobplugin /blobplugin
RUN apt update && apt-mark unhold libcap2
RUN clean-install ca-certificates libfuse-dev libcurl4-gnutls-dev libgnutls28-dev uuid-dev libgcrypt20-dev util-linux mount udev wget e2fsprogs nfs-common libssl1.1
RUN wget -O /tmp/packages-microsoft-prod.deb https://packages.microsoft.com/config/ubuntu/18.04/packages-microsoft-prod.deb && dpkg -i /tmp/packages-microsoft-prod.deb && apt-get update && apt install blobfuse fuse -y && rm -f /tmp/packages-microsoft-prod.deb
# this is a workaround to install nfs-kernel-server and don't quit with error
RUN apt install nfs-kernel-server -y || true
RUN apt remove wget -y
LABEL maintainers="andyzhangx"
LABEL description="Azure Blob Storage CSI driver"

ENTRYPOINT ["/blobplugin"]

