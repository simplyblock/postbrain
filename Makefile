## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p "$(LOCALBIN)"

GOLANGCI_LINT = $(LOCALBIN)/golangci-lint
GOLANGCI_LINT_VERSION ?= v2.5.0
GO_JUNIT_REPORT = $(LOCALBIN)/go-junit-report
GO_JUNIT_REPORT_VERSION ?= v1.0.0
GOPLS = $(LOCALBIN)/gopls
GOPLS_VERSION ?= v0.21.1
MARKITDOWN_VENV ?= $(shell pwd)/.venv-markitdown
MARKITDOWN_STAMP ?= $(MARKITDOWN_VENV)/.markitdown-all-ready
MARKITDOWN_VERSION ?= 0.1.5
DIST_DIR ?= $(shell pwd)/dist
TARGET_OSES ?= linux darwin windows
TARGET_ARCHES ?= amd64 arm64

.PHONY: build build-target build-cross build-archives package-target package-init test test-integration test-scope-authz test-scope-authz-integration lint fmt vet migrate-up migrate-down docker-up docker-down docker-build generate ensure-markitdown ensure-gopls

build:
	go build -o postbrain ./cmd/postbrain
	go build -o postbrain-hook ./cmd/postbrain-cli
	go build -o postbrain-cli ./cmd/postbrain-cli

build-target:
	@if [ -z "$(GOOS)" ] || [ -z "$(GOARCH)" ]; then \
		echo "GOOS and GOARCH are required, e.g. make build-target GOOS=linux GOARCH=amd64"; \
		exit 1; \
	fi
	@out="$(DIST_DIR)/$(GOOS)-$(GOARCH)"; \
	ext=""; \
	if [ "$(GOOS)" = "windows" ]; then ext=".exe"; fi; \
	mkdir -p "$$out"; \
	echo "building postbrain for $(GOOS)/$(GOARCH)"; \
	CGO_ENABLED=$${CGO_ENABLED:-0} GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o "$$out/postbrain$$ext" ./cmd/postbrain; \
	CGO_ENABLED=$${CGO_ENABLED:-0} GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o "$$out/postbrain-cli$$ext" ./cmd/postbrain-cli

build-cross:
	@rm -rf "$(DIST_DIR)"
	@for goos in $(TARGET_OSES); do \
		for goarch in $(TARGET_ARCHES); do \
			$(MAKE) build-target GOOS=$$goos GOARCH=$$goarch CGO_ENABLED=0; \
		done; \
	done

build-archives: build-cross
	@for goos in $(TARGET_OSES); do \
		for goarch in $(TARGET_ARCHES); do \
			$(MAKE) package-target GOOS=$$goos GOARCH=$$goarch; \
		done; \
	done

package-target:
	@if [ -z "$(GOOS)" ] || [ -z "$(GOARCH)" ]; then \
		echo "GOOS and GOARCH are required, e.g. make package-target GOOS=linux GOARCH=amd64"; \
		exit 1; \
	fi
	@target="$(DIST_DIR)/$(GOOS)-$(GOARCH)"; \
	ext=""; \
	if [ "$(GOOS)" = "windows" ]; then ext=".exe"; fi; \
	server_base="postbrain-server_$(GOOS)_$(GOARCH)"; \
	client_base="postbrain-client_$(GOOS)_$(GOARCH)"; \
	server_tmpdir="$$(mktemp -d)"; \
	client_tmpdir="$$(mktemp -d)"; \
	cp "$$target/postbrain$$ext" "$$server_tmpdir/"; \
	cp config.example.yaml "$$server_tmpdir/config.example.yaml"; \
	cp "$$target/postbrain-cli$$ext" "$$client_tmpdir/"; \
	if [ "$(GOOS)" = "windows" ]; then \
		( cd "$$server_tmpdir" && zip -q "$(DIST_DIR)/$$server_base.zip" "postbrain$$ext" "config.example.yaml" ); \
		( cd "$$client_tmpdir" && zip -q "$(DIST_DIR)/$$client_base.zip" "postbrain-cli$$ext" ); \
	else \
		tar -czf "$(DIST_DIR)/$$server_base.tar.gz" -C "$$server_tmpdir" "postbrain$$ext" "config.example.yaml"; \
		tar -czf "$(DIST_DIR)/$$client_base.tar.gz" -C "$$client_tmpdir" "postbrain-cli$$ext"; \
	fi; \
	rm -rf "$$server_tmpdir" "$$client_tmpdir"

package-init:
	@echo "Initial split package manifests are in packaging/ for postbrain-server and postbrain-client."

test: ensure-markitdown go-junit-report
	PATH="$(MARKITDOWN_VENV)/bin:$$PATH" go test -coverprofile=coverage.out -covermode=atomic -v 2>&1 ./... | $(GO_JUNIT_REPORT) -set-exit-code > report.xml

test-integration: ensure-markitdown go-junit-report ensure-gopls
	PATH="$(MARKITDOWN_VENV)/bin:$$PATH" go test -tags integration -coverprofile=coverage.out -covermode=atomic -v 2>&1 ./... | $(GO_JUNIT_REPORT) -set-exit-code > report.xml

test-scope-authz: go-junit-report
	go test -v 2>&1 ./internal/api/scopeauth ./internal/memory | $(GO_JUNIT_REPORT) -set-exit-code > report-scope-authz-unit.xml

test-scope-authz-integration: go-junit-report
	go test -tags integration -v 2>&1 ./internal/api/rest ./internal/api/mcp -run "Test(REST|MCP)_ScopeAuthz_|TestREST_Recall_IntersectsFanOutWithPrincipalScopes" | $(GO_JUNIT_REPORT) -set-exit-code > report-scope-authz-integration.xml

lint: golangci-lint
	"$(GOLANGCI_LINT)" run ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

migrate-up:
	./postbrain migrate up --config config.yaml

migrate-down:
	./postbrain migrate down 1 --config config.yaml

docker-up:
	docker compose up -d postgres

docker-down:
	docker compose down

docker-build:
	docker build \
		--build-arg GOPLS_VERSION=$(GOPLS_VERSION) \
		--build-arg MARKITDOWN_VERSION=$(MARKITDOWN_VERSION) \
		-t postbrain:latest .

generate:
	sqlc generate

ensure-markitdown:
	@if [ ! -x "$(MARKITDOWN_VENV)/bin/python" ]; then \
		echo "creating markitdown venv at $(MARKITDOWN_VENV)"; \
		python3 -m venv "$(MARKITDOWN_VENV)"; \
	fi
	@if [ ! -f "$(MARKITDOWN_STAMP)" ]; then \
		echo "installing markitdown[all]==$(MARKITDOWN_VERSION) into $(MARKITDOWN_VENV)"; \
		"$(MARKITDOWN_VENV)/bin/python" -m pip install --upgrade pip "markitdown[all]==$(MARKITDOWN_VERSION)"; \
		touch "$(MARKITDOWN_STAMP)"; \
	else \
		echo "using markitdown from $(MARKITDOWN_VENV)"; \
	fi

ensure-gopls: gopls
	@echo "using gopls from $(GOPLS)"

golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

go-junit-report: $(GO_JUNIT_REPORT) ## Download go-junit-report locally if necessary
$(GO_JUNIT_REPORT): $(LOCALBIN)
	$(call go-install-tool,$(GO_JUNIT_REPORT),github.com/jstemmer/go-junit-report,$(GO_JUNIT_REPORT_VERSION))

gopls: $(GOPLS) ## Download gopls locally if necessary
$(GOPLS): $(LOCALBIN)
	$(call go-install-tool,$(GOPLS),golang.org/x/tools/gopls,$(GOPLS_VERSION))

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] && [ "$$(readlink -- "$(1)" 2>/dev/null)" = "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f "$(1)" ;\
GOBIN="$(LOCALBIN)" go install $${package} ;\
mv "$(LOCALBIN)/$$(basename "$(1)")" "$(1)-$(3)" ;\
} ;\
ln -sf "$$(realpath "$(1)-$(3)")" "$(1)"
endef
