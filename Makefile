GO_IMAGE ?= golang:1.25-alpine
GO_DOCKER ?= docker run --rm -v $(CURDIR):/src -w /src $(GO_IMAGE)

.PHONY: test vet build fmt fmt-check frontend-test frontend-build compose-config up down ci-local

test:
	$(GO_DOCKER) go test ./...

vet:
	$(GO_DOCKER) go vet ./...

build:
	$(GO_DOCKER) go build ./cmd/...

fmt:
	$(GO_DOCKER) go run ./scripts/format-go -w cmd internal scripts/format-go

fmt-check:
	$(GO_DOCKER) go run ./scripts/format-go cmd internal scripts/format-go

frontend-test:
	cd frontend && npm test

frontend-build:
	cd frontend && npm run build

compose-config:
	docker compose config --quiet

up:
	docker compose up --build

down:
	docker compose down

ci-local: test vet build fmt-check frontend-test frontend-build compose-config
