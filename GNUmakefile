PKG_NAME    := github.com/uptrace/terraform
BINARY      := terraform-provider-uptrace
VERSION     ?= dev
LDFLAGS     := -X $(PKG_NAME)/version.ProviderVersion=$(VERSION)

GO          ?= go
OS_ARCH     := $(shell $(GO) env GOOS)_$(shell $(GO) env GOARCH)
GOFMT_FILES := $(shell find . -name '*.go' -not -path './vendor/*' -not -path './internal/generated/*')

.PHONY: default
default: build

# Vendor the API spec from an uptrace/app checkout at a ref, for example
# make sync-spec APP_DIR=../app APP_REF=origin/prod. Then run make generate.
.PHONY: sync-spec
sync-spec:
	scripts/sync-spec.sh $(APP_DIR) $(APP_REF)

.PHONY: generate
generate:
	rm -f internal/generated/*.go
	$(GO) tool oapi-codegen \
		-config oapi-codegen.yaml openapi/openapi.yaml

.PHONY: docs
docs:
	tfplugindocs generate --provider-name uptrace

.PHONY: build
build:
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BINARY) .

.PHONY: install
install: build
	mkdir -p ~/.terraform.d/plugins/registry.terraform.io/uptrace/uptrace/$(VERSION)/$(OS_ARCH)
	cp $(BINARY) ~/.terraform.d/plugins/registry.terraform.io/uptrace/uptrace/$(VERSION)/$(OS_ARCH)/

.PHONY: test
test:
	$(GO) test ./... -count=1

.PHONY: testacc
testacc:
	TF_ACC=1 $(GO) test ./... -v -count=1 -timeout 10m

VET_PKGS := $(shell $(GO) list ./... | grep -v /internal/generated)

.PHONY: vet
vet:
	$(GO) vet $(VET_PKGS)

.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: fmt
fmt:
	gofmt -s -w $(GOFMT_FILES)

.PHONY: fmtcheck
fmtcheck:
	@unformatted=$$(gofmt -s -l $(GOFMT_FILES)); \
	if [ -n "$$unformatted" ]; then \
		echo "The following files are not gofmt'd:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

.PHONY: tidy
tidy:
	$(GO) mod tidy

.PHONY: clean
clean:
	rm -f $(BINARY)
