package lalamove

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
)

const (
	sandboxBaseURL    = "https://rest.sandbox.lalamove.com"
	productionBaseURL = "https://rest.lalamove.com"
)

type Client struct {
	apiKey    string
	apiSecret string
	market    string
	baseURL   string
	http      *http.Client
}

// NewClient cria um cliente Lalamove. Se baseURL estiver vazio, usa o ambiente sandbox.
func NewClient(apiKey, apiSecret, market, baseURL string) *Client {
	if baseURL == "" {
		baseURL = sandboxBaseURL
	}
	return &Client{
		apiKey:    apiKey,
		apiSecret: apiSecret,
		market:    market,
		baseURL:   baseURL,
		http:      &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) sign(timestamp, method, path, body string) string {
	payload := fmt.Sprintf("%s\r\n%s\r\n%s\r\n\r\n%s", timestamp, method, path, body)
	mac := hmac.New(sha256.New, []byte(c.apiSecret))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func (c *Client) do(method, path string, reqBody any, out any) error {
	var bodyBytes []byte
	if reqBody != nil {
		var err error
		bodyBytes, err = json.Marshal(reqBody)
		if err != nil {
			return err
		}
	}

	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	sig := c.sign(ts, method, path, string(bodyBytes))

	req, err := http.NewRequest(method, c.baseURL+path, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("hmac %s:%s:%s", c.apiKey, ts, sig))
	req.Header.Set("Market", c.market)
	req.Header.Set("Request-ID", uuid.NewString())

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("lalamove %s %s → %d: %s", method, path, resp.StatusCode, raw)
	}

	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}
