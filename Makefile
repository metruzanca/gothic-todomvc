.PHONY: generate run test build

generate:
	go generate ./src

run: generate
	go run ./cmd/todomvc

build: generate
	go build -o bin/todomvc ./cmd/todomvc
