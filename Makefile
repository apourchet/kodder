PWD := ${CURDIR}

PACKAGE_NAME = github.com/apourchet/kodder
PACKAGE_VERSION ?= $(shell git describe --always --tags)
OS = $(shell uname)

ALL_SRC = $(shell find . -name "*.go" | grep -v -e vendor \
	-e ".*/\..*" \
	-e ".*/_.*" \
	-e ".*/mocks.*" \
	-e ".*/*.pb.go")
ALL_PKGS = $(shell go list $(sort $(dir $(ALL_SRC))) | grep -v vendor)
ALL_PKG_PATHS = $(shell go list -f '{{.Dir}}' ./...)
FMT_SRC = $(shell echo "$(ALL_SRC)" | tr ' ' '\n')
EXT_TOOLS = github.com/axw/gocov/gocov github.com/AlekSi/gocov-xml github.com/matm/gocov-html github.com/golang/mock/mockgen golang.org/x/lint/golint golang.org/x/tools/cmd/goimports github.com/client9/misspell/cmd/misspell
EXT_TOOLS_DIR = ext-tools/$(OS)
DEP_TOOL = $(EXT_TOOLS_DIR)/dep

GO_FLAGS = -gcflags '-N -l'
GO_VERSION = 1.11.1

REGISTRY ?= index.docker.io/apourchet

default: bins

### Targets to compile the binaries.
.PHONY: cbins bins
bins: bin/kodder

bin/kodder: $(ALL_SRC) vendor
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux go build -tags bins $(GO_FLAGS) -o $@ *.go

cbins:
	docker run -i --rm -v $(PWD):/go/src/$(PACKAGE_NAME) \
		--net=host \
		--entrypoint=bash \
		-w /go/src/$(PACKAGE_NAME) \
		golang:$(GO_VERSION) \
		-c "make bins"

$(ALL_SRC): ;



### Targets to install the dependencies.
$(DEP_TOOL):
	mkdir -p $(EXT_TOOLS_DIR)
	go get github.com/golang/dep/cmd/dep
	cp $(GOPATH)/bin/dep $(EXT_TOOLS_DIR)

vendor: $(DEP_TOOL) Gopkg.toml
	$(EXT_TOOLS_DIR)/dep ensure

cvendor:
	docker run --rm -v $(PWD):/go/src/$(PACKAGE_NAME) \
		-w /go/src/$(PACKAGE_NAME) \
		--entrypoint=/bin/sh \
		instrumentisto/dep \
		-c "dep ensure"

ext-tools: vendor $(EXT_TOOLS)

.PHONY: $(EXT_TOOLS)
$(EXT_TOOLS): vendor
	@echo "Installing external tool $@"
	@(ls $(EXT_TOOLS_DIR)/$(notdir $@) > /dev/null 2>&1) || GOBIN=$(PWD)/$(EXT_TOOLS_DIR) go install ./vendor/$@

mocks: ext-tools
	@echo "Generating mocks"

env: integration/requirements.txt
	[ -d env ] || virtualenv --setuptools env
	./env/bin/pip install -q -r integration/requirements.txt



### Target to build the docker image.
.PHONY: image publish
image:
	docker build -t $(REGISTRY)/kodder:$(PACKAGE_VERSION) -f Dockerfile .
	docker tag $(REGISTRY)/kodder:$(PACKAGE_VERSION) kodder:$(PACKAGE_VERSION)
	docker tag $(REGISTRY)/kodder:$(PACKAGE_VERSION) kodder:latest

publish: image
	docker push $(REGISTRY)/kodder:$(PACKAGE_VERSION) 



### Targets to test the codebase.
.PHONY: test unit-test integration cunit-test
test: unit-test integration

unit-test: $(ALL_SRC) vendor ext-tools mocks
	$(EXT_TOOLS_DIR)/gocov test $(ALL_PKGS) --tags "unit" | $(EXT_TOOLS_DIR)/gocov report

cunit-test: $(ALL_SRC)
	docker run -i --rm -v $(PWD):/go/src/$(PACKAGE_NAME) \
		--net=host \
		--entrypoint=bash \
		-w /go/src/$(PACKAGE_NAME) \
		golang:$(GO_VERSION) \
		-c "make ext-tools unit-test"

integration: env image
	PACKAGE_VERSION=$(PACKAGE_VERSION) ./env/bin/py.test --maxfail=1 --durations=6 --timeout=300 -vv integration 
