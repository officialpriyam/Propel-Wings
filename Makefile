GIT_HEAD = $(shell git rev-parse HEAD | head -c8)

build:
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -gcflags "all=-trimpath=$(pwd)" -o build/wings_linux_amd64 -v wings.go
	GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -gcflags "all=-trimpath=$(pwd)" -o build/wings_linux_arm64 -v wings.go

windows:
	GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o build/wings_windows_amd64.exe -v wings.go

test:
	go test -race ./...

swagger:
	@command -v swag >/dev/null 2>&1 || { echo >&2 "swag command not found. Please install swag (https://github.com/swaggo/swag) to generate Swagger docs."; exit 1; }
	go generate ./router

debug-swagger:
	go generate ./router
	go build -ldflags="-X github.com/priyxstudio/propel/system.Version=$(GIT_HEAD)"
	sudo ./propel --debug --ignore-certificate-errors --config config.yml --pprof --pprof-block-rate 1

debug:
	go build -ldflags="-X github.com/priyxstudio/propel/system.Version=$(GIT_HEAD)"
	sudo ./propel --debug --ignore-certificate-errors --config config.yml --pprof --pprof-block-rate 1

# Runs a remotly debuggable session for Wings allowing an IDE to connect and target
# different breakpoints.
rmdebug:
	go build -gcflags "all=-N -l" -ldflags="-X github.com/priyxstudio/propel/system.Version=$(GIT_HEAD)" -race
	sudo dlv --listen=:2345 --headless=true --api-version=2 --accept-multiclient exec ./propel -- --debug --ignore-certificate-errors --config config.yml

cross-build: clean build compress

clean:
	rm -rf build/wings_*

.PHONY: all build compress clean
