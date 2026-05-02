package auth

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// googleClientID retorna o Client ID configurado via GOOGLE_CLIENT_ID.
// Retorna string vazia se não definido — login Google fica desabilitado.
func googleClientID() string {
	return os.Getenv("GOOGLE_CLIENT_ID")
}

// googleHTTPClient é compartilhado por todos os requests ao Google.
var googleHTTPClient = &http.Client{Timeout: 8 * time.Second}

// GoogleClaims é o subset que usamos do payload do ID token Google.
type GoogleClaims struct {
	ISS           string `json:"iss"`
	AUD           string `json:"aud"`
	SUB           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified string `json:"email_verified"` // tokeninfo retorna como string ("true"/"false")
	Name          string `json:"name"`
	Picture       string `json:"picture"`
	Exp           string `json:"exp"`
}

// VerifyGoogleIDToken valida um ID token chamando o tokeninfo do Google.
// Retorna os claims se o token for válido para o nosso clientID.
func VerifyGoogleIDToken(idToken, expectedAudience string) (*GoogleClaims, error) {
	if idToken == "" {
		return nil, errors.New("missing credential")
	}
	if expectedAudience == "" {
		return nil, errors.New("GOOGLE_CLIENT_ID não configurado no servidor")
	}
	url := "https://oauth2.googleapis.com/tokeninfo?id_token=" + idToken
	resp, err := googleHTTPClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("google tokeninfo: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("token inválido: %s", string(body))
	}
	var c GoogleClaims
	if err := json.Unmarshal(body, &c); err != nil {
		return nil, fmt.Errorf("payload inválido: %w", err)
	}
	if c.AUD != expectedAudience {
		return nil, errors.New("audience do token não corresponde ao GOOGLE_CLIENT_ID")
	}
	if c.ISS != "accounts.google.com" && c.ISS != "https://accounts.google.com" {
		return nil, errors.New("issuer inválido")
	}
	if c.SUB == "" {
		return nil, errors.New("token sem sub")
	}
	if strings.ToLower(c.EmailVerified) != "true" {
		return nil, errors.New("e-mail não verificado pelo Google")
	}
	return &c, nil
}

// GoogleLoginRequest body do POST /api/auth/google.
type GoogleLoginRequest struct {
	Credential string `json:"credential"` // ID token do Google
}

// Google godoc
// @Summary  Login/cadastro com conta Google
// @Tags     auth
// @Accept   json
// @Produce  json
// @Param    body body     GoogleLoginRequest true "ID token do Google"
// @Success  200  {object} LoginResponse
// @Failure  401  {object} map[string]string
// @Router   /api/auth/google [post]
func (s *Service) Google(w http.ResponseWriter, r *http.Request) {
	clientID := googleClientID()
	var req GoogleLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	claims, err := VerifyGoogleIDToken(req.Credential, clientID)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}

	cust, err := s.upsertGoogleCustomer(claims)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not link google account: " + err.Error()})
		return
	}

	tok, exp, err := s.Sign(cust.ID, cust.Email, cust.Role, cust.Name)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not sign token"})
		return
	}
	writeJSON(w, http.StatusOK, LoginResponse{Token: tok, ExpiresAt: exp, User: cust})
}

// upsertGoogleCustomer faz match por google_sub OU por e-mail; cria se não existir.
func (s *Service) upsertGoogleCustomer(c *GoogleClaims) (Customer, error) {
	email := strings.ToLower(strings.TrimSpace(c.Email))

	// 1. Tenta achar por google_sub
	var cust Customer
	err := s.DB.QueryRow(
		`SELECT id, name, email, COALESCE(phone, ''), role, created_at, updated_at
		 FROM customers WHERE google_sub = $1`,
		c.SUB,
	).Scan(&cust.ID, &cust.Name, &cust.Email, &cust.Phone, &cust.Role, &cust.CreatedAt, &cust.UpdatedAt)
	if err == nil {
		return cust, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Customer{}, err
	}

	// 2. Tenta achar por email — se existir, vincula a conta Google.
	err = s.DB.QueryRow(
		`UPDATE customers SET google_sub = $1, picture = COALESCE(NULLIF($2, ''), picture), updated_at = now()
		 WHERE email = $3
		 RETURNING id, name, email, COALESCE(phone, ''), role, created_at, updated_at`,
		c.SUB, c.Picture, email,
	).Scan(&cust.ID, &cust.Name, &cust.Email, &cust.Phone, &cust.Role, &cust.CreatedAt, &cust.UpdatedAt)
	if err == nil {
		return cust, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Customer{}, err
	}

	// 3. Cria novo cliente.
	// Senha aleatória só pra não ficar NULL (campo agora é NULLABLE, mas mantemos algo).
	rnd, _ := randomHex(16)
	hash, _ := HashPassword("g_" + rnd) // só para preencher; usuário só faz login pelo Google
	name := c.Name
	if name == "" {
		name = strings.Split(email, "@")[0]
	}
	err = s.DB.QueryRow(
		`INSERT INTO customers (name, email, password_hash, role, google_sub, picture)
		 VALUES ($1, $2, $3, 'customer', $4, $5)
		 RETURNING id, name, email, COALESCE(phone, ''), role, created_at, updated_at`,
		name, email, hash, c.SUB, c.Picture,
	).Scan(&cust.ID, &cust.Name, &cust.Email, &cust.Phone, &cust.Role, &cust.CreatedAt, &cust.UpdatedAt)
	if err != nil {
		return Customer{}, err
	}
	return cust, nil
}

// GoogleConfig devolve o client_id para o frontend usar no GSI.
// @Summary  Retorna a configuração pública do login com Google
// @Tags     auth
// @Produce  json
// @Success  200 {object} map[string]string
// @Router   /api/auth/google/config [get]
func GoogleConfigHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"client_id": googleClientID(),
	})
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
