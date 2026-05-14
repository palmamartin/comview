PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin

BIN := comview
CMD := ./cmd/comview

.PHONY: all build install uninstall test clean

all: build

build:
	go build -o $(BIN) $(CMD)

install: build
	install -d $(DESTDIR)$(BINDIR)
	install -m 0755 $(BIN) $(DESTDIR)$(BINDIR)/$(BIN)

uninstall:
	rm -f $(DESTDIR)$(BINDIR)/$(BIN)

test:
	go test ./...

clean:
	rm -f $(BIN)
