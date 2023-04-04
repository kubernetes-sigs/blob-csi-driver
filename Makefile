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

PKG = sigs.k8s.io/blob-csi-driver
GIT_COMMIT ?= $(shell git rev-parse HEAD)
REGISTRY ?= andyzhangx
REGISTRY_NAME ?= $(shell echo $(REGISTRY) | sed "s/.azurecr.io//g")
IMAGE_NAME ?= blob-csi
IMAGE_VERSION ?= v1.21.0
CLOUD ?= AzurePublicCloud
# Use a custom version for E2E tests if we are in Prow
ifdef CI
ifndef PUBLISH
override IMAGE_VERSION := e2e-$(GIT_COMMIT)
endif
endif
CSI_IMAGE_TAG ?= $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_VERSION)
CSI_IMAGE_TAG_LATEST = $(REGISTRY)/$(IMAGE_NAME):latest
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS ?= "-X ${PKG}/pkg/blob.driverVersion=${IMAGE_VERSION} -X ${PKG}/pkg/blob.gitCommit=${GIT_COMMIT} -X ${PKG}/pkg/blob.buildDate=${BUILD_DATE} -s -w -extldflags '-static'"
E2E_HELM_OPTIONS ?= --set image.blob.pullPolicy=Always --set image.blob.repository=$(REGISTRY)/$(IMAGE_NAME) --set image.blob.tag=$(IMAGE_VERSION) --set driver.userAgentSuffix="e2e-test"
ifdef ENABLE_BLOBFUSE_PROXY
override E2E_HELM_OPTIONS := $(E2E_HELM_OPTIONS) --set controller.logLevel=6 --set node.logLevel=6 --set node.enableBlobfuseProxy=true
endif
E2E_HELM_OPTIONS += ${EXTRA_HELM_OPTIONS}
GO111MODULE = on
GOPATH ?= $(shell go env GOPATH)
GOBIN ?= $(GOPATH)/bin
DOCKER_CLI_EXPERIMENTAL = enabled
export GOPATH GOBIN GO111MODULE DOCKER_CLI_EXPERIMENTAL

# allow dpkg-deb to be run via docker
ifneq ("$(shell which dpkg-deb)", "")
DPKG_DEB := $(shell which dpkg-deb)
else
DPKG_DEB := docker run --rm -v $${PWD}:/src -w /src ubuntu dpkg-deb
endif

# The current context of image building
# The architecture of the image
ARCH ?= amd64
# Output type of docker buildx build
OUTPUT_TYPE ?= registry

ALL_ARCH.linux = amd64 arm64
ALL_OS_ARCH = $(foreach arch, ${ALL_ARCH.linux}, linux-$(arch))

all: blob blobfuse-proxy

.PHONY: verify
verify: unit-test
	hack/verify-all.sh

.PHONY: unit-test
unit-test:
	go test -covermode=count -coverprofile=profile.cov ./pkg/... ./test/utils/credentials

.PHONY: sanity-test
sanity-test: blob
	go test -v -timeout=30m ./test/sanity

.PHONY: integration-test
integration-test: blob
	go test -v -timeout=30m ./test/integration

.PHONY: e2e-test
e2e-test: install-ginkgo
	if [ ! -z "$(EXTERNAL_E2E_TEST_BLOBFUSE)" ] || [ ! -z "$(EXTERNAL_E2E_TEST_BLOBFUSE_v2)" ] || [ ! -z "$(EXTERNAL_E2E_TEST_NFS)" ]; then \
		bash ./test/external-e2e/run.sh;\
	else \
		ginkgo -p -vv --fail-fast --output-interceptor-mode=none --flake-attempts 2 ./test/e2e -- --project-root=$(shell pwd);\
	fi

.PHONY: e2e-bootstrap
e2e-bootstrap: install-helm
	# Only build and push the image if it does not exist in the registry
	docker pull $(CSI_IMAGE_TAG) || make blob-container push
	helm install blob-csi-driver ./charts/latest/blob-csi-driver --namespace kube-system --wait --timeout=15m -v=5 --debug \
		--set controller.replicas=1 \
		--set cloud=$(CLOUD) \
		$(E2E_HELM_OPTIONS)

.PHONY: install-helm
install-helm:
	curl https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3 | bash

.PHONY: install-ginkgo
install-ginkgo:
	go install github.com/onsi/ginkgo/v2/ginkgo@v2.9.2

.PHONY: e2e-teardown
e2e-teardown:
	helm delete blob-csi-driver --namespace kube-system

.PHONY: blob
blob: blobfuse-proxy
	CGO_ENABLED=0 GOOS=linux GOARCH=$(ARCH) go build -a -ldflags ${LDFLAGS} -mod vendor -o _output/${ARCH}/blobplugin ./pkg/blobplugin

.PHONY: blob-windows
blob-windows:
	CGO_ENABLED=0 GOOS=windows go build -a -ldflags ${LDFLAGS} -mod vendor -o _output/blobplugin.exe ./pkg/blobplugin

.PHONT: blob-darwin
blob-darwin:
	CGO_ENABLED=0 GOOS=darwin go build -a -ldflags ${LDFLAGS} -mod vendor -o _output/blobplugin ./pkg/blobplugin

.PHONY: container
container: blob
	docker build -t $(CSI_IMAGE_TAG) --output=type=docker -f ./pkg/blobplugin/Dockerfile .

.PHONY: container-linux
container-linux:
	docker buildx build --pull --output=type=$(OUTPUT_TYPE) --platform="linux/$(ARCH)" \
		--provenance=false --sbom=false \
		-t $(CSI_IMAGE_TAG)-linux-$(ARCH) --build-arg ARCH=$(ARCH) -f ./pkg/blobplugin/Dockerfile .

.PHONY: blob-container
blob-container:
	docker buildx rm container-builder || true
	docker buildx create --use --name=container-builder

ifeq ($(CLOUD), AzureStackCloud)
	docker run --privileged --name buildx_buildkit_container-builder0 -d --mount type=bind,src=/etc/ssl/certs,dst=/etc/ssl/certs moby/buildkit:latest || true
endif
	# enable qemu for arm64 build
	# https://github.com/docker/buildx/issues/464#issuecomment-741507760
	docker run --privileged --rm tonistiigi/binfmt --uninstall qemu-aarch64
	docker run --rm --privileged tonistiigi/binfmt --install all
	for arch in $(ALL_ARCH.linux); do \
		ARCH=$${arch} $(MAKE) blob; \
		ARCH=$${arch} $(MAKE) container-linux; \
	done

.PHONY: push
push:
ifdef CI
	docker manifest create --amend $(CSI_IMAGE_TAG) $(foreach osarch, $(ALL_OS_ARCH), $(CSI_IMAGE_TAG)-${osarch})
	docker manifest push --purge $(CSI_IMAGE_TAG)
	docker manifest inspect $(CSI_IMAGE_TAG)
else
	docker push $(CSI_IMAGE_TAG)
endif

.PHONY: push-latest
push-latest:
ifdef CI
	docker manifest create --amend $(CSI_IMAGE_TAG_LATEST) $(foreach osarch, $(ALL_OS_ARCH), $(CSI_IMAGE_TAG)-${osarch})
	docker manifest push --purge $(CSI_IMAGE_TAG_LATEST)
	docker manifest inspect $(CSI_IMAGE_TAG_LATEST)
else
	docker push $(CSI_IMAGE_TAG_LATEST)
endif

.PHONY: build-push
build-push: blob-container
	docker tag $(CSI_IMAGE_TAG) $(CSI_IMAGE_TAG_LATEST)
	docker push $(CSI_IMAGE_TAG_LATEST)

.PHONY: clean
clean:
	go clean -r -x
	-rm -rf _output

.PHONY: create-metrics-svc
create-metrics-svc:
	kubectl create -f deploy/example/metrics/csi-blob-controller-svc.yaml

.PHONY: delete-metrics-svc
delete-metrics-svc:
	kubectl delete -f deploy/example/metrics/csi-blob-controller-svc.yaml --ignore-not-found

.PHONY: blobfuse-proxy
blobfuse-proxy:
	CGO_ENABLED=0 GOOS=linux go build -mod vendor -ldflags="-s -w" -o _output/${ARCH}/blobfuse-proxy ./pkg/blobfuse-proxy
