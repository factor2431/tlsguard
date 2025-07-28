all: build

build:
	CGO_ENABLED='0' GOROOT_FINAL='/usr' GOOS='linux' GOARCH='amd64' go build -trimpath -asmflags '-s -w' -ldflags '-s -w' -o tlsguard

clean:
	rm -f tlsguard

release: build
	upx --lzma tlsguard
