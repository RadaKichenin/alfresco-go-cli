package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

type Claims struct {
	Subject string
	Roles   []string
}

type Validator interface {
	ValidateToken(ctx context.Context, token string) (Claims, error)
}

type contextKey struct{}

func ClaimsFromContext(ctx context.Context) (Claims, bool) {
	c, ok := ctx.Value(contextKey{}).(Claims)
	return c, ok
}

func Middleware(v Validator, required bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !required {
				next.ServeHTTP(w, r)
				return
			}

			token, err := bearerToken(r.Header.Get("Authorization"))
			if err != nil {
				http.Error(w, "missing or invalid bearer token", http.StatusUnauthorized)
				return
			}
			claims, err := v.ValidateToken(r.Context(), token)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			r = r.WithContext(context.WithValue(r.Context(), contextKey{}, claims))
			next.ServeHTTP(w, r)
		})
	}
}

func RequireRoles(next http.Handler, roles ...string) http.Handler {
	needed := map[string]bool{}
	for _, role := range roles {
		needed[strings.ToLower(strings.TrimSpace(role))] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := ClaimsFromContext(r.Context())
		if !ok {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		for _, role := range claims.Roles {
			if needed[strings.ToLower(strings.TrimSpace(role))] {
				next.ServeHTTP(w, r)
				return
			}
		}
		http.Error(w, "forbidden", http.StatusForbidden)
	})
}

func bearerToken(authHeader string) (string, error) {
	authHeader = strings.TrimSpace(authHeader)
	if authHeader == "" {
		return "", errors.New("missing authorization")
	}
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
		return "", errors.New("invalid authorization")
	}
	return strings.TrimSpace(parts[1]), nil
}
