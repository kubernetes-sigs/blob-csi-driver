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

FROM mcr.microsoft.com/aks/fundamental/base-ubuntu:v0.0.5
RUN wget -O /tmp/packages-microsoft-prod.deb https://packages.microsoft.com/config/ubuntu/16.04/packages-microsoft-prod.deb
RUN dpkg -i /tmp/packages-microsoft-prod.deb && apt-get update && apt-get install -y ca-certificates pkg-config libfuse-dev cmake libcurl4-gnutls-dev libgnutls28-dev uuid-dev libgcrypt20-dev blobfuse nfs-common
LABEL maintainers="andyzhangx"
LABEL description="Azure Blob Storage CSI driver"

# Create a nonroot user
RUN useradd -u 10001 nonroot
USER nonroot

COPY ./_output/blobplugin /blobplugin
ENTRYPOINT ["/blobplugin"]
