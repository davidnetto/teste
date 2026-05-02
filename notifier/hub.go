// Package notifier é um hub WebSocket simples para enviar notificações por orderID.
// Usado para avisar o cliente sobre payment_confirmed e ready_for_delivery.
package notifier

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	maxPendingPerOrder = 50
	pendingTTL         = 2 * time.Hour
)

// Event é uma mensagem enviada ao cliente.
type Event struct {
	Type    string    `json:"type"`
	OrderID string    `json:"order_id"`
	Message string    `json:"message"`
	At      time.Time `json:"at"`
}

type pendingEntry struct {
	event Event
	at    time.Time
}

// Hub mantém clients agrupados por orderID.
type Hub struct {
	mu       sync.Mutex
	clients  map[string]map[*websocket.Conn]struct{} // orderID → conns
	pending  map[string][]pendingEntry                 // mensagens "guardadas" para quem ainda não conectou
	upgrader websocket.Upgrader
}

// New cria um Hub.
func New() *Hub {
	allowedOrigins := os.Getenv("CORS_ALLOWED_ORIGINS")
	h := &Hub{
		clients: make(map[string]map[*websocket.Conn]struct{}),
		pending: make(map[string][]pendingEntry),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				if allowedOrigins == "" || allowedOrigins == "*" {
					return true
				}
				origin := r.Header.Get("Origin")
				for _, o := range strings.Split(allowedOrigins, ",") {
					if strings.TrimSpace(o) == origin {
						return true
					}
				}
				return false
			},
		},
	}
	go h.cleanupLoop()
	return h
}

// cleanupLoop remove periodicamente mensagens pendentes expiradas.
func (h *Hub) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-pendingTTL)
		h.mu.Lock()
		for orderID, entries := range h.pending {
			var fresh []pendingEntry
			for _, e := range entries {
				if e.at.After(cutoff) {
					fresh = append(fresh, e)
				}
			}
			if len(fresh) == 0 {
				delete(h.pending, orderID)
			} else {
				h.pending[orderID] = fresh
			}
		}
		h.mu.Unlock()
	}
}

// Subscribe abre WS para o orderID.
// Autenticação: o chamador deve validar o acesso antes de invocar este método.
func (h *Hub) Subscribe(orderID string, w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	h.mu.Lock()
	if h.clients[orderID] == nil {
		h.clients[orderID] = make(map[*websocket.Conn]struct{})
	}
	h.clients[orderID][conn] = struct{}{}
	// drena eventos pendentes
	pending := h.pending[orderID]
	delete(h.pending, orderID)
	h.mu.Unlock()

	for _, entry := range pending {
		_ = conn.WriteJSON(entry.event)
	}

	go h.readLoop(orderID, conn)
}

func (h *Hub) readLoop(orderID string, conn *websocket.Conn) {
	defer func() {
		h.mu.Lock()
		if conns, ok := h.clients[orderID]; ok {
			delete(conns, conn)
			if len(conns) == 0 {
				delete(h.clients, orderID)
			}
		}
		h.mu.Unlock()
		_ = conn.Close()
	}()
	conn.SetReadLimit(512)
	conn.SetReadDeadline(time.Now().Add(120 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(120 * time.Second))
		return nil
	})
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

// Push envia um evento a todos os clients do orderID. Se ninguém estiver conectado,
// guarda na fila (máximo maxPendingPerOrder) para a próxima Subscribe.
func (h *Hub) Push(orderID string, ev Event) {
	if ev.At.IsZero() {
		ev.At = time.Now()
	}
	if ev.OrderID == "" {
		ev.OrderID = orderID
	}
	data, err := json.Marshal(ev)
	if err != nil {
		slog.Warn("notifier: falha ao serializar evento", "error", err)
		return
	}

	h.mu.Lock()
	conns := h.clients[orderID]
	if len(conns) == 0 {
		entries := h.pending[orderID]
		if len(entries) >= maxPendingPerOrder {
			entries = entries[1:] // descarta o mais antigo
		}
		h.pending[orderID] = append(entries, pendingEntry{event: ev, at: time.Now()})
		h.mu.Unlock()
		return
	}
	// Copia as conexões com lock para evitar race com readLoop.
	snapshot := make([]*websocket.Conn, 0, len(conns))
	for c := range conns {
		snapshot = append(snapshot, c)
	}
	h.mu.Unlock()

	for _, c := range snapshot {
		if err := c.WriteMessage(websocket.TextMessage, data); err != nil {
			slog.Debug("notifier: erro ao enviar mensagem WS", "error", err)
		}
	}
}
