package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type contextKey string

const UserKey contextKey = "user"

// Claims mirrors Supabase JWT structure. We only use it to verify the
// signature and read the subject (user id) + email — role/office/display_id
// are looked up live from Postgres on every request (see Middleware below),
// not trusted from app_metadata, so role changes (like a new admin_allowlist
// signup) take effect immediately without waiting on a token refresh.
type Claims struct {
	jwt.RegisteredClaims
	Email string `json:"email"`
}

// UserInfo is stored in request context after JWT validation + DB lookup.
type UserInfo struct {
	ID        string
	Email     string
	Role      string // admin | manager | staff — read live from users.role
	Office    string // north | south
	DisplayID string // ADM-001 / MGR-003 / STF-007 — empty if not yet assigned
}

// Middleware validates the Supabase JWT signature, then looks up the
// current role/office/display_id from the users table. This is the layer
// that makes admin_allowlist recognition immediate: the moment the Postgres
// trigger sets role='admin' for an allowlisted signup, every subsequent
// request from that user is treated as admin — no re-login required.
func Middleware(jwtSecret string, db *pgxpool.Pool) func(http.Handler) http.Handler {
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
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, jwt.ErrSignatureInvalid
				}
				return []byte(jwtSecret), nil
			})

			if err != nil || !token.Valid {
				http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			user := &UserInfo{
				ID:    claims.Subject,
				Email: claims.Email,
				Role:  "staff", // fail safe to least privilege if the lookup below misses
			}

			// Live lookup — source of truth is the users table, never a cached claim.
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

// GetUser retrieves UserInfo from context. Panics if middleware was not applied.
func GetUser(ctx context.Context) *UserInfo {
	u, _ := ctx.Value(UserKey).(*UserInfo)
	return u
}

// RequireRole returns a middleware that enforces minimum role level.
// Role hierarchy: admin > manager > staff
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

// RequireExactRole restricts a route to one specific role only (e.g. the
// "only 3 admins can see Finance & the Job Funnel" rule — a manager, even
// though they outrank staff, must NOT pass this check).
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
