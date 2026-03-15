.PHONY: generate build install test spec-update clean lint

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
