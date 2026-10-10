package common

import (
	"testing"
	"time"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/golang-jwt/jwt/v4"
)

func setupAuthTest(t *testing.T) {
	t.Helper()

	oldConf := conf.Conf
	oldSecretKey := SecretKey
	conf.Conf = &conf.Config{TokenExpiresIn: 1}
	SecretKey = []byte("test-secret")

	t.Cleanup(func() {
		conf.Conf = oldConf
		SecretKey = oldSecretKey
	})
}

func TestParseTokenDoesNotRequireLocalTokenState(t *testing.T) {
	setupAuthTest(t)

	tokenString, err := jwt.NewWithClaims(jwt.SigningMethodHS256, UserClaims{
		Username: "alice",
		PwdTS:    123,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}).SignedString(SecretKey)
	if err != nil {
		t.Fatal(err)
	}

	claims, err := ParseToken(tokenString)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Username != "alice" || claims.PwdTS != 123 {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestInvalidateTokenDoesNotBreakStatelessJWTValidation(t *testing.T) {
	setupAuthTest(t)

	tokenString, err := GenerateToken(&model.User{Username: "alice", PwdTS: 456})
	if err != nil {
		t.Fatal(err)
	}

	if err := InvalidateToken(tokenString); err != nil {
		t.Fatal(err)
	}
	if IsTokenInvalidated(tokenString) {
		t.Fatal("token should not be invalidated by local process state")
	}

	claims, err := ParseToken(tokenString)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Username != "alice" || claims.PwdTS != 456 {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}
