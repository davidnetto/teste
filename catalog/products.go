package catalog

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"

	"userapi/auth"
)

// Product representa um produto vendável.
type Product struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description"`
	PriceCents  *int      `json:"price_cents"` // nil = sob consulta
	Images      []string  `json:"images"`
	Stock       int       `json:"stock"`
	WeightKg    *float64  `json:"weight_kg,omitempty"`
	IsActive    bool      `json:"is_active"`
	CategoryIDs []string  `json:"category_ids"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ProductRequest é o body para criar/atualizar produto.
type ProductRequest struct {
	Name         string   `json:"name"`
	Slug         string   `json:"slug"`
	Description  string   `json:"description"`
	PriceCents   *int     `json:"price_cents"`   // ausente ou nil = sob consulta
	UnderConsult bool     `json:"under_consult"` // alternativa explícita
	Images       []string `json:"images"`
	Stock        int      `json:"stock"`
	WeightKg     *float64 `json:"weight_kg"`
	IsActive     *bool    `json:"is_active"`
	CategoryIDs  []string `json:"category_ids"`
}

// ProductStore acessa produtos.
type ProductStore struct{ DB *sql.DB }

// NewProductStore cria um store.
func NewProductStore(db *sql.DB) *ProductStore { return &ProductStore{DB: db} }

// Create insere produto + associa categorias.
func (s *ProductStore) Create(req ProductRequest) (Product, error) {
	if strings.TrimSpace(req.Name) == "" {
		return Product{}, errors.New("name is required")
	}
	if req.Slug == "" {
		req.Slug = slugify(req.Name)
	}
	if req.UnderConsult {
		req.PriceCents = nil
	}
	if req.Images == nil {
		req.Images = []string{}
	}
	imagesJSON, _ := json.Marshal(req.Images)

	tx, err := s.DB.Begin()
	if err != nil {
		return Product{}, err
	}
	defer tx.Rollback()

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	var p Product
	var price sql.NullInt64
	var weight sql.NullFloat64
	if req.PriceCents != nil {
		price.Valid = true
		price.Int64 = int64(*req.PriceCents)
	}
	if req.WeightKg != nil {
		weight.Valid = true
		weight.Float64 = *req.WeightKg
	}

	err = tx.QueryRow(
		`INSERT INTO products (name, slug, description, price_cents, images, stock, weight_kg, is_active)
		 VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7, $8)
		 RETURNING id, name, slug, description, price_cents, images, stock, weight_kg, is_active, created_at, updated_at`,
		req.Name, req.Slug, req.Description, price, string(imagesJSON), req.Stock, weight, isActive,
	).Scan(&p.ID, &p.Name, &p.Slug, &p.Description, &price, &imagesJSON, &p.Stock, &weight, &p.IsActive, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return Product{}, err
	}
	if price.Valid {
		v := int(price.Int64)
		p.PriceCents = &v
	}
	if weight.Valid {
		v := weight.Float64
		p.WeightKg = &v
	}
	_ = json.Unmarshal(imagesJSON, &p.Images)

	if err := upsertProductCategories(tx, p.ID, req.CategoryIDs); err != nil {
		return Product{}, err
	}
	if err := tx.Commit(); err != nil {
		return Product{}, err
	}
	p.CategoryIDs = req.CategoryIDs
	if p.CategoryIDs == nil {
		p.CategoryIDs = []string{}
	}
	return p, nil
}

// Update edita um produto e suas categorias.
func (s *ProductStore) Update(id string, req ProductRequest) (Product, bool, error) {
	if strings.TrimSpace(req.Name) == "" {
		return Product{}, false, errors.New("name is required")
	}
	if req.Slug == "" {
		req.Slug = slugify(req.Name)
	}
	if req.UnderConsult {
		req.PriceCents = nil
	}
	if req.Images == nil {
		req.Images = []string{}
	}
	imagesJSON, _ := json.Marshal(req.Images)

	tx, err := s.DB.Begin()
	if err != nil {
		return Product{}, false, err
	}
	defer tx.Rollback()

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	var price sql.NullInt64
	var weight sql.NullFloat64
	if req.PriceCents != nil {
		price.Valid = true
		price.Int64 = int64(*req.PriceCents)
	}
	if req.WeightKg != nil {
		weight.Valid = true
		weight.Float64 = *req.WeightKg
	}

	var p Product
	err = tx.QueryRow(
		`UPDATE products SET
			name = $1, slug = $2, description = $3,
			price_cents = $4, images = $5::jsonb,
			stock = $6, weight_kg = $7, is_active = $8,
			updated_at = now()
		 WHERE id = $9
		 RETURNING id, name, slug, description, price_cents, images, stock, weight_kg, is_active, created_at, updated_at`,
		req.Name, req.Slug, req.Description, price, string(imagesJSON), req.Stock, weight, isActive, id,
	).Scan(&p.ID, &p.Name, &p.Slug, &p.Description, &price, &imagesJSON, &p.Stock, &weight, &p.IsActive, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Product{}, false, nil
	}
	if err != nil {
		return Product{}, false, err
	}
	if price.Valid {
		v := int(price.Int64)
		p.PriceCents = &v
	}
	if weight.Valid {
		v := weight.Float64
		p.WeightKg = &v
	}
	_ = json.Unmarshal(imagesJSON, &p.Images)

	if _, err := tx.Exec(`DELETE FROM product_categories WHERE product_id = $1`, p.ID); err != nil {
		return Product{}, false, err
	}
	if err := upsertProductCategories(tx, p.ID, req.CategoryIDs); err != nil {
		return Product{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return Product{}, false, err
	}
	p.CategoryIDs = req.CategoryIDs
	if p.CategoryIDs == nil {
		p.CategoryIDs = []string{}
	}
	return p, true, nil
}

// Delete remove um produto.
func (s *ProductStore) Delete(id string) (bool, error) {
	res, err := s.DB.Exec(`DELETE FROM products WHERE id = $1`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// Get retorna um produto pelo ID ou slug.
func (s *ProductStore) Get(idOrSlug string) (Product, bool, error) {
	var p Product
	var price sql.NullInt64
	var weight sql.NullFloat64
	var imagesJSON []byte
	q := `SELECT id, name, slug, description, price_cents, images, stock, weight_kg, is_active, created_at, updated_at
	      FROM products 
	      WHERE id::text = $1 
	         OR slug = $1 
	         OR slug LIKE $1 || '-%'
	         OR slug LIKE '%-' || $1`
	err := s.DB.QueryRow(q, idOrSlug).Scan(&p.ID, &p.Name, &p.Slug, &p.Description, &price, &imagesJSON, &p.Stock, &weight, &p.IsActive, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Product{}, false, nil
	}
	if err != nil {
		return Product{}, false, err
	}
	if price.Valid {
		v := int(price.Int64)
		p.PriceCents = &v
	}
	if weight.Valid {
		v := weight.Float64
		p.WeightKg = &v
	}
	_ = json.Unmarshal(imagesJSON, &p.Images)
	cats, err := s.categoriesOf(p.ID)
	if err != nil {
		return Product{}, false, err
	}
	p.CategoryIDs = cats
	return p, true, nil
}

func (s *ProductStore) categoriesOf(productID string) ([]string, error) {
	rows, err := s.DB.Query(`SELECT category_id::text FROM product_categories WHERE product_id = $1`, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ListFilter define filtros de listagem de produto.
type ListFilter struct {
	Search        string
	CategoryIDs   []string
	IncludeInactive bool
	Limit         int
	Offset        int
}

// List devolve produtos opcionalmente filtrados por categoria e busca.
func (s *ProductStore) List(f ListFilter) ([]Product, error) {
	q := strings.Builder{}
	q.WriteString(`SELECT DISTINCT p.id, p.name, p.slug, p.description, p.price_cents, p.images, p.stock, p.weight_kg, p.is_active, p.created_at, p.updated_at
	               FROM products p`)
	args := []any{}
	conds := []string{}
	if len(f.CategoryIDs) > 0 {
		q.WriteString(` JOIN product_categories pc ON pc.product_id = p.id`)
		args = append(args, pq.Array(f.CategoryIDs))
		conds = append(conds, "pc.category_id = ANY($"+itoa(len(args))+"::uuid[])")
	}
	if !f.IncludeInactive {
		conds = append(conds, "p.is_active = TRUE")
	}
	if strings.TrimSpace(f.Search) != "" {
		args = append(args, "%"+strings.ToLower(f.Search)+"%")
		conds = append(conds, "(LOWER(p.name) LIKE $"+itoa(len(args))+" OR LOWER(p.description) LIKE $"+itoa(len(args))+")")
	}
	if len(conds) > 0 {
		q.WriteString(" WHERE " + strings.Join(conds, " AND "))
	}
	q.WriteString(" ORDER BY p.created_at DESC")
	if f.Limit > 0 {
		q.WriteString(" LIMIT " + itoa64(int64(f.Limit)))
	}
	if f.Offset > 0 {
		q.WriteString(" OFFSET " + itoa64(int64(f.Offset)))
	}

	rows, err := s.DB.Query(q.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Product{}
	ids := []string{}
	for rows.Next() {
		var p Product
		var price sql.NullInt64
		var weight sql.NullFloat64
		var imagesJSON []byte
		if err := rows.Scan(&p.ID, &p.Name, &p.Slug, &p.Description, &price, &imagesJSON, &p.Stock, &weight, &p.IsActive, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		if price.Valid {
			v := int(price.Int64)
			p.PriceCents = &v
		}
		if weight.Valid {
			v := weight.Float64
			p.WeightKg = &v
		}
		_ = json.Unmarshal(imagesJSON, &p.Images)
		p.CategoryIDs = []string{}
		out = append(out, p)
		ids = append(ids, p.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(ids) == 0 {
		return out, nil
	}

	// Carrega categorias em batch.
	rows2, err := s.DB.Query(
		`SELECT product_id::text, category_id::text FROM product_categories WHERE product_id = ANY($1::uuid[])`,
		pq.Array(ids),
	)
	if err != nil {
		return nil, err
	}
	defer rows2.Close()
	byID := map[string]int{}
	for i, p := range out {
		byID[p.ID] = i
	}
	for rows2.Next() {
		var pid, cid string
		if err := rows2.Scan(&pid, &cid); err != nil {
			return nil, err
		}
		if i, ok := byID[pid]; ok {
			out[i].CategoryIDs = append(out[i].CategoryIDs, cid)
		}
	}
	return out, rows2.Err()
}

func upsertProductCategories(tx *sql.Tx, productID string, catIDs []string) error {
	for _, cid := range catIDs {
		if cid == "" {
			continue
		}
		_, err := tx.Exec(
			`INSERT INTO product_categories (product_id, category_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			productID, cid,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// ===== HTTP handlers =====

// ListHandler godoc
// @Summary Lista produtos
// @Tags    catalog
// @Produce json
// @Param   q          query string false "busca livre"
// @Param   category   query string false "ID de categoria (inclui descendentes)"
// @Param   limit      query int    false "limite"
// @Success 200 {array} Product
// @Router  /api/products [get]
func (s *ProductStore) ListHandler(catStore *CategoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f := ListFilter{
			Search: r.URL.Query().Get("q"),
		}
		if l := r.URL.Query().Get("limit"); l != "" {
			n, _ := strconv.Atoi(l)
			f.Limit = n
		} else {
			f.Limit = 20 // default limit
		}
		if o := r.URL.Query().Get("offset"); o != "" {
			n, _ := strconv.Atoi(o)
			f.Offset = n
		}
		if cat := r.URL.Query().Get("category"); cat != "" {
			descendants, err := catStore.DescendantsOf(cat)
			if err == nil && len(descendants) > 0 {
				f.CategoryIDs = descendants
			} else {
				f.CategoryIDs = []string{cat}
			}
		}
		if r.URL.Query().Get("include_inactive") == "1" {
			c, ok := auth.FromContext(r.Context())
			if ok && c.Role == "admin" {
				f.IncludeInactive = true
			}
		}
		list, err := s.List(f)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, list)
	}
}

// GetHandler retorna um produto por id/slug.
func (s *ProductStore) GetHandler(idOrSlug string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok, err := s.Get(idOrSlug)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		if !ok {
			writeJSON(w, 404, map[string]string{"error": "product not found"})
			return
		}
		writeJSON(w, 200, p)
	}
}

// CreateHandler cria produto (admin).
func (s *ProductStore) CreateHandler(w http.ResponseWriter, r *http.Request) {
	var req ProductRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}
	p, err := s.Create(req)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 201, p)
}

// UpdateHandler atualiza produto (admin).
func (s *ProductStore) UpdateHandler(id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ProductRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid body"})
			return
		}
		p, ok, err := s.Update(id, req)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		if !ok {
			writeJSON(w, 404, map[string]string{"error": "product not found"})
			return
		}
		writeJSON(w, 200, p)
	}
}

// DeleteHandler remove produto (admin).
func (s *ProductStore) DeleteHandler(id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ok, err := s.Delete(id)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		if !ok {
			writeJSON(w, 404, map[string]string{"error": "product not found"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func itoa(n int) string   { return strconv.Itoa(n) }
func itoa64(n int64) string { return strconv.FormatInt(n, 10) }
