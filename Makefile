## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p "$(LOCALBIN)"

GOLANGCI_LINT = $(LOCALBIN)/golangci-lint
GOLANGCI_LINT_VERSION ?= v2.5.0
MARKITDOWN_VENV ?= $(shell pwd)/.venv-markitdown
MARKITDOWN_STAMP ?= $(MARKITDOWN_VENV)/.markitdown-all-ready
MARKITDOWN_VERSION ?= 0.1.5

.PHONY: build test test-integration lint fmt vet migrate-up migrate-down docker-up docker-down generate ensure-markitdown

build:
	go build -o postbrain ./cmd/postbrain
	go build -o postbrain-hook ./cmd/postbrain-hook

test: ensure-markitdown
	PATH="$(MARKITDOWN_VENV)/bin:$$PATH" go test -coverprofile=coverage.out -covermode=atomic ./...

test-integration: ensure-markitdown
	PATH="$(MARKITDOWN_VENV)/bin:$$PATH" go test -tags integration -coverprofile=coverage.out -covermode=atomic ./...

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

golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

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
