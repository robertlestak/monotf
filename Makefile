VERSION=v0.0.1

.PHONY: bin
bin: bin/monotf_darwin_x86_64 bin/monotf_darwin_arm64 bin/monotf_linux_x86_64 bin/monotf_linux_arm bin/monotf_linux_arm64 bin/monotf_windows_x86_64.exe

bin/monotf_darwin_x86_64:
	mkdir -p bin
	GOOS=darwin GOARCH=amd64 go build -ldflags="-X 'main.Version=$(VERSION)'" -o bin/monotf_darwin_x86_64 cmd/monotf/*.go
	openssl sha512 bin/monotf_darwin_x86_64 > bin/monotf_darwin_x86_64.sha512

bin/monotf_darwin_arm64:
	mkdir -p bin
	GOOS=darwin GOARCH=arm64 go build -ldflags="-X 'main.Version=$(VERSION)'" -o bin/monotf_darwin_arm64 cmd/monotf/*.go
	openssl sha512 bin/monotf_darwin_arm64 > bin/monotf_darwin_arm64.sha512

bin/monotf_linux_x86_64:
	mkdir -p bin
	GOOS=linux GOARCH=amd64 go build -ldflags="-X 'main.Version=$(VERSION)'" -o bin/monotf_linux_x86_64 cmd/monotf/*.go
	openssl sha512 bin/monotf_linux_x86_64 > bin/monotf_linux_x86_64.sha512

bin/monotf_linux_arm:
	mkdir -p bin
	GOOS=linux GOARCH=arm go build -ldflags="-X 'main.Version=$(VERSION)'" -o bin/monotf_linux_arm cmd/monotf/*.go
	openssl sha512 bin/monotf_linux_arm > bin/monotf_linux_arm.sha512

bin/monotf_linux_arm64:
	mkdir -p bin
	GOOS=linux GOARCH=arm64 go build -ldflags="-X 'main.Version=$(VERSION)'" -o bin/monotf_linux_arm64 cmd/monotf/*.go
	openssl sha512 bin/monotf_linux_arm64 > bin/monotf_linux_arm64.sha512

bin/monotf_windows_x86_64.exe:
	mkdir -p bin
	GOOS=windows GOARCH=amd64 go build -ldflags="-X 'main.Version=$(VERSION)'" -o bin/monotf_windows_x86_64.exe cmd/monotf/*.go
	openssl sha512 bin/monotf_windows_x86_64.exe > bin/monotf_windows_x86_64.exe.sha512