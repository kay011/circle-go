# 与 CI / Docker 一致，避免默认代理 502
export GOPROXY := https://goproxy.io,https://proxy.golang.org,direct

.PHONY: build test tidy run check

build:
	go build -o bin/circle-go ./cmd/api

test:
	go test ./... -count=1

# 提交前自检：单测 + 全包编译（与改代码后应满足的条件一致）
check: test
	go build ./...

tidy:
	go mod tidy

run:
	go run ./cmd/api/main.go
