package auth

import (
	"errors"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func TestClassifyTokenError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"expired", jwt.ErrTokenExpired, AuthFailReasonExpired},
		{"not-yet-valid", jwt.ErrTokenNotValidYet, AuthFailReasonExpired},
		{"bad-signature", jwt.ErrTokenSignatureInvalid, AuthFailReasonInvalid},
		{"malformed", jwt.ErrTokenMalformed, AuthFailReasonInvalid},
		{"bad-issuer", jwt.ErrTokenInvalidIssuer, AuthFailReasonInvalid},
		{"bad-audience", jwt.ErrTokenInvalidAudience, AuthFailReasonInvalid},
		{"unknown", errors.New("something else"), AuthFailReasonInvalid},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := classifyTokenError(c.err)
			if got != c.want {
				t.Errorf("classifyTokenError(%v) = %q, want %q", c.err, got, c.want)
			}
		})
	}
}

// We don't have a full keyfunc/JWKS mock here so we can only unit-test the
// pure helpers. Integration-level testing of JWT leeway happens via the
// curl-based e2e in scripts/e2e-token-refresh.sh.
