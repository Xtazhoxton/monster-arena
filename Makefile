# Lambda binaries: Linux arm64, static, named "bootstrap" as required by the provided.al2023 runtime.
GOFLAGS_LAMBDA := -tags lambda.norpc -trimpath -buildvcs=false -ldflags="-s -w"
SERVICES := hello catalog

# Pinned so local runs and CI lint with the exact same version. Run via "go run":
# nothing to install, and the Go build cache makes later runs fast.
GOLANGCI := github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2

.PHONY: test lint build $(SERVICES)

test:
	go vet ./...
	go test ./...

lint:
	go run $(GOLANGCI) run

build: $(SERVICES)

$(SERVICES):
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build $(GOFLAGS_LAMBDA) -o dist/$@/bootstrap ./services/$@/cmd/lambda
