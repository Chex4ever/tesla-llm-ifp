package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

func RandomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func APIKey() (raw, prefix, hash string, err error) {
	raw, err = RandomToken(32)
	if err != nil {
		return "", "", "", err
	}
	raw = "sk-pirate-" + raw
	if len(raw) < 16 {
		return "", "", "", fmt.Errorf("api key too short")
	}
	prefix = raw[:16]
	hash = HashToken(raw)
	return raw, prefix, hash, nil
}
