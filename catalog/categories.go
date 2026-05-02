// Package catalog implementa categorias recursivas e produtos.
package catalog

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Category é um nó da árvore de categorias.
type Category struct {
	ID        string      `json:"id"`
	ParentID  *string     `json:"parent_id"`
	Name      string      `json:"name"`
	Slug      string      `json:"slug"`
	Icon      string      `json:"icon,omitempty"`
	SortOrder int         `json:"sort_order"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
	Children  []*Category `json:"children,omitempty"`
}

// CategoryRequest é o body para criar/atualizar.
type CategoryRequest struct {
	ParentID  *string `json:"parent_id"`
	Name      string  `json:"name"      example:"Plantas"`
	Slug      string  `json:"slug"      example:"plantas"`
	Icon      string  `json:"icon"      example:"🌱"`
	SortOrder int     `json:"sort_order"`
}

// CategoryStore acessa categorias.
type CategoryStore struct{ DB *sql.DB }

// NewCategoryStore cria um store.
func NewCategoryStore(db *sql.DB) *CategoryStore { return &CategoryStore{DB: db} }

// List devolve todas as categorias em formato plano.
func (s *CategoryStore) List() ([]Category, error) {
	rows, err := s.DB.Query(
		`SELECT id, parent_id, name, slug, COALESCE(icon, ''), sort_order, created_at, updated_at
		 FROM categories ORDER BY sort_order, name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Category
	for rows.Next() {
		var c Category
		var parent sql.NullString
		if err := rows.Scan(&c.ID, &parent, &c.Name, &c.Slug, &c.Icon, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		if parent.Valid {
			s := parent.String
			c.ParentID = &s
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Tree monta a árvore a partir da lista plana.
func (s *CategoryStore) Tree() ([]*Category, error) {
	flat, err := s.List()
	if err != nil {
		return nil, err
	}
	byID := make(map[string]*Category)
	for i := range flat {
		flat[i].Children = []*Category{}
		byID[flat[i].ID] = &flat[i]
	}
	var roots []*Category
	for i := range flat {
		c := byID[flat[i].ID]
		if c.ParentID == nil {
			roots = append(roots, c)
		} else if parent, ok := byID[*c.ParentID]; ok {
			parent.Children = append(parent.Children, c)
		} else {
			roots = append(roots, c) // órfão vira raiz
		}
	}
	return roots, nil
}

// Create insere uma categoria.
func (s *CategoryStore) Create(req CategoryRequest) (Category, error) {
	if strings.TrimSpace(req.Name) == "" {
		return Category{}, errors.New("name is required")
	}
	if req.Slug == "" {
		req.Slug = slugify(req.Name)
	}
	var c Category
	var parent sql.NullString
	if req.ParentID != nil && *req.ParentID != "" {
		parent.String = *req.ParentID
		parent.Valid = true
	}
	err := s.DB.QueryRow(
		`INSERT INTO categories (parent_id, name, slug, icon, sort_order)
		 VALUES (NULLIF($1, '')::uuid, $2, $3, $4, $5)
		 RETURNING id, parent_id, name, slug, COALESCE(icon, ''), sort_order, created_at, updated_at`,
		nullStr(parent), req.Name, req.Slug, req.Icon, req.SortOrder,
	).Scan(&c.ID, &parent, &c.Name, &c.Slug, &c.Icon, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return Category{}, err
	}
	if parent.Valid {
		v := parent.String
		c.ParentID = &v
	}
	return c, nil
}

// Update edita uma categoria.
func (s *CategoryStore) Update(id string, req CategoryRequest) (Category, bool, error) {
	if strings.TrimSpace(req.Name) == "" {
		return Category{}, false, errors.New("name is required")
	}
	if req.Slug == "" {
		req.Slug = slugify(req.Name)
	}
	var parent sql.NullString
	if req.ParentID != nil && *req.ParentID != "" {
		parent.String = *req.ParentID
		parent.Valid = true
	}
	var c Category
	err := s.DB.QueryRow(
		`UPDATE categories
		    SET parent_id  = NULLIF($1, '')::uuid,
		        name       = $2,
		        slug       = $3,
		        icon       = $4,
		        sort_order = $5,
		        updated_at = now()
		  WHERE id = $6
		  RETURNING id, parent_id, name, slug, COALESCE(icon, ''), sort_order, created_at, updated_at`,
		nullStr(parent), req.Name, req.Slug, req.Icon, req.SortOrder, id,
	).Scan(&c.ID, &parent, &c.Name, &c.Slug, &c.Icon, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Category{}, false, nil
	}
	if err != nil {
		return Category{}, false, err
	}
	if parent.Valid {
		v := parent.String
		c.ParentID = &v
	}
	return c, true, nil
}

// Delete remove uma categoria (cascata pelos filhos).
func (s *CategoryStore) Delete(id string) (bool, error) {
	res, err := s.DB.Exec(`DELETE FROM categories WHERE id = $1`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// DescendantsOf retorna o ID da categoria + todos os descendentes (recursivo).
func (s *CategoryStore) DescendantsOf(id string) ([]string, error) {
	rows, err := s.DB.Query(`
WITH RECURSIVE tree AS (
    SELECT id FROM categories WHERE id = $1
    UNION ALL
    SELECT c.id FROM categories c JOIN tree t ON c.parent_id = t.id
)
SELECT id FROM tree`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ===== HTTP handlers =====

// ListHandler godoc
// @Summary Lista categorias em árvore
// @Tags    catalog
// @Produce json
// @Success 200 {array} Category
// @Router  /api/categories [get]
func (s *CategoryStore) ListHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("flat") == "1" {
		list, err := s.List()
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, list)
		return
	}
	tree, err := s.Tree()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, tree)
}

// CreateHandler godoc
// @Summary  Cria nova categoria (admin)
// @Tags     catalog
// @Accept   json
// @Produce  json
// @Param    body body CategoryRequest true "Dados"
// @Success  201 {object} Category
// @Security BearerAuth
// @Router   /api/categories [post]
func (s *CategoryStore) CreateHandler(w http.ResponseWriter, r *http.Request) {
	var req CategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}
	c, err := s.Create(req)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 201, c)
}

// UpdateHandler atualiza categoria (admin).
func (s *CategoryStore) UpdateHandler(id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CategoryRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid body"})
			return
		}
		c, ok, err := s.Update(id, req)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		if !ok {
			writeJSON(w, 404, map[string]string{"error": "category not found"})
			return
		}
		writeJSON(w, 200, c)
	}
}

// DeleteHandler remove categoria (admin).
func (s *CategoryStore) DeleteHandler(id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ok, err := s.Delete(id)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		if !ok {
			writeJSON(w, 404, map[string]string{"error": "category not found"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ----- helpers -----

var slugRE = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	rep := map[string]string{
		"á": "a", "à": "a", "ã": "a", "â": "a", "ä": "a",
		"é": "e", "ê": "e", "è": "e", "ë": "e",
		"í": "i", "ì": "i", "î": "i", "ï": "i",
		"ó": "o", "ò": "o", "õ": "o", "ô": "o", "ö": "o",
		"ú": "u", "ù": "u", "û": "u", "ü": "u",
		"ç": "c", "ñ": "n",
	}
	for k, v := range rep {
		s = strings.ReplaceAll(s, k, v)
	}
	s = slugRE.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "item"
	}
	return s
}

func nullStr(n sql.NullString) any {
	if !n.Valid {
		return ""
	}
	return n.String
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
