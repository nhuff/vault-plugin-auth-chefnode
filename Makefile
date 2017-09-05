.PHONY: bootstrap vendor clean test release dev

SRCS := main.go $(wildcard plugin/*.go)
OS_ARCHS := linux/amd64 linux/386 freebsd/386 freebsd/amd64
BIN_NAME := vault-plugin-auth-chefnode
RELEASE_TARGETS := $(foreach OS_ARCH,$(OS_ARCHS),pkg/$(OS_ARCH)/$(BIN_NAME))

default: dev

bootstrap:
	go get -u github.com/golang/dep/cmd/dep
	go get -u github.com/mitchellh/gox

vendor:
	dep ensure

bin/vault-plugin-auth-chefnode: $(SRCS)
	mkdir -p bin
	CGO_ENABLED=0 go build -o bin/vault-plugin-auth-chefnode main.go

test:
	go test -v github.com/nhuff/vault-plugin-auth-chefnode/plugin

clean:
	rm -rf bin
	rm -rf pkg

dev: test bin/vault-plugin-auth-chefnode

$(RELEASE_TARGETS) : test $(SRCS)
	mkdir -p pkg
	CGO_ENABLED=0 gox -osarch=$(subst /$(BIN_NAME),,$(subst pkg/,,$@)) \
		-output="pkg/{{.OS}}/{{.Arch}}/vault-plugin-auth-chefnode"

release: test $(RELEASE_TARGETS)
