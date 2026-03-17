.PHONY: generate build install test spec-update clean lint docs-generate docs-dev docs-build docs

VERSION ?= dev
LDFLAGS := -s -w -X github.com/sofq/jira-cli/cmd.Version=$(VERSION)
SPEC_URL := https://dac-static.atlassian.com/cloud/jira/platform/swagger-v3.v3.json

generate:
	go run ./gen/...

build: generate
	go build -ldflags "$(LDFLAGS)" -o jr .

install: generate
	go install -ldflags "$(LDFLAGS)" .

test:
	go test ./...

lint:
	golangci-lint run

spec-update:
	curl -sL "$(SPEC_URL)" -o spec/jira-v3.json

clean:
	rm -f jr
	rm -f cmd/generated/*.go

docs-generate:
	go run ./cmd/gendocs/... website

docs-dev: docs-generate
	cd website && npx vitepress dev

docs-build: docs-generate
	cd website && npx vitepress build

docs: docs-build
