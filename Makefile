GO_LDFLAGS := -X github.com/brightfellow-net/liturgist/server.Version=$(shell git describe --tags --always --dirty 2>/dev/null || echo dev) \
              -X github.com/brightfellow-net/liturgist/server.Commit=$(shell git rev-parse --short HEAD 2>/dev/null || echo unknown) \
              -X github.com/brightfellow-net/liturgist/server.BuildDate=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)

.PHONY: build web gen test test-pg e2e lint dev

build: gen web
	go build -ldflags "$(GO_LDFLAGS)" -o bin/liturgist ./cmd/liturgist

web: node_modules
	pnpm --filter web build

gen: node_modules
	go run ./cmd/liturgist openapi > packages/api-client/openapi.json
	pnpm --filter @liturgist/api-client gen

test:
	go test ./...
	pnpm -r test

test-pg:
	LITURGIST_TEST_POSTGRES=1 go test ./...

# Browser tests against the real binary (05 §9). Uses Playwright's Chromium
# ("pnpm --filter web exec playwright install chromium"), or an installed
# Google Chrome with PLAYWRIGHT_CHANNEL=chrome.
e2e: web
	pnpm --filter web e2e

lint: node_modules
	golangci-lint run ./...
	scripts/check-license.sh
	pnpm -r lint

# Unix shells only; on Windows run the two commands in two terminals.
dev: node_modules
	@echo "Go on :8080, Vite on :5173 (proxies /api to Go)"
	LITURGIST_LISTEN=127.0.0.1:8080 go run ./cmd/liturgist serve & pnpm --filter web dev; kill %1

node_modules: package.json pnpm-lock.yaml
	pnpm install --frozen-lockfile
	@touch node_modules
