package tracking

import (
	"encoding/json"
	"log"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"userapi/lalamove"
)

type RoutePoint struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type LocationUpdate struct {
	OrderID   string       `json:"orderId"`
	Status    string       `json:"status"`
	DriverID  string       `json:"driverId,omitempty"`
	Name      string       `json:"name,omitempty"`
	Phone     string       `json:"phone,omitempty"`
	Plate     string       `json:"plate,omitempty"`
	Photo     string       `json:"photo,omitempty"`
	Lat       string       `json:"lat,omitempty"`
	Lng       string       `json:"lng,omitempty"`
	UpdatedAt string       `json:"updatedAt,omitempty"`
	Error     string       `json:"error,omitempty"`
	Route     []RoutePoint `json:"route,omitempty"`
	Origin    *RoutePoint  `json:"origin,omitempty"`
	Dest      *RoutePoint  `json:"dest,omitempty"`
}

type client struct {
	conn      *websocket.Conn
	send      chan []byte
	closeOnce sync.Once
}

type orderTracker struct {
	clients map[*client]struct{}
	mu      sync.Mutex
	stop    chan struct{}
}

type Hub struct {
	mu       sync.Mutex
	trackers map[string]*orderTracker
	lala     *lalamove.Client
}

func NewHub(lala *lalamove.Client) *Hub {
	return &Hub{
		trackers: make(map[string]*orderTracker),
		lala:     lala,
	}
}

func (h *Hub) Subscribe(orderID string, conn *websocket.Conn) {
	h.mu.Lock()
	t, exists := h.trackers[orderID]
	if !exists {
		t = &orderTracker{
			clients: make(map[*client]struct{}),
			stop:    make(chan struct{}),
		}
		h.trackers[orderID] = t
		go h.poll(orderID, t)
	}
	c := &client{conn: conn, send: make(chan []byte, 16)}
	t.mu.Lock()
	t.clients[c] = struct{}{}
	t.mu.Unlock()
	h.mu.Unlock()

	go c.writePump()
	c.readPump(func() { h.unsubscribe(orderID, c) })
}

func (h *Hub) unsubscribe(orderID string, c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	t, ok := h.trackers[orderID]
	if !ok {
		return
	}
	t.mu.Lock()
	delete(t.clients, c)
	empty := len(t.clients) == 0
	t.mu.Unlock()
	c.closeOnce.Do(func() { close(c.send) })
	if empty {
		close(t.stop)
		delete(h.trackers, orderID)
	}
}

func (h *Hub) broadcast(t *orderTracker, update LocationUpdate) {
	data, err := json.Marshal(update)
	if err != nil {
		slog.Warn("tracking: falha ao serializar update", "error", err)
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for c := range t.clients {
		select {
		case c.send <- data:
		default:
			slog.Debug("tracking: canal cheio, mensagem descartada")
		}
	}
}

// Push envia um update direto para todos os clientes de um pedido (usado pelo webhook).
func (h *Hub) Push(orderID string, update LocationUpdate) {
	h.mu.Lock()
	t, ok := h.trackers[orderID]
	h.mu.Unlock()
	if ok {
		h.broadcast(t, update)
	}
}

func (h *Hub) poll(orderID string, t *orderTracker) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	fetch := func() {
		order, err := h.lala.GetOrder(orderID)
		if err != nil {
			h.broadcast(t, LocationUpdate{OrderID: orderID, Error: err.Error()})
			return
		}

		update := LocationUpdate{
			OrderID:  orderID,
			Status:   order.Data.Status,
			DriverID: order.Data.DriverID,
		}

		if order.Data.DriverID != "" {
			driver, err := h.lala.GetDriver(orderID, order.Data.DriverID)
			if err == nil {
				update.Name  = driver.Data.Name
				update.Phone = driver.Data.Phone
				update.Plate = driver.Data.PlateNumber
				update.Photo = driver.Data.Photo
				update.Lat   = driver.Data.Coordinates.Lat
				update.Lng   = driver.Data.Coordinates.Lng
				update.UpdatedAt = driver.Data.Coordinates.UpdatedAt
			}
		}

		h.broadcast(t, update)
	}

	fetch()
	for {
		select {
		case <-ticker.C:
			fetch()
		case <-t.stop:
			return
		}
	}
}

func (c *client) writePump() {
	defer c.conn.Close()
	for msg := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			log.Printf("ws write: %v", err)
			return
		}
	}
}

func (c *client) readPump(onClose func()) {
	defer onClose()
	c.conn.SetReadLimit(512)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			break
		}
	}
}
