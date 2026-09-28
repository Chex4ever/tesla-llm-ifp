FROM golang:1.24-alpine AS build
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/tesla-agent ./agent/cmd/tesla-agent

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=build /out/tesla-agent /usr/local/bin/tesla-agent
EXPOSE 7843
ENTRYPOINT ["tesla-agent"]
