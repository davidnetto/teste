// Package auth implementa autenticação JWT + bcrypt para customers/admins.
package auth

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// ctxKey é a chave usada para guardar o usuário autenticado no contexto.
type ctxKey string

const ctxUserKey ctxKey = "auth.user"

// Claims é o payload do JWT.
type Claims struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Role  string `json:"role"`
	Name  string `json:"name"`
	jwt.RegisteredClaims
}

// Service encapsula configuração de auth (segredo, expiração).
type Service struct {
	DB        *sql.DB
	Secret    []byte
	TTL       time.Duration
	Issuer    string
}

// New retorna um Service com defaults razoáveis.
func New(db *sql.DB, secret string) *Service {
	return &Service{
		DB:     db,
		Secret: []byte(secret),
		TTL:    24 * time.Hour,
		Issuer: "florentina-api",
	}
}

// HashPassword gera hash bcrypt de uma senha.
func HashPassword(plain string) (string, error) {
	if len(plain) < 6 {
		return "", errors.New("senha precisa ter no mínimo 6 caracteres")
	}
	h, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	return string(h), err
}

// CheckPassword compara senha em texto com hash bcrypt.
func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// Sign gera um JWT para o usuário informado.
func (s *Service) Sign(id, email, role, name string) (string, time.Time, error) {
	now := time.Now()
	exp := now.Add(s.TTL)
	claims := Claims{
		Sub:   id,
		Email: email,
		Role:  role,
		Name:  name,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.Issuer,
			Subject:   id,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	str, err := tok.SignedString(s.Secret)
	return str, exp, err
}

// Parse valida e retorna os claims de um token.
func (s *Service) Parse(tokenStr string) (*Claims, error) {
	tok, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("método de assinatura inválido")
		}
		return s.Secret, nil
	})
	if err != nil {
		return nil, err
	}
	c, ok := tok.Claims.(*Claims)
	if !ok || !tok.Valid {
		return nil, errors.New("token inválido")
	}
	return c, nil
}

// FromContext retorna o usuário autenticado, se houver.
func FromContext(ctx context.Context) (*Claims, bool) {
	c, ok := ctx.Value(ctxUserKey).(*Claims)
	return c, ok
}

// IntoContext injeta claims em um contexto (usado por handlers de WebSocket).
func IntoContext(ctx context.Context, c *Claims) context.Context {
	return context.WithValue(ctx, ctxUserKey, c)
}

// ClaimsFromRequest extrai e valida o JWT do header Authorization.
// Retorna nil, false se o token estiver ausente ou inválido.
func (s *Service) ClaimsFromRequest(r *http.Request) (*Claims, bool) {
	raw := tokenFromHeader(r)
	if raw == "" {
		return nil, false
	}
	c, err := s.Parse(raw)
	if err != nil {
		return nil, false
	}
	return c, true
}

// Middleware exige um JWT válido no header Authorization.
func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := tokenFromHeader(r)
		if raw == "" {
			httpError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		c, err := s.Parse(raw)
		if err != nil {
			httpError(w, http.StatusUnauthorized, "invalid token: "+err.Error())
			return
		}
		ctx := context.WithValue(r.Context(), ctxUserKey, c)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// AdminOnly exige role=admin além de JWT válido.
func (s *Service) AdminOnly(next http.Handler) http.Handler {
	return s.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, _ := FromContext(r.Context())
		if c == nil || c.Role != "admin" {
			httpError(w, http.StatusForbidden, "admin role required")
			return
		}
		next.ServeHTTP(w, r)
	}))
}

// Optional injeta o usuário no contexto se houver token, mas não exige.
func (s *Service) Optional(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := tokenFromHeader(r)
		if raw != "" {
			if c, err := s.Parse(raw); err == nil {
				ctx := context.WithValue(r.Context(), ctxUserKey, c)
				r = r.WithContext(ctx)
			}
		}
		next.ServeHTTP(w, r)
	})
}

func tokenFromHeader(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	parts := strings.SplitN(h, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func httpError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write([]byte(`{"error":"` + escape(msg) + `"}`))
}

func escape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`)
}
