.PHONY: bootstrap
bootstrap:
	script/install-glide
	glide install

.PHONY: update
update:
	glide up
	glide install

.PHONY: compile-test
compile-test:
	go test -cover -race $(shell glide nv)
