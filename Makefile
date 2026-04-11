VERSION  ?= dev
LDFLAGS  := -ldflags="-s -w -X main.version=$(VERSION)"
DIST     := dist

.PHONY: build build-all clean

build:
	go build $(LDFLAGS) -o $(DIST)/codex2api .

build-all: clean
	mkdir -p $(DIST)
	GOOS=windows GOARCH=amd64  go build $(LDFLAGS) -o $(DIST)/codex2api-windows-amd64.exe .
	GOOS=windows GOARCH=arm64  go build $(LDFLAGS) -o $(DIST)/codex2api-windows-arm64.exe .
	GOOS=linux   GOARCH=amd64  go build $(LDFLAGS) -o $(DIST)/codex2api-linux-amd64       .
	GOOS=linux   GOARCH=arm64  go build $(LDFLAGS) -o $(DIST)/codex2api-linux-arm64       .
	GOOS=darwin  GOARCH=amd64  go build $(LDFLAGS) -o $(DIST)/codex2api-darwin-amd64      .
	GOOS=darwin  GOARCH=arm64  go build $(LDFLAGS) -o $(DIST)/codex2api-darwin-arm64      .

clean:
	rm -rf $(DIST)
