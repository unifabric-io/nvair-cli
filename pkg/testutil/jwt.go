package testutil

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"
)

// MakeTestJWT creates an unsigned JWT-like token with the provided expiration time.
func MakeTestJWT(exp time.Time) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d}`, exp.Unix())))
	return strings.Join([]string{header, payload, "signature"}, ".")
}
