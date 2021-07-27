build:
	go build

test:
	go test

fmt:
	gofmt -w .
	goimports -w -local git.code.oa.com .

lint:
	golangci-lint run