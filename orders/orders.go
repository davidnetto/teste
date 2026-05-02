// Package orders implementa o ciclo de pedido: carrinho → cobrança Pix → entrega Lalamove.
package orders

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"userapi/auth"
	"userapi/cep"
	"userapi/lalamove"
	"userapi/notifier"
	"userapi/payments"
)

// Item é uma linha do carrinho.
type Item struct {
	ProductID    string `json:"product_id"`
	Name         string `json:"name"`
	Quantity     int    `json:"quantity"`
	UnitCents    *int   `json:"unit_cents"`
	UnderConsult bool   `json:"under_consult"`
}

// Address é o endereço de entrega.
type Address struct {
	CEP          string  `json:"cep"`
	Street       string  `json:"street"`
	Number       string  `json:"number"`
	Complement   string  `json:"complement"`
	Neighborhood string  `json:"neighborhood"`
	City         string  `json:"city"`
	State        string  `json:"state"`
	Lat          float64 `json:"lat"`
	Lng          float64 `json:"lng"`
}

// CreateRequest é o body do POST /api/orders.
type CreateRequest struct {
	CustomerName  string  `json:"customer_name"`
	CustomerEmail string  `json:"customer_email"`
	CustomerPhone string  `json:"customer_phone"`
	CpfCnpj       string  `json:"cpf_cnpj"`
	Address       Address `json:"address"`
	Items         []Item  `json:"items"`
	Notes         string  `json:"notes"`
	ShippingCents int     `json:"shipping_cents"`
	QuotationID   string  `json:"quotation_id,omitempty"`
}

// Order representa um pedido persistido.
type Order struct {
	ID                  string    `json:"id"`
	CustomerID          *string   `json:"customer_id"`
	CustomerName        string    `json:"customer_name"`
	CustomerEmail       string    `json:"customer_email"`
	CustomerPhone       string    `json:"customer_phone"`
	Address             Address   `json:"address"`
	Items               []Item    `json:"items"`
	SubtotalCents       int       `json:"subtotal_cents"`
	ShippingCents       int       `json:"shipping_cents"`
	TotalCents          int       `json:"total_cents"`
	Status              string    `json:"status"`
	PaymentStatus       string    `json:"payment_status"`
	LalamoveQuotationID string    `json:"lalamove_quotation_id,omitempty"`
	LalamoveOrderID     string    `json:"lalamove_order_id,omitempty"`
	Notes               string    `json:"notes"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	Payment             *Payment  `json:"payment,omitempty"`
}

// Payment é o resumo do pagamento (Pix).
type Payment struct {
	ProviderPaymentID string    `json:"provider_payment_id"`
	Status            string    `json:"status"`
	AmountCents       int       `json:"amount_cents"`
	PixQRCodeBase64   string    `json:"pix_qr_code_base64"`
	PixPayload        string    `json:"pix_payload"`
	PixExpiresAt      time.Time `json:"pix_expires_at"`
	PaidAt            *time.Time `json:"paid_at,omitempty"`
}

// Service é o serviço de pedidos.
type Service struct {
	DB       *sql.DB
	Asaas    *payments.AsaasClient
	Lala     *lalamove.Client
	CEP      *cep.Service
	Notifier *notifier.Hub
	WebhookSecret string
}

// New cria um service.
func New(db *sql.DB, asaas *payments.AsaasClient, lala *lalamove.Client, cepSvc *cep.Service, hub *notifier.Hub, webhookSecret string) *Service {
	return &Service{
		DB:       db,
		Asaas:    asaas,
		Lala:     lala,
		CEP:      cepSvc,
		Notifier: hub,
		WebhookSecret: webhookSecret,
	}
}

// Create persiste o pedido + cria cobrança Pix no Asaas.
func (s *Service) Create(req CreateRequest, customerID *string) (*Order, error) {
	if len(req.Items) == 0 {
		return nil, errors.New("items é obrigatório")
	}
	// valida CEP / cidade
	if !s.CEP.IsDeliverable(req.Address.City, req.Address.State) {
		return nil, errors.New("não entregamos nessa região no momento")
	}

	// valida e reescreve preços a partir do banco — nunca confia no cliente
	for i, it := range req.Items {
		if it.ProductID == "" {
			continue
		}
		var dbPrice sql.NullInt64
		var isActive bool
		err := s.DB.QueryRow(
			`SELECT price_cents, is_active FROM products WHERE id = $1`, it.ProductID,
		).Scan(&dbPrice, &isActive)
		if err != nil {
			return nil, errors.New("produto não encontrado: " + it.ProductID)
		}
		if !isActive {
			return nil, errors.New("produto indisponível: " + it.Name)
		}
		if !dbPrice.Valid {
			req.Items[i].UnderConsult = true
			req.Items[i].UnitCents = nil
		} else {
			v := int(dbPrice.Int64)
			req.Items[i].UnitCents = &v
		}
	}

	// calcula subtotal — items sob consulta não somam
	var subtotal int
	hasConsult := false
	for _, it := range req.Items {
		if it.Quantity <= 0 {
			it.Quantity = 1
		}
		if it.UnderConsult || it.UnitCents == nil {
			hasConsult = true
			continue
		}
		subtotal += *it.UnitCents * it.Quantity
	}

	// frete: validado contra a cotação Lalamove armazenada (se houver quotation_id)
	shippingCents, err := s.validatedShipping(req.QuotationID, req.ShippingCents)
	if err != nil {
		return nil, err
	}
	req.ShippingCents = shippingCents
	total := subtotal + req.ShippingCents

	tx, err := s.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var custIDArg any
	if customerID != nil {
		custIDArg = *customerID
	}

	var o Order
	var custIDOut sql.NullString
	err = tx.QueryRow(`
		INSERT INTO orders (
			customer_id, customer_name, customer_email, customer_phone,
			cep, street, number, complement, neighborhood, city, state, lat, lng,
			subtotal_cents, shipping_cents, total_cents, status, payment_status,
			lalamove_quotation_id, notes
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
		RETURNING id, customer_id, customer_name, customer_email, customer_phone,
		          cep, street, COALESCE(number,''), COALESCE(complement,''), COALESCE(neighborhood,''), city, state,
		          COALESCE(lat,0), COALESCE(lng,0),
		          subtotal_cents, shipping_cents, total_cents, status, payment_status,
		          COALESCE(lalamove_quotation_id, ''), COALESCE(lalamove_order_id, ''),
		          COALESCE(notes, ''), created_at, updated_at`,
		custIDArg, req.CustomerName, strings.ToLower(req.CustomerEmail), req.CustomerPhone,
		req.Address.CEP, req.Address.Street, req.Address.Number, req.Address.Complement, req.Address.Neighborhood, req.Address.City, req.Address.State, req.Address.Lat, req.Address.Lng,
		subtotal, req.ShippingCents, total, statusForOrder(hasConsult, total), "pending",
		req.QuotationID, req.Notes,
	).Scan(
		&o.ID, &custIDOut, &o.CustomerName, &o.CustomerEmail, &o.CustomerPhone,
		&o.Address.CEP, &o.Address.Street, &o.Address.Number, &o.Address.Complement, &o.Address.Neighborhood, &o.Address.City, &o.Address.State,
		&o.Address.Lat, &o.Address.Lng,
		&o.SubtotalCents, &o.ShippingCents, &o.TotalCents, &o.Status, &o.PaymentStatus,
		&o.LalamoveQuotationID, &o.LalamoveOrderID, &o.Notes, &o.CreatedAt, &o.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if custIDOut.Valid {
		v := custIDOut.String
		o.CustomerID = &v
	}

	// items
	for _, it := range req.Items {
		var unit, sub sql.NullInt64
		if !it.UnderConsult && it.UnitCents != nil {
			unit.Valid = true
			unit.Int64 = int64(*it.UnitCents)
			sub.Valid = true
			sub.Int64 = int64(*it.UnitCents * it.Quantity)
		}
		_, err := tx.Exec(
			`INSERT INTO order_items (order_id, product_id, product_name, quantity, unit_cents, subtotal_cents, under_consult)
			 VALUES ($1, NULLIF($2,'')::uuid, $3, $4, $5, $6, $7)`,
			o.ID, it.ProductID, it.Name, it.Quantity, unit, sub, it.UnderConsult || it.UnitCents == nil,
		)
		if err != nil {
			return nil, err
		}
	}
	o.Items = req.Items

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	// Se tem item sob consulta OU total = 0, não cobra Pix agora — aguarda contato.
	if total > 0 && !hasConsult && s.Asaas != nil {
		pay, err := s.createPix(o, req.CpfCnpj)
		if err != nil {
			// não falha o pedido — só registra que o Pix não foi gerado
			slog.Error("não foi possível criar Pix", "order_id", o.ID, "error", err)
		} else {
			o.Payment = pay
		}
	}

	return &o, nil
}

func statusForOrder(hasConsult bool, total int) string {
	if hasConsult || total == 0 {
		return "pending_quote"
	}
	return "pending_payment"
}

func (s *Service) createPix(o Order, cpfCnpj string) (*Payment, error) {
	cust, err := s.Asaas.CreateCustomer(o.CustomerName, o.CustomerEmail, o.CustomerPhone, cpfCnpj)
	if err != nil {
		return nil, err
	}
	pay, err := s.Asaas.CreatePixPayment(payments.PaymentRequest{
		Customer:    cust.ID,
		BillingType: "PIX",
		Value:       float64(o.TotalCents) / 100.0,
		Description: "Pedido " + o.ID[:8] + " - Florentina",
		ExternalRef: o.ID,
	})
	if err != nil {
		return nil, err
	}
	qr, err := s.Asaas.GetPixQRCode(pay.ID)
	if err != nil {
		return nil, err
	}
	exp, _ := time.Parse(time.RFC3339, qr.ExpirationDate)
	if exp.IsZero() {
		exp = time.Now().Add(2 * time.Hour)
	}
	_, err = s.DB.Exec(
		`INSERT INTO payments (order_id, provider, provider_payment_id, method, amount_cents, status, pix_qr_code, pix_payload, pix_expires_at)
		 VALUES ($1, 'asaas', $2, 'PIX', $3, $4, $5, $6, $7)`,
		o.ID, pay.ID, o.TotalCents, pay.Status, qr.EncodedImage, qr.Payload, exp,
	)
	if err != nil {
		return nil, err
	}
	return &Payment{
		ProviderPaymentID: pay.ID,
		Status:            pay.Status,
		AmountCents:       o.TotalCents,
		PixQRCodeBase64:   qr.EncodedImage,
		PixPayload:        qr.Payload,
		PixExpiresAt:      exp,
	}, nil
}

// Get carrega um pedido completo.
func (s *Service) Get(id string) (*Order, error) {
	o := Order{}
	var custID sql.NullString
	err := s.DB.QueryRow(`
		SELECT id, customer_id, customer_name, customer_email, customer_phone,
		       cep, street, COALESCE(number,''), COALESCE(complement,''), COALESCE(neighborhood,''), city, state,
		       COALESCE(lat,0), COALESCE(lng,0),
		       subtotal_cents, shipping_cents, total_cents, status, payment_status,
		       COALESCE(lalamove_quotation_id, ''), COALESCE(lalamove_order_id, ''),
		       COALESCE(notes, ''), created_at, updated_at
		FROM orders WHERE id = $1`, id,
	).Scan(
		&o.ID, &custID, &o.CustomerName, &o.CustomerEmail, &o.CustomerPhone,
		&o.Address.CEP, &o.Address.Street, &o.Address.Number, &o.Address.Complement, &o.Address.Neighborhood, &o.Address.City, &o.Address.State,
		&o.Address.Lat, &o.Address.Lng,
		&o.SubtotalCents, &o.ShippingCents, &o.TotalCents, &o.Status, &o.PaymentStatus,
		&o.LalamoveQuotationID, &o.LalamoveOrderID, &o.Notes, &o.CreatedAt, &o.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if custID.Valid {
		v := custID.String
		o.CustomerID = &v
	}
	// items
	rows, err := s.DB.Query(
		`SELECT COALESCE(product_id::text,''), product_name, quantity, COALESCE(unit_cents,0), under_consult
		 FROM order_items WHERE order_id = $1`, o.ID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	o.Items = []Item{}
	for rows.Next() {
		var it Item
		var unit int
		if err := rows.Scan(&it.ProductID, &it.Name, &it.Quantity, &unit, &it.UnderConsult); err != nil {
			return nil, err
		}
		if !it.UnderConsult {
			u := unit
			it.UnitCents = &u
		}
		o.Items = append(o.Items, it)
	}
	// payment
	pay, err := s.latestPayment(o.ID)
	if err == nil {
		o.Payment = pay
	}
	return &o, nil
}

func (s *Service) latestPayment(orderID string) (*Payment, error) {
	var p Payment
	var paid sql.NullTime
	err := s.DB.QueryRow(
		`SELECT COALESCE(provider_payment_id,''), status, amount_cents, COALESCE(pix_qr_code,''), COALESCE(pix_payload,''), COALESCE(pix_expires_at, now()), paid_at
		 FROM payments WHERE order_id = $1 ORDER BY created_at DESC LIMIT 1`, orderID,
	).Scan(&p.ProviderPaymentID, &p.Status, &p.AmountCents, &p.PixQRCodeBase64, &p.PixPayload, &p.PixExpiresAt, &paid)
	if err != nil {
		return nil, err
	}
	if paid.Valid {
		t := paid.Time
		p.PaidAt = &t
	}
	return &p, nil
}

// validatedShipping devolve o frete real: se quotation_id existir, consulta o
// Lalamove e usa o preço oficial; caso contrário, aceita o valor informado
// (mínimo 0) e registra warning para revisão manual.
func (s *Service) validatedShipping(quotationID string, clientShipping int) (int, error) {
	if clientShipping < 0 {
		return 0, errors.New("valor de frete inválido")
	}
	if quotationID == "" || s.Lala == nil {
		return clientShipping, nil
	}
	q, err := s.Lala.GetQuotation(quotationID)
	if err != nil {
		slog.Warn("não foi possível revalidar frete; usando valor informado pelo cliente",
			"quotation_id", quotationID, "error", err)
		return clientShipping, nil
	}
	totalStr := q.Data.PriceBreakdown.Total
	val, err := strconv.ParseFloat(strings.TrimSpace(totalStr), 64)
	if err != nil || val < 0 {
		slog.Warn("preço de frete inválido na cotação Lalamove; usando valor do cliente",
			"quotation_id", quotationID, "raw", totalStr)
		return clientShipping, nil
	}
	return int(math.Round(val * 100)), nil
}

// List devolve pedidos (admin).
func (s *Service) List(filterCustomerID string) ([]Order, error) {
	q := `SELECT id, COALESCE(customer_id::text, ''), customer_name, customer_email, customer_phone,
	             cep, city, state, total_cents, status, payment_status,
	             COALESCE(lalamove_order_id, ''), created_at, updated_at
	      FROM orders`
	args := []any{}
	if filterCustomerID != "" {
		q += " WHERE customer_id = $1"
		args = append(args, filterCustomerID)
	}
	q += " ORDER BY created_at DESC LIMIT 200"
	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Order{}
	for rows.Next() {
		var o Order
		var custID string
		if err := rows.Scan(&o.ID, &custID, &o.CustomerName, &o.CustomerEmail, &o.CustomerPhone,
			&o.Address.CEP, &o.Address.City, &o.Address.State,
			&o.TotalCents, &o.Status, &o.PaymentStatus,
			&o.LalamoveOrderID, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		if custID != "" {
			c := custID
			o.CustomerID = &c
		}
		out = append(out, o)
	}
	return out, nil
}

// MarkPaid atualiza status para "paid" e dispara notificação WS.
func (s *Service) MarkPaid(providerPaymentID, orderID, status string, raw json.RawMessage) error {
	if orderID == "" && providerPaymentID != "" {
		_ = s.DB.QueryRow(`SELECT order_id FROM payments WHERE provider_payment_id = $1 LIMIT 1`, providerPaymentID).Scan(&orderID)
	}
	if orderID == "" {
		return errors.New("orderID não encontrado")
	}
	_, err := s.DB.Exec(
		`UPDATE payments SET status = $1, paid_at = COALESCE(paid_at, now()), raw_payload = $2::jsonb, updated_at = now()
		 WHERE provider_payment_id = $3`,
		status, string(raw), providerPaymentID,
	)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(
		`UPDATE orders SET payment_status = 'paid', status = 'paid', updated_at = now() WHERE id = $1`, orderID,
	)
	if err != nil {
		return err
	}
	if s.Notifier != nil {
		s.Notifier.Push(orderID, notifier.Event{Type: "payment_confirmed", OrderID: orderID, Message: "Pagamento confirmado!"})
	}
	return nil
}

// MarkReady marca o pedido como "pronto para entrega" e notifica o cliente.
func (s *Service) MarkReady(orderID, message string) error {
	_, err := s.DB.Exec(
		`UPDATE orders SET status = 'ready', updated_at = now() WHERE id = $1`, orderID,
	)
	if err != nil {
		return err
	}
	if s.Notifier != nil {
		msg := message
		if msg == "" {
			msg = "Seu pedido está pronto! Entraremos em contato para combinar a entrega."
		}
		s.Notifier.Push(orderID, notifier.Event{Type: "ready_for_delivery", OrderID: orderID, Message: msg})
	}
	return nil
}

// PollPayment chama o Asaas para sincronizar status (fallback do webhook).
func (s *Service) PollPayment(orderID string) (string, error) {
	pay, err := s.latestPayment(orderID)
	if err != nil {
		return "", err
	}
	if pay.ProviderPaymentID == "" || s.Asaas == nil {
		return pay.Status, nil
	}
	live, err := s.Asaas.GetPayment(pay.ProviderPaymentID)
	if err != nil {
		return pay.Status, err
	}
	if payments.IsPaid(live.Status) && (pay.PaidAt == nil) {
		raw, _ := json.Marshal(live)
		_ = s.MarkPaid(pay.ProviderPaymentID, orderID, live.Status, raw)
		return "paid", nil
	}
	return live.Status, nil
}

// ===== HTTP Handlers =====

// CreateHandler cria um pedido (cliente autenticado opcional).
func (s *Service) CreateHandler(w http.ResponseWriter, r *http.Request) {
	var req CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}
	if req.CustomerEmail == "" || req.CustomerName == "" || req.CustomerPhone == "" {
		writeJSON(w, 400, map[string]string{"error": "customer_name, customer_email and customer_phone required"})
		return
	}
	var customerID *string
	if c, ok := auth.FromContext(r.Context()); ok {
		id := c.Sub
		customerID = &id
	}
	o, err := s.Create(req, customerID)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 201, o)
}

// GetHandler retorna um pedido (autenticado para o dono ou admin).
func (s *Service) GetHandler(id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		o, err := s.Get(id)
		if err != nil {
			writeJSON(w, 404, map[string]string{"error": "order not found"})
			return
		}
		// Pedido com dono: exige autenticação como esse dono ou admin.
		// Pedido anônimo (customer_id = nil): qualquer um com o UUID pode ver.
		if o.CustomerID != nil {
			c, ok := auth.FromContext(r.Context())
			if !ok {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
				return
			}
			if c.Role != "admin" && *o.CustomerID != c.Sub {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
				return
			}
		}
		writeJSON(w, 200, o)
	}
}

// PaymentStatusHandler retorna o status do pagamento (para polling do frontend).
func (s *Service) PaymentStatusHandler(orderID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status, err := s.PollPayment(orderID)
		if err != nil {
			writeJSON(w, 200, map[string]any{"status": status, "warning": err.Error()})
			return
		}
		paid := payments.IsPaid(status) || status == "paid"
		writeJSON(w, 200, map[string]any{"status": status, "paid": paid})
	}
}

// AdminListHandler lista pedidos (admin).
func (s *Service) AdminListHandler(w http.ResponseWriter, r *http.Request) {
	list, err := s.List("")
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, list)
}

// MyOrdersHandler lista pedidos do cliente autenticado.
func (s *Service) MyOrdersHandler(w http.ResponseWriter, r *http.Request) {
	c, ok := auth.FromContext(r.Context())
	if !ok {
		writeJSON(w, 401, map[string]string{"error": "unauthorized"})
		return
	}
	list, err := s.List(c.Sub)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, list)
}

// MarkReadyHandler marca pedido como pronto (admin).
func (s *Service) MarkReadyHandler(orderID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Message string `json:"message"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if err := s.MarkReady(orderID, body.Message); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]string{"status": "ready"})
	}
}

// AsaasWebhookHandler recebe eventos do Asaas (PAYMENT_RECEIVED etc.).
func (s *Service) AsaasWebhookHandler(w http.ResponseWriter, r *http.Request) {
	// ASAAS_WEBHOOK_TOKEN é obrigatório; sem ele o endpoint fica desabilitado.
	if s.WebhookSecret == "" {
		slog.Error("webhook Asaas recebido mas ASAAS_WEBHOOK_TOKEN não está configurado — requisição rejeitada")
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "webhook not configured"})
		return
	}
	got := r.Header.Get("asaas-access-token")
	if got != s.WebhookSecret {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid webhook token"})
		return
	}
	body := struct {
		Event   string           `json:"event"`
		Payment payments.Payment `json:"payment"`
	}{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}
	raw, _ := json.Marshal(body)
	if payments.IsPaidEvent(body.Event) {
		if err := s.MarkPaid(body.Payment.ID, body.Payment.ExternalRef, body.Payment.Status, raw); err != nil {
			slog.Error("erro ao marcar pedido como pago via webhook", "payment_id", body.Payment.ID, "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error processing payment"})
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
