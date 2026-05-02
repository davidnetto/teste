package lalamove

// ---------- Quotations ----------

func (c *Client) CreateQuotation(req QuotationRequest) (*QuotationResponse, error) {
	var resp QuotationResponse
	return &resp, c.do("POST", "/v3/quotations", req, &resp)
}

func (c *Client) GetQuotation(quotationID string) (*QuotationResponse, error) {
	var resp QuotationResponse
	return &resp, c.do("GET", "/v3/quotations/"+quotationID, nil, &resp)
}

// ---------- Orders ----------

func (c *Client) PlaceOrder(req OrderRequest) (*OrderResponse, error) {
	var resp OrderResponse
	return &resp, c.do("POST", "/v3/orders", req, &resp)
}

func (c *Client) GetOrder(orderID string) (*OrderResponse, error) {
	var resp OrderResponse
	return &resp, c.do("GET", "/v3/orders/"+orderID, nil, &resp)
}

func (c *Client) EditOrder(orderID string, req EditOrderRequest) (*OrderResponse, error) {
	var resp OrderResponse
	return &resp, c.do("PATCH", "/v3/orders/"+orderID, req, &resp)
}

func (c *Client) CancelOrder(orderID string) error {
	return c.do("DELETE", "/v3/orders/"+orderID, nil, nil)
}

// ---------- Drivers ----------

func (c *Client) GetDriver(orderID, driverID string) (*DriverResponse, error) {
	var resp DriverResponse
	return &resp, c.do("GET", "/v3/orders/"+orderID+"/drivers/"+driverID, nil, &resp)
}

func (c *Client) ChangeDriver(orderID, driverID string, req ChangeDriverRequest) error {
	return c.do("DELETE", "/v3/orders/"+orderID+"/drivers/"+driverID, req, nil)
}

// ---------- Priority Fee ----------

func (c *Client) AddPriorityFee(orderID string, req PriorityFeeRequest) (*OrderResponse, error) {
	var resp OrderResponse
	return &resp, c.do("POST", "/v3/orders/"+orderID+"/priority-fee", req, &resp)
}

// ---------- Cities ----------

func (c *Client) GetCities() (*CitiesResponse, error) {
	var resp CitiesResponse
	return &resp, c.do("GET", "/v3/cities", nil, &resp)
}

// ---------- Webhook ----------

func (c *Client) SetWebhook(req WebhookRequest) (*WebhookResponse, error) {
	var resp WebhookResponse
	return &resp, c.do("PATCH", "/v3/webhook", req, &resp)
}
