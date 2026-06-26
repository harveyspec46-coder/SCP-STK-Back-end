package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type contextKey string

const UserKey contextKey = "user"

type Claims struct {
	jwt.RegisteredClaims
	Email string `json:"email"`
}

type UserInfo struct {
	ID        string
	Email     string
	Role      string
	Office    string
	DisplayID string
}

// jwksCache caches the public keys from Supabase JWKS endpoint
var (
	jwksCache    map[string]*ecdsa.PublicKey
	jwksCacheMu  sync.RWMutex
	jwksCachedAt time.Time
)

func getECDSAPublicKey(supabaseURL string, kid string) (*ecdsa.PublicKey, error) {
	// Check cache first (cache for 1 hour)
	jwksCacheMu.RLock()
	if jwksCache != nil && time.Since(jwksCachedAt) < time.Hour {
		if key, ok := jwksCache[kid]; ok {
			jwksCacheMu.RUnlock()
			return key, nil
		}
	}
	jwksCacheMu.RUnlock()

	// Fetch fresh JWKS
	resp, err := http.Get(supabaseURL + "/auth/v1/.well-known/jwks.json")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
	}
	defer resp.Body.Close()

	var jwks struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			Crv string `json:"crv"`
			X   string `json:"x"`
			Y   string `json:"y"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, fmt.Errorf("failed to decode JWKS: %w", err)
	}

	// Build cache
	newCache := make(map[string]*ecdsa.PublicKey)
	for _, k := range jwks.Keys {
		if k.Kty != "EC" {
			continue
		}
		xBytes, err := base64.RawURLEncoding.DecodeString(k.X)
		if err != nil {
			continue
		}
		yBytes, err := base64.RawURLEncoding.DecodeString(k.Y)
		if err != nil {
			continue
		}
		pubKey := &ecdsa.PublicKey{
			Curve: elliptic.P256(),
			X:     new(big.Int).SetBytes(xBytes),
			Y:     new(big.Int).SetBytes(yBytes),
		}
		newCache[k.Kid] = pubKey
	}

	jwksCacheMu.Lock()
	jwksCache = newCache
	jwksCachedAt = time.Now()
	jwksCacheMu.Unlock()

	if key, ok := newCache[kid]; ok {
		return key, nil
	}
	return nil, fmt.Errorf("key with kid %q not found in JWKS", kid)
}

func Middleware(jwtSecret string, db *pgxpool.Pool, supabaseURL string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
				http.Error(w, `{"error":"invalid authorization header format"}`, http.StatusUnauthorized)
				return
			}

			tokenStr := parts[1]
			claims := &Claims{}

			token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
				switch t.Method.(type) {
				case *jwt.SigningMethodHMAC:
					// Legacy HS256
					return []byte(jwtSecret), nil
				case *jwt.SigningMethodECDSA:
					// New ECC P-256 — fetch public key from JWKS
					kid, _ := t.Header["kid"].(string)
					return getECDSAPublicKey(supabaseURL, kid)
				default:
					return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
				}
			})

			if err != nil || !token.Valid {
				http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			user := &UserInfo{
				ID:    claims.Subject,
				Email: claims.Email,
				Role:  "staff",
			}

			var displayID *string
			row := db.QueryRow(r.Context(),
				`SELECT role, office, display_id FROM users WHERE id = $1`, claims.Subject)
			if scanErr := row.Scan(&user.Role, &user.Office, &displayID); scanErr == nil && displayID != nil {
				user.DisplayID = *displayID
			}

			ctx := context.WithValue(r.Context(), UserKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetUser(ctx context.Context) *UserInfo {
	u, _ := ctx.Value(UserKey).(*UserInfo)
	return u
}

func RequireRole(minRole string) func(http.Handler) http.Handler {
	rank := map[string]int{"staff": 1, "manager": 2, "admin": 3}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := GetUser(r.Context())
			if user == nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			if rank[user.Role] < rank[minRole] {
				http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func RequireExactRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := GetUser(r.Context())
			if user == nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			if user.Role != role {
				http.Error(w, `{"error":"this area is restricted to admins"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
