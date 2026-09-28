package fleetcrypto

import (
	"fmt"
	"time"
)

func NestAdvertMac(secret, nestID, url, kind string, ts time.Time) string {
	canon := fmt.Sprintf("%s|%s|%s|%d", nestID, url, kind, ts.Unix())
	return HMACHex(secret, canon)
}
