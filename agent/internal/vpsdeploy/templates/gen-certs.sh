#!/usr/bin/env bash
# Generate a self-signed cert with IP or DNS SAN for Crow's Nest (no ACME).
set -euo pipefail
HOST="${1:?usage: gen-certs.sh <ip-or-hostname> [outdir]}"
OUT="${2:-./certs}"
mkdir -p "$OUT"

if [[ "$HOST" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  SAN="IP:${HOST}"
else
  SAN="DNS:${HOST}"
fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

cat >"$TMP/openssl.cnf" <<EOF
[req]
default_bits = 2048
prompt = no
default_md = sha256
distinguished_name = dn
x509_extensions = v3_req

[dn]
CN = ${HOST}
O = Pirate Fleet Nest

[v3_req]
subjectAltName = ${SAN}
basicConstraints = CA:FALSE
keyUsage = digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
EOF

openssl req -x509 -nodes -newkey rsa:2048 \
  -keyout "$OUT/key.pem" \
  -out "$OUT/cert.pem" \
  -days 825 \
  -config "$TMP/openssl.cnf"
chmod 644 "$OUT/cert.pem"
chmod 600 "$OUT/key.pem"
echo "self-signed cert for ${SAN} -> ${OUT}"
