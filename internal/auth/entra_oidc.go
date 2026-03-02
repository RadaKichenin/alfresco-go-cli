package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
)

type EntraOIDCValidator struct {
	verifier *oidc.IDTokenVerifier
	issuer   string
	audience string
}

// NewEntraOIDCValidator creates a validator that discovers Microsoft Entra signing keys via OIDC/JWKS.
// tenantID can be a concrete tenant GUID/domain, or "common" for multitenant testing.
// audience should be your API App ID URI or client ID expected in aud.
func NewEntraOIDCValidator(ctx context.Context, tenantID, audience, issuerOverride string) (*EntraOIDCValidator, error) {
	tenantID = strings.TrimSpace(tenantID)
	audience = strings.TrimSpace(audience)
	if tenantID == "" {
		return nil, errors.New("entra tenant id is required")
	}
	if audience == "" {
		return nil, errors.New("entra audience (client/app id) is required")
	}

	issuer := strings.TrimSpace(issuerOverride)
	if issuer == "" {
		issuer = fmt.Sprintf("https://login.microsoftonline.com/%s/v2.0", tenantID)
	}

	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("failed to discover OIDC provider: %w", err)
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: audience})
	return &EntraOIDCValidator{
		verifier: verifier,
		issuer:   issuer,
		audience: audience,
	}, nil
}

func (v *EntraOIDCValidator) ValidateToken(ctx context.Context, token string) (Claims, error) {
	idToken, err := v.verifier.Verify(ctx, token)
	if err != nil {
		return Claims{}, fmt.Errorf("token verification failed: %w", err)
	}

	var payload map[string]any
	if err := idToken.Claims(&payload); err != nil {
		return Claims{}, fmt.Errorf("failed to parse token claims: %w", err)
	}

	sub := claimString(payload, "oid")
	if sub == "" {
		sub = claimString(payload, "sub")
	}
	roles := collectRoles(payload)
	return Claims{Subject: sub, Roles: roles}, nil
}

func claimString(claims map[string]any, key string) string {
	if v, ok := claims[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func collectRoles(claims map[string]any) []string {
	roles := make([]string, 0)
	switch raw := claims["roles"].(type) {
	case []any:
		for _, r := range raw {
			if s, ok := r.(string); ok {
				roles = append(roles, strings.TrimSpace(s))
			}
		}
	case []string:
		for _, s := range raw {
			roles = append(roles, strings.TrimSpace(s))
		}
	case string:
		roles = append(roles, strings.TrimSpace(raw))
	}
	if len(roles) == 0 {
		if scp, ok := claims["scp"].(string); ok {
			for _, s := range strings.Fields(scp) {
				roles = append(roles, strings.TrimSpace(s))
			}
		}
	}

	clean := make([]string, 0, len(roles))
	seen := map[string]bool{}
	for _, r := range roles {
		if r == "" {
			continue
		}
		key := strings.ToLower(r)
		if seen[key] {
			continue
		}
		seen[key] = true
		clean = append(clean, r)
	}
	return clean
}
