# Copyright 2021 The Kubernetes Authors.
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

FROM mcr.microsoft.com/oss/nginx/nginx:1.17.3-alpine
RUN wget -O /tmp/packages-microsoft-prod.deb https://packages.microsoft.com/config/ubuntu/18.04/packages-microsoft-prod.deb
RUN wget -O /tmp/blobfuse-proxy-v0.1.0.deb https://github.com/kubernetes-sigs/blob-csi-driver/raw/master/deploy/blobfuse-proxy/v0.1.0/blobfuse-proxy-v0.1.0.deb

LABEL maintainers="andyzhangx"
LABEL description="Azure Blobfuse Proxy Installer"

