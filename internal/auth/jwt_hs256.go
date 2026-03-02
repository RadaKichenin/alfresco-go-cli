package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type HS256Validator struct {
	secret   []byte
	issuer   string
	audience string
}

func NewHS256Validator(secret, issuer, audience string) (*HS256Validator, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, errors.New("jwt secret is required")
	}
	return &HS256Validator{secret: []byte(secret), issuer: issuer, audience: audience}, nil
}

func (v *HS256Validator) ValidateToken(_ context.Context, token string) (Claims, error) {
	parsed, err := jwt.Parse(token, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("unexpected signing method: %s", t.Method.Alg())
		}
		return v.secret, nil
	})
	if err != nil || !parsed.Valid {
		return Claims{}, errors.New("invalid token")
	}

	claimsMap, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return Claims{}, errors.New("invalid claims")
	}
	if v.issuer != "" && claimsMap["iss"] != v.issuer {
		return Claims{}, errors.New("issuer mismatch")
	}
	if v.audience != "" {
		audValid := false
		switch aud := claimsMap["aud"].(type) {
		case string:
			audValid = aud == v.audience
		case []any:
			for _, entry := range aud {
				if s, ok := entry.(string); ok && s == v.audience {
					audValid = true
					break
				}
			}
		}
		if !audValid {
			return Claims{}, errors.New("audience mismatch")
		}
	}

	sub, _ := claimsMap["sub"].(string)
	roles := make([]string, 0)
	switch raw := claimsMap["roles"].(type) {
	case []any:
		for _, role := range raw {
			if s, ok := role.(string); ok {
				roles = append(roles, s)
			}
		}
	case []string:
		roles = append(roles, raw...)
	case string:
		roles = append(roles, raw)
	}
	if len(roles) == 0 {
		if scp, ok := claimsMap["scp"].(string); ok {
			roles = append(roles, strings.Fields(scp)...)
		}
	}
	return Claims{Subject: sub, Roles: roles}, nil
}
