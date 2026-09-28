FROM golang:1.24-alpine AS build
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/fleet-api ./apps/fleet-api

FROM alpine:3.21
RUN apk add --no-cache ca-certificates curl
COPY --from=build /out/fleet-api /usr/local/bin/fleet-api
EXPOSE 8081
ENTRYPOINT ["fleet-api"]
