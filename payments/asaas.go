// Package payments implementa a integração com Asaas (Pix QR + webhook).
package payments

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// AsaasClient é o cliente HTTP do Asaas.
type AsaasClient struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// NewAsaasClient retorna um cliente. Se sandbox=true usa endpoint de sandbox.
func NewAsaasClient(apiKey string, sandbox bool) *AsaasClient {
	base := "https://api.asaas.com/v3"
	if sandbox {
		base = "https://sandbox.asaas.com/api/v3"
	}
	return &AsaasClient{
		baseURL: base,
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 20 * time.Second},
	}
}

// AsaasCustomer é o cliente cadastrado no Asaas.
type AsaasCustomer struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Email      string `json:"email,omitempty"`
	Phone      string `json:"phone,omitempty"`
	MobilePhone string `json:"mobilePhone,omitempty"`
	CpfCnpj    string `json:"cpfCnpj,omitempty"`
}

// PaymentRequest cria uma cobrança Pix.
type PaymentRequest struct {
	Customer    string  `json:"customer"`
	BillingType string  `json:"billingType"` // "PIX"
	Value       float64 `json:"value"`
	DueDate     string  `json:"dueDate"`               // YYYY-MM-DD
	Description string  `json:"description,omitempty"`
	ExternalRef string  `json:"externalReference,omitempty"`
}

// Payment é a cobrança devolvida pelo Asaas.
type Payment struct {
	ID          string  `json:"id"`
	Customer    string  `json:"customer"`
	Status      string  `json:"status"`
	Value       float64 `json:"value"`
	BillingType string  `json:"billingType"`
	DueDate     string  `json:"dueDate"`
	InvoiceURL  string  `json:"invoiceUrl"`
	ExternalRef string  `json:"externalReference"`
}

// PixQRCode é o payload do endpoint /payments/{id}/pixQrCode.
type PixQRCode struct {
	EncodedImage   string `json:"encodedImage"`   // base64 PNG
	Payload        string `json:"payload"`        // copia-e-cola
	ExpirationDate string `json:"expirationDate"` // ISO datetime
}

// WebhookEvent é o payload de webhook do Asaas.
type WebhookEvent struct {
	Event   string  `json:"event"` // PAYMENT_RECEIVED, PAYMENT_CONFIRMED, ...
	Payment Payment `json:"payment"`
}

// CreateCustomer cria/atualiza um cliente no Asaas.
func (c *AsaasClient) CreateCustomer(name, email, phone, cpfCnpj string) (*AsaasCustomer, error) {
	body := map[string]string{
		"name":  name,
		"email": email,
	}
	if phone != "" {
		body["mobilePhone"] = phone
	}
	if cpfCnpj != "" {
		body["cpfCnpj"] = cpfCnpj
	}
	var out AsaasCustomer
	if err := c.do("POST", "/customers", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreatePixPayment cria uma cobrança Pix.
func (c *AsaasClient) CreatePixPayment(req PaymentRequest) (*Payment, error) {
	if req.BillingType == "" {
		req.BillingType = "PIX"
	}
	if req.DueDate == "" {
		req.DueDate = time.Now().Add(24 * time.Hour).Format("2006-01-02")
	}
	var out Payment
	if err := c.do("POST", "/payments", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetPixQRCode busca QR Code de uma cobrança Pix.
func (c *AsaasClient) GetPixQRCode(paymentID string) (*PixQRCode, error) {
	var out PixQRCode
	if err := c.do("GET", "/payments/"+paymentID+"/pixQrCode", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetPayment retorna o status atual de uma cobrança.
func (c *AsaasClient) GetPayment(paymentID string) (*Payment, error) {
	var out Payment
	if err := c.do("GET", "/payments/"+paymentID, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// IsPaid retorna true se o status indica pagamento confirmado.
func IsPaid(status string) bool {
	s := strings.ToUpper(status)
	return s == "RECEIVED" || s == "CONFIRMED" || s == "RECEIVED_IN_CASH"
}

// IsPaidEvent retorna true se o evento do webhook é confirmação de pagamento.
func IsPaidEvent(event string) bool {
	e := strings.ToUpper(event)
	return e == "PAYMENT_CONFIRMED" || e == "PAYMENT_RECEIVED" || e == "PAYMENT_RECEIVED_IN_CASH"
}

func (c *AsaasClient) do(method, path string, body any, out any) error {
	if c.apiKey == "" {
		return errors.New("ASAAS_API_KEY não configurada")
	}
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, c.baseURL+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("access_token", c.apiKey)
	req.Header.Set("User-Agent", "florentina-api/1.0")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("asaas %s %s → %d: %s", method, path, resp.StatusCode, string(raw))
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}
