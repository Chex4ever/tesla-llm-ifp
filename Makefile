.PHONY: build-agent build-agent-linux nest-up test

build-agent:
	go build -o bin/pirate.exe ./agent/cmd/pirate

build-agent-linux:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/pirate-linux-amd64 ./agent/cmd/pirate

# Requires deploy/crowsnest/pirate (Linux) and NEST_HOST in .env — see deploy/crowsnest/README.md
nest-up:
	cd deploy/crowsnest && docker compose up -d --build

test:
	go test ./agent/...
