.PHONY: build test test-integration lint fmt migrate-up migrate-down docker-up docker-down generate

build:
	go build -o postbrain ./cmd/postbrain
	go build -o postbrain-hook ./cmd/postbrain-hook

test:
	go test -coverprofile=coverage.out -covermode=atomic ./...

test-integration:
	go test -tags integration -coverprofile=coverage.out -covermode=atomic ./...

lint:
	golangci-lint run ./...

fmt:
	gofmt -w .

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
