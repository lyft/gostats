# the name of this package
PKG := $(shell go list .)

.PHONY: install
install: glide #download dependencies (including test deps) for the package
	glide install

.PHONY: update
update: glide #updates dependencies used by the package and installs them
	glide update

.PHONY: glide
glide: # ensure the glide package tool is installed
	which glide || (curl https://glide.sh/get | sh)

.PHONY: lint
lint: #lints the package for common code smells
	which golint || go get -u github.com/golang/lint/golint
	test -z "$(gofmt -d -s ./*.go)" || (gofmt -d -s ./*.go && exit 1)
	golint -set_exit_status
	go tool vet -all -shadow -shadowstrict *.go

.PHONY: test
test: # runs all tests against the package with race detection and coverage percentage
	go test -race -cover

.PHONY: quick
quick: # runs all tests without coverage or the race detector
	go test

.PHONY: cover
cover: # runs all tests against the package, generating a coverage report and opening it in the default browser
	go test -race -covermode=atomic -coverprofile=cover.out
	go tool cover -html cover.out -o cover.html
	open cover.html
