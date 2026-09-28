.PHONY: build-agent build-agent-linux nest-up test

build-agent:
	go build -o bin/tesla-agent.exe ./agent/cmd/tesla-agent

build-agent-linux:
	GOOS=linux GOARCH=amd64 go build -o bin/tesla-agent ./agent/cmd/tesla-agent

nest-up:
	cd deploy/crowsnest && docker compose up -d --build

test:
	go test ./agent/...
