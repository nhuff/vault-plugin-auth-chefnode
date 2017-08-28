.PHONY: bootstrap vendor clean test

SRCS=main.go $(wildcard plugin/*.go)

default: dev

bootstrap:
	go get -u github.com/golang/dep/cmd/dep

vendor:
	dep ensure

bin/vault-plugin-auth-chefnode: $(SRCS)
	mkdir -p bin
	CGO_ENABLED=0 go build -o bin/vault-plugin-auth-chefnode main.go

test:
	go test github.com/nhuff/vault-plugin-auth-chefnode/plugin

clean:
	rm -rf bin

dev: test bin/vault-plugin-auth-chefnode

release: clean
