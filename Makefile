# Lambda binaries: Linux arm64, static, named "bootstrap" as required by the provided.al2023 runtime.
GOFLAGS_LAMBDA := -tags lambda.norpc -trimpath -buildvcs=false -ldflags="-s -w"
SERVICES := hello catalog

# Pinned so local runs and CI lint with the exact same version. Run via "go run":
# nothing to install, and the Go build cache makes later runs fast.
GOLANGCI := github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2

# Terraform stack holding the Cognito outputs, and the name of its user pool.
TF_DIR := infra/envs/dev
USER_POOL_NAME := monster-arena-dev-auth

.PHONY: test lint build token $(SERVICES)

test:
	go vet ./...
	go test ./...

lint:
	go run $(GOLANGCI) run

build: $(SERVICES)

$(SERVICES):
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build $(GOFLAGS_LAMBDA) -o dist/$@/bootstrap ./services/$@/cmd/lambda

# token prints a fresh access token for the admin machine client, and nothing else, so it
# can be captured: TOKEN=$$(make -s token)
#
# The client secret is read from Cognito on every call and never written to disk: there is
# no file to gitignore and no credential to rotate out of the repository. Requires a live
# AWS session (aws login) and a terraform init in $(TF_DIR).
token:
	@set -eu; \
	client_id=$$(terraform -chdir=$(TF_DIR) output -raw admin_client_id); \
	token_url=$$(terraform -chdir=$(TF_DIR) output -raw auth_token_endpoint); \
	pool_id=$$(aws cognito-idp list-user-pools --max-results 60 \
		--query "UserPools[?Name=='$(USER_POOL_NAME)'].Id" --output text); \
	test -n "$$pool_id" || { echo "user pool $(USER_POOL_NAME) not found" >&2; exit 1; }; \
	secret=$$(aws cognito-idp describe-user-pool-client \
		--user-pool-id "$$pool_id" --client-id "$$client_id" \
		--query 'UserPoolClient.ClientSecret' --output text); \
	response=$$(curl -sS --fail-with-body -X POST "$$token_url" \
		-u "$$client_id:$$secret" \
		-H 'Content-Type: application/x-www-form-urlencoded' \
		-d 'grant_type=client_credentials&scope=catalog/write'); \
	printf '%s' "$$response" \
	| python3 -c 'import sys, json; print(json.load(sys.stdin)["access_token"])'
