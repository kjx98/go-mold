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

MAKEFILE=GNUmakefile

# We Use Compact Memory Model

all: bin/client bin/server
	@[ -d bin ] || exit

bin/client:	cmd/client/main.go
	@[ -d bin ] || mkdir bin
	@go build -o $@ $^
	@strip $@ || echo "client OK"

bin/clientRaw:	cmd/client/main.go
	@[ -d bin ] || mkdir bin
	@go build -tags rawSocket -o $@ $^
	@strip $@ || echo "clientRaw OK"

bin/server:	cmd/server/main.go
	@[ -d bin ] || mkdir bin
	@go build -o $@ $^
	@strip $@ || echo "server OK"

raw: bin/clientRaw

test:
	@go test
	@go test -tags rawSocket

bench:
	sudo cpupower frequency-set --governor performance
	@(GOGC=400 go test -bench=.)
	sudo cpupower frequency-set --governor powersave

clean:
	@rm -f bin/*

distclean: clean
	@rm -rf bin
