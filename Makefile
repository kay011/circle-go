# 与 CI / Docker 一致，避免默认代理 502
export GOPROXY := https://goproxy.io,https://proxy.golang.org,direct

.PHONY: build test tidy run

build:
	go build -o bin/circle-go ./cmd/api

test:
	go test ./... -count=1

tidy:
	go mod tidy

run:
	go run ./cmd/api/main.go
