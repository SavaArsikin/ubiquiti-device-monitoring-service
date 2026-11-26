.PHONY: test-file test-verbose test test-setup db-setup db-setup-podman

test-file:
		@test -n "$(FILE)" || { echo "Usage: make test-file FILE=path/to/file_test.go" >&2; exit 1; }
		@test -f "$(FILE)" || { echo "Test file not found: $(FILE)" >&2; exit 1; }
		NODE_ENV=test go test -v -p 1 -parallel 1 -count=1 "./$(dir $(FILE))"

test-verbose:
	NODE_ENV=test go test -v -p 1 -parallel 1 -count=1 ./...

test: 
	NODE_ENV=test go test -p 1 -parallel 1 -count=1 ./...

# test-setup:
# 	NODE_ENV=test go run ./cmd/test-setup

db-setup:
	go run ./cmd/localsetup