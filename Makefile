.PHONY: generate build install test spec-update clean

SPEC_URL := https://dac-static.atlassian.com/cloud/jira/platform/swagger-v3.v3.json?_v=1.8420.0

generate:
	go run ./gen/...

build: generate
	go build -o jr .

install: generate
	go install .

test:
	go test ./...

spec-update:
	curl -sL "$(SPEC_URL)" -o spec/jira-v3.json

clean:
	rm -f jr
	rm -f cmd/generated/*.go
