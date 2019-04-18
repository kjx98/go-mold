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

win64: bin/client64.exe
	@[ -d bin ] || exit

bin/client:	cmd/client/main.go
	@[ -d bin ] || mkdir bin
	@go build -o $@ $^
	@strip $@ || echo "client OK"

bin/server:	cmd/server/main.go
	@[ -d bin ] || mkdir bin
	@go build -o $@ $^
	@strip $@ || echo "server OK"

bin/client64.exe:	cmd/client/main.go
	@[ -d bin ] || mkdir bin
	(. ./mingw64-env.sh; go build -o $@ $^)
	@echo "client64.exe OK"

test:
	@go test -v
	@go test -tags nativeEndian -v

bench:
	sudo cpupower frequency-set --governor performance
	@(GOGC=400 go test -bench=.)
	sudo cpupower frequency-set --governor powersave

clean:
	@rm -f bin/*

distclean: clean
	@rm -rf bin
