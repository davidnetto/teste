package auth

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"
)

// Customer representa um cliente persistido.
type Customer struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Phone     string    `json:"phone,omitempty"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// RegisterRequest é o body do POST /api/auth/register.
type RegisterRequest struct {
	Name     string `json:"name"     example:"Maria"`
	Email    string `json:"email"    example:"maria@email.com"`
	Phone    string `json:"phone"    example:"+5511999999999"`
	Password string `json:"password" example:"abcdef"`
}

// LoginRequest é o body do POST /api/auth/login.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginResponse traz o token + dados do usuário.
type LoginResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	User      Customer  `json:"user"`
}

// Register godoc
// @Summary  Cadastra um novo cliente
// @Tags     auth
// @Accept   json
// @Produce  json
// @Param    body body     RegisterRequest true "Dados de cadastro"
// @Success  201  {object} LoginResponse
// @Failure  400  {object} map[string]string
// @Failure  409  {object} map[string]string
// @Router   /api/auth/register [post]
func (s *Service) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Name == "" || req.Email == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name, email and password required"})
		return
	}
	if _, err := mail.ParseAddress(req.Email); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid email"})
		return
	}

	hash, err := HashPassword(req.Password)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	var c Customer
	err = s.DB.QueryRow(
		`INSERT INTO customers (name, email, phone, password_hash, role)
		 VALUES ($1, $2, $3, $4, 'customer')
		 RETURNING id, name, email, COALESCE(phone, ''), role, created_at, updated_at`,
		req.Name, req.Email, req.Phone, hash,
	).Scan(&c.ID, &c.Name, &c.Email, &c.Phone, &c.Role, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "email already registered"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not register: " + err.Error()})
		return
	}

	tok, exp, err := s.Sign(c.ID, c.Email, c.Role, c.Name)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not sign token"})
		return
	}
	writeJSON(w, http.StatusCreated, LoginResponse{Token: tok, ExpiresAt: exp, User: c})
}

// Login godoc
// @Summary  Autentica um cliente e retorna JWT
// @Tags     auth
// @Accept   json
// @Produce  json
// @Param    body body     LoginRequest true "Credenciais"
// @Success  200  {object} LoginResponse
// @Failure  401  {object} map[string]string
// @Router   /api/auth/login [post]
func (s *Service) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email and password required"})
		return
	}

	var c Customer
	var hash string
	err := s.DB.QueryRow(
		`SELECT id, name, email, COALESCE(phone, ''), COALESCE(password_hash, ''), role, created_at, updated_at
		 FROM customers WHERE email = $1`,
		req.Email,
	).Scan(&c.ID, &c.Name, &c.Email, &c.Phone, &hash, &c.Role, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (hash == "" || !CheckPassword(hash, req.Password))) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not authenticate"})
		return
	}

	tok, exp, err := s.Sign(c.ID, c.Email, c.Role, c.Name)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not sign token"})
		return
	}
	writeJSON(w, http.StatusOK, LoginResponse{Token: tok, ExpiresAt: exp, User: c})
}

// Me godoc
// @Summary  Retorna dados do usuário autenticado
// @Tags     auth
// @Produce  json
// @Success  200 {object} Customer
// @Failure  401 {object} map[string]string
// @Security BearerAuth
// @Router   /api/auth/me [get]
func (s *Service) Me(w http.ResponseWriter, r *http.Request) {
	c, ok := FromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	var cust Customer
	err := s.DB.QueryRow(
		`SELECT id, name, email, COALESCE(phone, ''), role, created_at, updated_at
		 FROM customers WHERE id = $1`, c.Sub,
	).Scan(&cust.ID, &cust.Name, &cust.Email, &cust.Phone, &cust.Role, &cust.CreatedAt, &cust.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "user not found"})
		return
	}
	writeJSON(w, http.StatusOK, cust)
}

// EnsureAdmin cria/atualiza um admin a partir de envs (idempotente).
func EnsureAdmin(db *sql.DB, email, password, name string) error {
	if email == "" || password == "" {
		return nil
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	_, err = db.Exec(
		`INSERT INTO customers (name, email, password_hash, role)
		 VALUES ($1, $2, $3, 'admin')
		 ON CONFLICT (email) DO UPDATE SET password_hash = EXCLUDED.password_hash, role = 'admin', updated_at = now()`,
		name, strings.ToLower(email), hash,
	)
	return err
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
