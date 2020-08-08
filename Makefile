# Go source files
SRCS := $(shell find . -type d -name 'vendor' -prune -o -name '*.go' -print)

.PHONY: install
install:
	go mod download

.PHONY: golint
golint:
	scripts/lint
	golint -set_exit_status $(shell go list ./...)

.PHONY: govet
govet:
	scripts/vet

.PHONY: lint
lint: golint govet

.PHONY: test
test: # runs all tests against the package with race detection and coverage percentage
	go test -race -cover ./...

.PHONY: quick
quick: # runs all tests without coverage or the race detector
	go test ./...

.PHONY: cover
cover: # runs all tests against the package, generating a coverage report and opening it in the default browser
	go test -race -covermode=atomic -coverprofile=cover.out ./...
	go tool cover -html cover.out -o cover.html
	which open && open cover.html
