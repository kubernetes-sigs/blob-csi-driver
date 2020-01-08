# Copyright 2017 The Kubernetes Authors.
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

PKG=sigs.k8s.io/blobfuse-csi-driver
REGISTRY_NAME?=andyzhangx
IMAGE_NAME=blobfuse-csi
IMAGE_VERSION=v0.4.0
IMAGE_TAG=$(REGISTRY_NAME)/$(IMAGE_NAME):$(IMAGE_VERSION)
IMAGE_TAG_LATEST=$(REGISTRY_NAME)/$(IMAGE_NAME):latest
GIT_COMMIT?=$(shell git rev-parse HEAD)
BUILD_DATE?=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS?="-X ${PKG}/pkg/blobfuse.driverVersion=${IMAGE_VERSION} -X ${PKG}/pkg/blobfuse.gitCommit=${GIT_COMMIT} -X ${PKG}/pkg/blobfuse.buildDate=${BUILD_DATE} -s -w -extldflags '-static'"
GO111MODULE = off
export GO111MODULE

all: blobfuse

.PHONY: verify
verify:
	hack/verify-all.sh

.PHONY: unit-test
unit-test:
	go test -covermode=count -coverprofile=profile.cov ./pkg/... ./test/utils/credentials

.PHONY: sanity-test
sanity-test: blobfuse
	go test -v -timeout=30m ./test/sanity

.PHONY: integration-test
integration-test: blobfuse
	go test -v -timeout=30m ./test/integration

.PHONY: e2e-test
e2e-test:
	test/e2e/run-test.sh

.PHONY: blobfuse
blobfuse:
	if [ ! -d ./vendor ]; then dep ensure -vendor-only; fi
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags ${LDFLAGS} -o _output/blobfuseplugin ./pkg/blobfuseplugin

.PHONY: blobfuse-windows
blobfuse-windows:
	if [ ! -d ./vendor ]; then dep ensure -vendor-only; fi
	CGO_ENABLED=0 GOOS=windows go build -a -ldflags ${LDFLAGS} -o _output/blobfuseplugin.exe ./pkg/blobfuseplugin

.PHONY: blobfuse-container
blobfuse-container: blobfuse
	docker build --no-cache -t $(IMAGE_TAG) -f ./pkg/blobfuseplugin/Dockerfile .

.PHONY: push
push: blobfuse-container
	docker push $(IMAGE_TAG)

.PHONY: push-latest
push-latest: blobfuse-container
	docker push $(IMAGE_TAG)
	docker tag $(IMAGE_TAG) $(IMAGE_TAG_LATEST)
	docker push $(IMAGE_TAG_LATEST)

.PHONY: clean
clean:
	go clean -r -x
	-rm -rf _output
