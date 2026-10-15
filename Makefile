.PHONY: build vet fmt test check

build:
	go build -o bin/instagram-publisher ./cmd/instagram-publisher

vet:
	go vet ./...

fmt:
	gofmt -l -w .

test:
	go test ./...

check: vet test
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed:"; gofmt -l .; exit 1)
