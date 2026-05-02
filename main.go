// @title           Florentina API
// @version         2.0
// @description     E-commerce Florentina: usuários, catálogo recursivo, Pix Asaas, entrega Lalamove.
// @host            localhost:8080
// @BasePath        /
// @securityDefinitions.apikey BearerAuth
// @in              header
// @name            Authorization

package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	_ "github.com/lib/pq"
	httpSwagger "github.com/swaggo/http-swagger"

	"userapi/auth"
	"userapi/catalog"
	"userapi/cep"
	_ "userapi/docs"
	"userapi/lalamove"
	"userapi/migrations"
	"userapi/notifier"
	"userapi/orders"
	"userapi/payments"
	"userapi/routing"
	"userapi/tracking"
	"github.com/joho/godotenv"
)

// User representa um usuário do sistema (legado).
type User struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateUserRequest body para criação e atualização de usuário (legado).
type CreateUserRequest struct {
	Name  string `json:"name"  example:"João Silva"`
	Email string `json:"email" example:"joao@email.com"`
}

type store struct {
	db *sql.DB
}

func newStore(db *sql.DB) *store { return &store{db: db} }

func (s *store) list() ([]User, error) {
	rows, err := s.db.Query(`SELECT id, name, email, created_at, updated_at FROM users ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := []User{}
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *store) get(id string) (User, bool, error) {
	var u User
	err := s.db.QueryRow(
		`SELECT id, name, email, created_at, updated_at FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Name, &u.Email, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return User{}, false, nil
	}
	return u, err == nil, err
}

func (s *store) create(u User) (User, error) {
	err := s.db.QueryRow(
		`INSERT INTO users (name, email) VALUES ($1, $2) RETURNING id, name, email, created_at, updated_at`,
		u.Name, u.Email,
	).Scan(&u.ID, &u.Name, &u.Email, &u.CreatedAt, &u.UpdatedAt)
	return u, err
}

func (s *store) update(id string, u User) (User, bool, error) {
	err := s.db.QueryRow(
		`UPDATE users SET name=$1, email=$2, updated_at=now() WHERE id=$3 RETURNING id, name, email, created_at, updated_at`,
		u.Name, u.Email, id,
	).Scan(&u.ID, &u.Name, &u.Email, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return User{}, false, nil
	}
	return u, err == nil, err
}

func (s *store) delete(id string) (bool, error) {
	res, err := s.db.Exec(`DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// gatewayError loga o erro real e retorna mensagem genérica ao cliente.
func gatewayError(w http.ResponseWriter, msg string, err error) {
	slog.Error(msg, "error", err)
	writeError(w, http.StatusBadGateway, "serviço externo indisponível, tente novamente")
}

func extractID(path, prefix string) string {
	return strings.Trim(strings.TrimPrefix(path, prefix), "/")
}

func decode(r *http.Request, v any) bool {
	return json.NewDecoder(r.Body).Decode(v) == nil
}

func pathSegment(path, prefix string, n int) string {
	rest := strings.TrimPrefix(path, prefix)
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if n < len(parts) {
		return parts[n]
	}
	return ""
}

// ================================================================
// User handlers (legados)
// ================================================================

func listUsers(s *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		users, err := s.list()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not list users")
			return
		}
		writeJSON(w, http.StatusOK, users)
	}
}

func createUser(s *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body User
		if !decode(r, &body) {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
		if body.Name == "" || body.Email == "" {
			writeError(w, http.StatusBadRequest, "name and email are required")
			return
		}
		created, err := s.create(body)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not create user")
			return
		}
		writeJSON(w, http.StatusCreated, created)
	}
}

func getUser(s *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := extractID(r.URL.Path, "/users/")
		u, ok, err := s.get(id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not get user")
			return
		}
		if !ok {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeJSON(w, http.StatusOK, u)
	}
}

func updateUser(s *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := extractID(r.URL.Path, "/users/")
		var body User
		if !decode(r, &body) {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
		if body.Name == "" || body.Email == "" {
			writeError(w, http.StatusBadRequest, "name and email are required")
			return
		}
		updated, ok, err := s.update(id, body)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not update user")
			return
		}
		if !ok {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeJSON(w, http.StatusOK, updated)
	}
}

func deleteUser(s *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := extractID(r.URL.Path, "/users/")
		ok, err := s.delete(id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not delete user")
			return
		}
		if !ok {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ================================================================
// Lalamove handlers
// ================================================================

func createQuotation(lala *lalamove.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req lalamove.QuotationRequest
		if !decode(r, &req) {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
		resp, err := lala.CreateQuotation(req)
		if err != nil {
			gatewayError(w, "lalamove create quotation", err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func fetchQuotation(lala *lalamove.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := extractID(r.URL.Path, "/delivery/quotations/")
		resp, err := lala.GetQuotation(id)
		if err != nil {
			gatewayError(w, "lalamove fetch quotation", err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func placeOrder(lala *lalamove.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req lalamove.OrderRequest
		if !decode(r, &req) {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
		resp, err := lala.PlaceOrder(req)
		if err != nil {
			gatewayError(w, "lalamove", err)
			return
		}
		writeJSON(w, http.StatusCreated, resp)
	}
}

func getOrder(lala *lalamove.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := pathSegment(r.URL.Path, "/delivery/orders/", 0)
		resp, err := lala.GetOrder(id)
		if err != nil {
			gatewayError(w, "lalamove", err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func editOrder(lala *lalamove.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := pathSegment(r.URL.Path, "/delivery/orders/", 0)
		var req lalamove.EditOrderRequest
		if !decode(r, &req) {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
		resp, err := lala.EditOrder(id, req)
		if err != nil {
			gatewayError(w, "lalamove", err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func cancelOrder(lala *lalamove.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := pathSegment(r.URL.Path, "/delivery/orders/", 0)
		if err := lala.CancelOrder(id); err != nil {
			gatewayError(w, "lalamove", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func getDriver(lala *lalamove.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orderID := pathSegment(r.URL.Path, "/delivery/orders/", 0)
		driverID := pathSegment(r.URL.Path, "/delivery/orders/", 2)
		resp, err := lala.GetDriver(orderID, driverID)
		if err != nil {
			gatewayError(w, "lalamove", err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func changeDriver(lala *lalamove.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orderID := pathSegment(r.URL.Path, "/delivery/orders/", 0)
		driverID := pathSegment(r.URL.Path, "/delivery/orders/", 2)
		var req lalamove.ChangeDriverRequest
		if !decode(r, &req) {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
		if err := lala.ChangeDriver(orderID, driverID, req); err != nil {
			gatewayError(w, "lalamove", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func addPriorityFee(lala *lalamove.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := pathSegment(r.URL.Path, "/delivery/orders/", 0)
		var req lalamove.PriorityFeeRequest
		if !decode(r, &req) {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
		resp, err := lala.AddPriorityFee(id, req)
		if err != nil {
			gatewayError(w, "lalamove", err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func getCities(lala *lalamove.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp, err := lala.GetCities()
		if err != nil {
			gatewayError(w, "lalamove", err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func setWebhook(lala *lalamove.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req lalamove.WebhookRequest
		if !decode(r, &req) {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
		resp, err := lala.SetWebhook(req)
		if err != nil {
			gatewayError(w, "lalamove", err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// WebhookEvent é o payload enviado pela Lalamove.
type WebhookEvent struct {
	Events []struct {
		Event string `json:"event"`
		Order struct {
			OrderID  string `json:"orderId"`
			Status   string `json:"status"`
			DriverID string `json:"driverId"`
		} `json:"order"`
		Driver *struct {
			DriverID    string `json:"driverId"`
			Name        string `json:"name"`
			Phone       string `json:"phone"`
			PlateNumber string `json:"plateNumber"`
			Photo       string `json:"photo"`
			Coordinates *struct {
				Lat       string `json:"lat"`
				Lng       string `json:"lng"`
				UpdatedAt string `json:"updatedAt"`
			} `json:"coordinates"`
		} `json:"driver"`
	} `json:"events"`
}

func webhookHandler(hub *tracking.Hub, lala *lalamove.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload WebhookEvent
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeError(w, http.StatusBadRequest, "invalid payload")
			return
		}

		for _, e := range payload.Events {
			orderID := e.Order.OrderID
			update := tracking.LocationUpdate{
				OrderID:  orderID,
				Status:   e.Order.Status,
				DriverID: e.Order.DriverID,
			}

			if e.Driver != nil {
				update.Name = e.Driver.Name
				update.Phone = e.Driver.Phone
				update.Plate = e.Driver.PlateNumber
				update.Photo = e.Driver.Photo
				if e.Driver.Coordinates != nil {
					update.Lat = e.Driver.Coordinates.Lat
					update.Lng = e.Driver.Coordinates.Lng
					update.UpdatedAt = e.Driver.Coordinates.UpdatedAt
				}
			}

			if update.DriverID != "" && update.Lat == "" {
				if driver, err := lala.GetDriver(orderID, update.DriverID); err == nil {
					update.Name = driver.Data.Name
					update.Phone = driver.Data.Phone
					update.Plate = driver.Data.PlateNumber
					update.Photo = driver.Data.Photo
					update.Lat = driver.Data.Coordinates.Lat
					update.Lng = driver.Data.Coordinates.Lng
					update.UpdatedAt = driver.Data.Coordinates.UpdatedAt
				}
			}

			hub.Push(orderID, update)
		}
		w.WriteHeader(http.StatusOK)
	}
}

func simulateTracking(hub *tracking.Hub, lala *lalamove.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orderID := extractID(r.URL.Path, "/debug/simulate/")

		order, err := lala.GetOrder(orderID)
		if err != nil {
			writeError(w, http.StatusBadGateway, "could not fetch order: "+err.Error())
			return
		}

		stops := order.Data.Stops
		if len(stops) < 2 {
			writeError(w, http.StatusBadRequest, "order has less than 2 stops")
			return
		}

		var routeStops []routing.Point
		for _, s := range stops {
			lat, _ := strconv.ParseFloat(s.Coordinates.Lat, 64)
			lng, _ := strconv.ParseFloat(s.Coordinates.Lng, 64)
			routeStops = append(routeStops, routing.Point{Lat: lat, Lng: lng})
		}

		route, err := routing.GetRoute(routeStops)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "routing error: "+err.Error())
			return
		}

		routePoints := make([]tracking.RoutePoint, len(route.Points))
		for i, p := range route.Points {
			routePoints[i] = tracking.RoutePoint{Lat: p.Lat, Lng: p.Lng}
		}
		origin := &tracking.RoutePoint{Lat: routeStops[0].Lat, Lng: routeStops[0].Lng}
		dest := &tracking.RoutePoint{Lat: routeStops[len(routeStops)-1].Lat, Lng: routeStops[len(routeStops)-1].Lng}

		go func() {
			total := len(routePoints)
			hub.Push(orderID, tracking.LocationUpdate{
				OrderID: orderID, Status: "ON_GOING",
				DriverID: "sim-driver-001", Name: "Carlos Silva (Simulação)",
				Phone: "+5511999990000", Plate: "ABC-1234",
				Lat: fmt.Sprintf("%f", routePoints[0].Lat),
				Lng: fmt.Sprintf("%f", routePoints[0].Lng),
				UpdatedAt: time.Now().UTC().Format(time.RFC3339),
				Route:     routePoints, Origin: origin, Dest: dest,
			})

			interval := time.Duration(int(route.Duration*1000)/total) * time.Millisecond
			if interval < 300*time.Millisecond {
				interval = 300 * time.Millisecond
			}
			if interval > 3*time.Second {
				interval = 3 * time.Second
			}

			for i, pt := range routePoints {
				status := "ON_GOING"
				if i == 0 {
					status = "ASSIGNING_DRIVER"
				} else if i == total-1 {
					status = "COMPLETED"
				} else if i == total/2 {
					status = "PICKED_UP"
				}

				hub.Push(orderID, tracking.LocationUpdate{
					OrderID: orderID, Status: status,
					DriverID: "sim-driver-001", Name: "Carlos Silva (Simulação)",
					Phone: "+5511999990000", Plate: "ABC-1234",
					Lat: fmt.Sprintf("%f", pt.Lat), Lng: fmt.Sprintf("%f", pt.Lng),
					UpdatedAt: time.Now().UTC().Format(time.RFC3339),
				})
				time.Sleep(interval)
			}
		}()

		writeJSON(w, http.StatusOK, map[string]any{
			"status":    "simulation started",
			"orderId":   orderID,
			"waypoints": len(routePoints),
			"distanceM": route.Distance,
			"durationS": route.Duration,
		})
	}
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func trackingPage(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "static/tracking.html")
}

func trackingWS(hub *tracking.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orderID := extractID(r.URL.Path, "/ws/tracking/")
		if orderID == "" {
			writeError(w, http.StatusBadRequest, "missing orderId")
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		hub.Subscribe(orderID, conn)
	}
}

// ================================================================
// CORS
// ================================================================

// ================================================================
// MIDDLEWARES
// ================================================================

func corsMiddleware(next http.Handler) http.Handler {
	allowedOrigins := os.Getenv("CORS_ALLOWED_ORIGINS")
	isProd := os.Getenv("ASAAS_ENV") == "production"
	if allowedOrigins == "" {
		if isProd {
			slog.Warn("CORS_ALLOWED_ORIGINS não definido em produção — requisições cross-origin serão bloqueadas")
			allowedOrigins = "" // bloqueia tudo por padrão em produção
		} else {
			allowedOrigins = "*" // libera apenas em desenvolvimento
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if allowedOrigins == "*" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else if origin != "" && strings.Contains(allowedOrigins, origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, asaas-access-token")

		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// bodyLimitMiddleware rejeita bodies maiores que maxBytes em POST/PUT/PATCH.
func bodyLimitMiddleware(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch:
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// rateLimiter conta tentativas por IP dentro de uma janela de tempo.
type rateLimiter struct {
	sync.Mutex
	window   time.Duration
	maxReqs  int
	entries  map[string][]time.Time
}

func newRateLimiter(window time.Duration, maxReqs int) *rateLimiter {
	rl := &rateLimiter{
		window:  window,
		maxReqs: maxReqs,
		entries: make(map[string][]time.Time),
	}
	// limpa entradas antigas periodicamente
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			rl.Lock()
			cutoff := time.Now().Add(-rl.window)
			for ip, times := range rl.entries {
				var fresh []time.Time
				for _, t := range times {
					if t.After(cutoff) {
						fresh = append(fresh, t)
					}
				}
				if len(fresh) == 0 {
					delete(rl.entries, ip)
				} else {
					rl.entries[ip] = fresh
				}
			}
			rl.Unlock()
		}
	}()
	return rl
}

func (rl *rateLimiter) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		if i := strings.LastIndex(ip, ":"); i != -1 {
			ip = ip[:i]
		}
		// respeita X-Forwarded-For se houver (proxy/load balancer)
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			if parts := strings.Split(fwd, ","); len(parts) > 0 {
				ip = strings.TrimSpace(parts[0])
			}
		}
		now := time.Now()
		cutoff := now.Add(-rl.window)
		rl.Lock()
		times := rl.entries[ip]
		var fresh []time.Time
		for _, t := range times {
			if t.After(cutoff) {
				fresh = append(fresh, t)
			}
		}
		if len(fresh) >= rl.maxReqs {
			rl.Unlock()
			w.Header().Set("Retry-After", strconv.Itoa(int(rl.window.Seconds())))
			writeError(w, http.StatusTooManyRequests, "too many requests, try again later")
			return
		}
		rl.entries[ip] = append(fresh, now)
		rl.Unlock()
		next.ServeHTTP(w, r)
	})
}

// authLimiter: máx 10 tentativas por minuto por IP (login, registro, Google)
var authLimiter = newRateLimiter(time.Minute, 10)

// methodGate roteia por método HTTP usando um mapa.
func methodGate(handlers map[string]http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h, ok := handlers[r.Method]; ok {
			h(w, r)
			return
		}
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func main() {
	// Initialize Logger
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	// Load .env
	if err := godotenv.Load(); err != nil {
		slog.Info("no .env file found, using system environment variables")
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		slog.Error("DATABASE_URL must be set")
		os.Exit(1)
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		slog.Error("db open", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		slog.Error("db ping", "error", err)
		os.Exit(1)
	}

	if err := migrations.Run(db); err != nil {
		slog.Error("migrate", "error", err)
		os.Exit(1)
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		slog.Error("JWT_SECRET must be set")
		os.Exit(1)
	}

	// ---- Auth ----
	authSvc := auth.New(db, jwtSecret)
	if err := auth.EnsureAdmin(
		db,
		os.Getenv("ADMIN_EMAIL"),
		os.Getenv("ADMIN_PASSWORD"),
		coalesce(os.Getenv("ADMIN_NAME"), "Administrador"),
	); err != nil {
		slog.Error("ensure admin", "error", err)
	}

	// ---- Lalamove ----
	lalaKey := os.Getenv("LALAMOVE_API_KEY")
	lalaSecret := os.Getenv("LALAMOVE_API_SECRET")
	lalaMarket := os.Getenv("LALAMOVE_MARKET")
	if lalaMarket == "" {
		lalaMarket = "BR"
	}
	lalaBaseURL := os.Getenv("LALAMOVE_BASE_URL") // vazio = sandbox; em produção usar https://rest.lalamove.com
	lala := lalamove.NewClient(lalaKey, lalaSecret, lalaMarket, lalaBaseURL)
	hub := tracking.NewHub(lala)

	// ---- Asaas ----
	asaasKey := os.Getenv("ASAAS_API_KEY")
	asaasSandbox := os.Getenv("ASAAS_ENV") != "production"
	asaas := payments.NewAsaasClient(asaasKey, asaasSandbox)

	// ---- CEP ----
	cepSvc := cep.New()

	// ---- Notifier ----
	notif := notifier.New()

	// ---- Catalog ----
	catSvc := catalog.NewCategoryStore(db)
	prodSvc := catalog.NewProductStore(db)

	// ---- Orders ----
	orderSvc := orders.New(db, asaas, lala, cepSvc, notif, os.Getenv("ASAAS_WEBHOOK_TOKEN"))

	s := newStore(db)
	mux := http.NewServeMux()

	// ====== STATIC FILES ======
	// O site (loja, login, checkout, admin) fica em /site/.
	siteFS := http.FileServer(http.Dir("site"))
	mux.Handle("/site/", http.StripPrefix("/site/", siteFS))
	staticFS := http.FileServer(http.Dir("static"))
	mux.Handle("/static/", http.StripPrefix("/static/", staticFS))

	// Raiz redireciona para a loja
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/site/index.html", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	})

	// ====== SWAGGER (apenas fora de produção) ======
	if os.Getenv("ASAAS_ENV") != "production" {
		mux.HandleFunc("/swagger/", httpSwagger.WrapHandler)
	}

	// ====== TRACKING ======
	mux.HandleFunc("/tracking", trackingPage)
	mux.HandleFunc("/ws/tracking/", trackingWS(hub))
	mux.HandleFunc("/debug/simulate/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		simulateTracking(hub, lala)(w, r)
	})
	mux.HandleFunc("/webhook/lalamove", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.WriteHeader(http.StatusOK)
		case http.MethodPost:
			webhookHandler(hub, lala)(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})

	// ====== AUTH (novo) ======
	mux.Handle("/api/auth/register", authLimiter.Limit(methodGate(map[string]http.HandlerFunc{
		http.MethodPost: authSvc.Register,
	})))
	mux.Handle("/api/auth/login", authLimiter.Limit(methodGate(map[string]http.HandlerFunc{
		http.MethodPost: authSvc.Login,
	})))
	mux.Handle("/api/auth/google", authLimiter.Limit(methodGate(map[string]http.HandlerFunc{
		http.MethodPost: authSvc.Google,
	})))
	mux.HandleFunc("/api/auth/google/config", methodGate(map[string]http.HandlerFunc{
		http.MethodGet: auth.GoogleConfigHandler,
	}))
	mux.Handle("/api/auth/me", authSvc.Middleware(http.HandlerFunc(authSvc.Me)))

	// ====== CEP ======
	mux.HandleFunc("/api/cep", cepSvc.LookupHandler)

	// ====== CATEGORIES ======
	mux.HandleFunc("/api/categories", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			catSvc.ListHandler(w, r)
		case http.MethodPost:
			authSvc.AdminOnly(http.HandlerFunc(catSvc.CreateHandler)).ServeHTTP(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})
	mux.HandleFunc("/api/categories/", func(w http.ResponseWriter, r *http.Request) {
		id := extractID(r.URL.Path, "/api/categories/")
		if id == "" {
			writeError(w, http.StatusBadRequest, "missing category id")
			return
		}
		switch r.Method {
		case http.MethodPut, http.MethodPatch:
			authSvc.AdminOnly(catSvc.UpdateHandler(id)).ServeHTTP(w, r)
		case http.MethodDelete:
			authSvc.AdminOnly(catSvc.DeleteHandler(id)).ServeHTTP(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})

	// ====== PRODUCTS ======
	mux.HandleFunc("/api/products", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			prodSvc.ListHandler(catSvc)(w, r)
		case http.MethodPost:
			authSvc.AdminOnly(http.HandlerFunc(prodSvc.CreateHandler)).ServeHTTP(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})
	mux.HandleFunc("/api/products/", func(w http.ResponseWriter, r *http.Request) {
		id := extractID(r.URL.Path, "/api/products/")
		if id == "" {
			writeError(w, http.StatusBadRequest, "missing product id")
			return
		}
		switch r.Method {
		case http.MethodGet:
			prodSvc.GetHandler(id)(w, r)
		case http.MethodPut, http.MethodPatch:
			authSvc.AdminOnly(prodSvc.UpdateHandler(id)).ServeHTTP(w, r)
		case http.MethodDelete:
			authSvc.AdminOnly(prodSvc.DeleteHandler(id)).ServeHTTP(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})

	// ====== ORDERS ======
	mux.HandleFunc("/api/orders", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			authSvc.Optional(http.HandlerFunc(orderSvc.CreateHandler)).ServeHTTP(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})
	mux.HandleFunc("/api/orders/", func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/api/orders/")
		parts := strings.Split(strings.Trim(rest, "/"), "/")
		switch {
		case len(parts) == 1 && parts[0] != "":
			id := parts[0]
			authSvc.Optional(orderSvc.GetHandler(id)).ServeHTTP(w, r)
		case len(parts) == 2 && parts[1] == "payment-status":
			id := parts[0]
			orderSvc.PaymentStatusHandler(id)(w, r)
		default:
			writeError(w, http.StatusNotFound, "route not found")
		}
	})
	mux.Handle("/api/me/orders", authSvc.Middleware(http.HandlerFunc(orderSvc.MyOrdersHandler)))

	// ====== ADMIN ORDERS ======
	mux.Handle("/api/admin/orders", authSvc.AdminOnly(http.HandlerFunc(orderSvc.AdminListHandler)))
	mux.HandleFunc("/api/admin/orders/", func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/api/admin/orders/")
		parts := strings.Split(strings.Trim(rest, "/"), "/")
		if len(parts) == 2 && parts[1] == "ready" && r.Method == http.MethodPost {
			authSvc.AdminOnly(orderSvc.MarkReadyHandler(parts[0])).ServeHTTP(w, r)
			return
		}
		writeError(w, http.StatusNotFound, "route not found")
	})

	// ====== ASAAS WEBHOOK ======
	mux.HandleFunc("/webhook/asaas", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusOK)
			return
		}
		orderSvc.AsaasWebhookHandler(w, r)
	})

	// ====== NOTIFICATIONS WS (cliente recebe payment_confirmed / ready) ======
	// Browsers não enviam Authorization header em WebSocket; o token vem como ?token=
	mux.HandleFunc("/ws/orders/", func(w http.ResponseWriter, r *http.Request) {
		orderID := extractID(r.URL.Path, "/ws/orders/")
		if orderID == "" {
			writeError(w, http.StatusBadRequest, "missing orderId")
			return
		}
		// Valida token query-param contra o pedido
		// Tenta autenticar: header Authorization ou query-param ?token= (WS não envia header)
		if tok := r.URL.Query().Get("token"); tok != "" {
			if claims, err := authSvc.Parse(tok); err == nil {
				r = r.WithContext(auth.IntoContext(r.Context(), claims))
			}
		}
		// Verifica acesso: pedido anônimo é livre; pedido com dono exige auth
		order, err := orderSvc.Get(orderID)
		if err != nil {
			writeError(w, http.StatusNotFound, "order not found")
			return
		}
		if order.CustomerID != nil {
			claims, ok := auth.FromContext(r.Context())
			if !ok {
				writeError(w, http.StatusUnauthorized, "authentication required")
				return
			}
			if claims.Role != "admin" && *order.CustomerID != claims.Sub {
				writeError(w, http.StatusForbidden, "forbidden")
				return
			}
		}
		notif.Subscribe(orderID, w, r)
	})

	// ====== USERS legados ======
	mux.HandleFunc("/users", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listUsers(s)(w, r)
		case http.MethodPost:
			createUser(s)(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})
	mux.HandleFunc("/users/", func(w http.ResponseWriter, r *http.Request) {
		if extractID(r.URL.Path, "/users/") == "" {
			writeError(w, http.StatusBadRequest, "missing user id")
			return
		}
		switch r.Method {
		case http.MethodGet:
			getUser(s)(w, r)
		case http.MethodPut:
			updateUser(s)(w, r)
		case http.MethodDelete:
			deleteUser(s)(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})

	// ====== DELIVERY (Lalamove direto) ======
	mux.HandleFunc("/delivery/cities", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		getCities(lala)(w, r)
	})
	mux.HandleFunc("/delivery/webhook", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		setWebhook(lala)(w, r)
	})
	mux.HandleFunc("/delivery/quotations", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		createQuotation(lala)(w, r)
	})
	mux.HandleFunc("/delivery/quotations/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		fetchQuotation(lala)(w, r)
	})
	mux.HandleFunc("/delivery/orders", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		placeOrder(lala)(w, r)
	})
	mux.HandleFunc("/delivery/orders/", func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/delivery/orders/")
		parts := strings.Split(strings.Trim(rest, "/"), "/")

		switch {
		case len(parts) == 3 && parts[1] == "drivers":
			switch r.Method {
			case http.MethodGet:
				getDriver(lala)(w, r)
			case http.MethodDelete:
				changeDriver(lala)(w, r)
			default:
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			}
		case len(parts) == 2 && parts[1] == "priority-fee":
			if r.Method != http.MethodPost {
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			addPriorityFee(lala)(w, r)
		case len(parts) == 1:
			switch r.Method {
			case http.MethodGet:
				getOrder(lala)(w, r)
			case http.MethodPatch:
				editOrder(lala)(w, r)
			case http.MethodDelete:
				cancelOrder(lala)(w, r)
			default:
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			}
		default:
			writeError(w, http.StatusNotFound, "route not found")
		}
	})

	addr := ":" + coalesce(os.Getenv("PORT"), "8080")
	slog.Info("server running", "addr", addr)
	slog.Info("links",
		"loja", "http://localhost"+addr+"/site/index.html",
		"admin", "http://localhost"+addr+"/site/admin.html",
		"swagger", "http://localhost"+addr+"/swagger/index.html",
	)
	handler := corsMiddleware(bodyLimitMiddleware(2 << 20)(mux)) // 2 MB
	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 90 * time.Second, // maior que ReadTimeout para suportar WebSocket
		IdleTimeout:  120 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("server fatal", "error", err)
		os.Exit(1)
	}
}

func coalesce(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
