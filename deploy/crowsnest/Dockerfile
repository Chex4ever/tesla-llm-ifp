# Expects a pre-built Linux `pirate` binary next to this Dockerfile.
# (Used by TUI / vpsdeploy; avoids compiling Go on the VPS.)
FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY pirate /usr/local/bin/pirate
EXPOSE 7843
ENTRYPOINT ["pirate"]
