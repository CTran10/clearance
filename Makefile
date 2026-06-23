GO_IMAGE ?= golang:1.25-alpine
GO_DOCKER ?= docker run --rm -v $(CURDIR):/src -w /src $(GO_IMAGE)

.PHONY: test vet fmt frontend-test compose-config up down ci-local

test:
	$(GO_DOCKER) go test ./...

vet:
	$(GO_DOCKER) go vet ./...

fmt:
	$(GO_DOCKER) gofmt -w cmd internal

frontend-test:
	cd frontend && npm test

compose-config:
	docker compose config

up:
	docker compose up --build

down:
	docker compose down

ci-local: test vet frontend-test compose-config
