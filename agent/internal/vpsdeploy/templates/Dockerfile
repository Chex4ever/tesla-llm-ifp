FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY pirate /usr/local/bin/pirate
EXPOSE 7843
ENTRYPOINT ["pirate"]
