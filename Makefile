.PHONY: build test lint fmt migrate-up migrate-down docker-up docker-down generate

build:
	go build ./cmd/postbrain ./cmd/postbrain-hook

test:
	go test ./...

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
