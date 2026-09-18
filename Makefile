# Lambda binaries: Linux arm64, static, named "bootstrap" as required by the provided.al2023 runtime.
GOFLAGS_LAMBDA := -tags lambda.norpc -trimpath -buildvcs=false -ldflags="-s -w"
SERVICES := hello

.PHONY: test build $(SERVICES)

test:
	go vet ./...
	go test ./...

build: $(SERVICES)

$(SERVICES):
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build $(GOFLAGS_LAMBDA) -o dist/$@/bootstrap ./services/$@/cmd/lambda
