# 与 CI / Docker 一致，避免默认代理 502
export GOPROXY := https://goproxy.cn,direct
export GOSUMDB := sum.golang.org

.PHONY: build test tidy run dev kill-port health

build:
	go build -o bin/circle-go ./cmd/api

test:
	go test ./... -count=1

tidy:
	go mod tidy

run:
	go run ./cmd/api/main.go

kill-port:
	@pid=$$(lsof -t -iTCP:8080 -sTCP:LISTEN 2>/dev/null || true); \
	if [ -n "$$pid" ]; then \
		echo "Killing process on 8080: $$pid"; \
		kill $$pid; \
		sleep 1; \
	fi

health:
	@curl -fsS http://127.0.0.1:8080/health >/dev/null && echo "Health check passed: http://127.0.0.1:8080/health"

dev: kill-port
	@echo "Starting API at http://127.0.0.1:8080 ..."
	@echo "Tip: open another terminal and run 'make health'"
	go run ./cmd/api
