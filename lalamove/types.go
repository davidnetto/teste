package lalamove

// ---------- Shared ----------

type Coordinates struct {
	Lat string `json:"lat"`
	Lng string `json:"lng"`
}

type PriceBreakdown struct {
	Base     string `json:"base"`
	Currency string `json:"currency"`
	Total    string `json:"total"`
}

// ---------- Quotation ----------

type Stop struct {
	Coordinates Coordinates `json:"coordinates"`
	Address     string      `json:"address"`
}

type Item struct {
	Quantity    int    `json:"quantity"`
	Weight      string `json:"weight"`
	Categories  []string `json:"categories,omitempty"`
	HandlingInstructions []string `json:"handlingInstructions,omitempty"`
}

type QuotationRequest struct {
	Data struct {
		ServiceType      string   `json:"serviceType"`
		Stops            []Stop   `json:"stops"`
		Language         string   `json:"language"`
		ScheduleAt       string   `json:"scheduleAt,omitempty"`
		SpecialRequests  []string `json:"specialRequests,omitempty"`
		IsRouteOptimized bool     `json:"isRouteOptimized,omitempty"`
		Item             *Item    `json:"item,omitempty"`
	} `json:"data"`
}

type QuotationStop struct {
	StopID      string      `json:"stopId"`
	Coordinates Coordinates `json:"coordinates"`
	Address     string      `json:"address"`
}

type QuotationResponse struct {
	Data struct {
		QuotationID      string          `json:"quotationId"`
		ScheduleAt       string          `json:"scheduleAt,omitempty"`
		ExpiresAt        string          `json:"expiresAt"`
		ServiceType      string          `json:"serviceType"`
		SpecialRequests  []string        `json:"specialRequests,omitempty"`
		Stops            []QuotationStop `json:"stops"`
		PriceBreakdown   PriceBreakdown  `json:"priceBreakdown"`
		IsRouteOptimized bool            `json:"isRouteOptimized,omitempty"`
		Distance         struct {
			Value string `json:"value"`
			Unit  string `json:"unit"`
		} `json:"distance"`
	} `json:"data"`
}

// ---------- Order ----------

type ContactInfo struct {
	StopID  string `json:"stopId"`
	Name    string `json:"name"`
	Phone   string `json:"phone"`
	Remarks string `json:"remarks,omitempty"`
}

type OrderRequest struct {
	Data struct {
		QuotationID  string        `json:"quotationId"`
		Sender       ContactInfo   `json:"sender"`
		Recipients   []ContactInfo `json:"recipients"`
		IsPODEnabled bool          `json:"isPODEnabled,omitempty"`
		Metadata     []Metadata    `json:"metadata,omitempty"`
	} `json:"data"`
}

type Metadata struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type OrderStop struct {
	StopID      string      `json:"stopId"`
	Coordinates Coordinates `json:"coordinates"`
	Address     string      `json:"address"`
	Name        string      `json:"name,omitempty"`
	Phone       string      `json:"phone,omitempty"`
	Remarks     string      `json:"remarks,omitempty"`
	POD         *POD        `json:"POD,omitempty"`
}

type POD struct {
	Status string `json:"status,omitempty"`
	Image  string `json:"image,omitempty"`
}

type OrderResponse struct {
	Data struct {
		OrderID        string         `json:"orderId"`
		QuotationID    string         `json:"quotationId"`
		Status         string         `json:"status"`
		DriverID       string         `json:"driverId,omitempty"`
		ShareLink      string         `json:"shareLink,omitempty"`
		PriceBreakdown PriceBreakdown `json:"priceBreakdown"`
		Stops          []OrderStop    `json:"stops,omitempty"`
		Metadata       []Metadata     `json:"metadata,omitempty"`
		Distance       struct {
			Value string `json:"value"`
			Unit  string `json:"unit"`
		} `json:"distance,omitempty"`
	} `json:"data"`
}

// ---------- Edit Order ----------

type EditOrderStop struct {
	Coordinates Coordinates `json:"coordinates"`
	Address     string      `json:"address"`
	Name        string      `json:"name,omitempty"`
	Phone       string      `json:"phone,omitempty"`
	Remarks     string      `json:"remarks,omitempty"`
}

type EditOrderRequest struct {
	Data struct {
		Stops []EditOrderStop `json:"stops"`
	} `json:"data"`
}

// ---------- Driver ----------

type DriverLocation struct {
	Lat       string `json:"lat"`
	Lng       string `json:"lng"`
	UpdatedAt string `json:"updatedAt"`
}

type DriverResponse struct {
	Data struct {
		DriverID    string         `json:"driverId"`
		Name        string         `json:"name"`
		Phone       string         `json:"phone"`
		PlateNumber string         `json:"plateNumber"`
		Photo       string         `json:"photo"`
		Coordinates DriverLocation `json:"coordinates"`
	} `json:"data"`
}

type ChangeDriverRequest struct {
	Data struct {
		Reason string `json:"reason"`
	} `json:"data"`
}

// ---------- Priority Fee ----------

type PriorityFeeRequest struct {
	Data struct {
		PriorityFee string `json:"priorityFee"`
	} `json:"data"`
}

// ---------- Cities ----------

type SpecialRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type Service struct {
	Key             string           `json:"key"`
	Description     string           `json:"description"`
	SpecialRequests []SpecialRequest `json:"specialRequests,omitempty"`
}

type City struct {
	Locode   string    `json:"locode"`
	Name     string    `json:"name"`
	Services []Service `json:"services"`
}

type CitiesResponse struct {
	Data []City `json:"data"`
}

// ---------- Webhook ----------

type WebhookRequest struct {
	Data struct {
		URL string `json:"url"`
	} `json:"data"`
}

type WebhookResponse struct {
	Data struct {
		URL string `json:"url"`
	} `json:"data"`
}
