.PHONY: build test lint check web-build web-test provision-build ops-build acceptance
acceptance:
	go test ./tests/acceptance -count=1 -timeout=60s -v
ops-build:
	go build -o bin/edge-ops ./cmd/edge-ops
provision-build:
	go build -o bin/edge-provision ./cmd/edge-provision
web-build:
	cd web && npm ci && npm run build
web-test:
	cd web && npm ci && npm run check && npm test
build:
	go build -o bin/edge-agent ./cmd/edge-agent
test:
	go test -race ./...
lint:
	go vet ./...
	@test -z "$$(gofmt -l cmd internal)" || (echo 'Run gofmt on Go sources'; exit 1)
check: lint test
