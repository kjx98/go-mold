#
#	Makefile for hookAPI
#
# switches:
#	define the ones you want in the CFLAGS definition...
#
#	TRACE		- turn on tracing/debugging code
#
#
#
#

# Version for distribution
VER=1_0r1
GOPATH=$(shell go env GOPATH):$(PWD)/build

export GOPATH
MAKEFILE=GNUmakefile

# We Use Compact Memory Model

all: link bin/client bin/server
	@[ -d bin ] || exit

bin/client:	cmd/client/main.go
	@[ -d bin ] || mkdir bin
	@go build -o $@ $^
	@strip $@ || echo "client OK"

bin/server:	cmd/server/main.go
	@[ -d bin ] || mkdir bin
	@go build -o $@ $^
	@strip $@ || echo "server OK"

link:
	@[ -d build/src ] || (mkdir -p build/src/github.com/kjx98 \
		&& ln -s $(PWD) build/src/github.com/kjx98/go-mold)

test:
	@go test
	@go test -tags norecover

bench:
	sudo cpupower frequency-set --governor performance
	@(GOGC=400 go test -bench=.)
	sudo cpupower frequency-set --governor powersave

clean:
	@rm -f bin/*

distclean: clean
	@rm -rf bin build
