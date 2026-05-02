// Package cep valida CEPs e responde se entregamos na região.
package cep

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Result é o que devolvemos para o frontend.
type Result struct {
	CEP          string `json:"cep"`
	Street       string `json:"street"`
	Neighborhood string `json:"neighborhood"`
	City         string `json:"city"`
	State        string `json:"state"`
	IBGE         string `json:"ibge"`
	Deliverable  bool   `json:"deliverable"`
	Reason       string `json:"reason,omitempty"`
}

// viaCEPResponse é o payload do ViaCEP.
type viaCEPResponse struct {
	CEP        string `json:"cep"`
	Logradouro string `json:"logradouro"`
	Bairro     string `json:"bairro"`
	Localidade string `json:"localidade"`
	UF         string `json:"uf"`
	IBGE       string `json:"ibge"`
	Erro       bool   `json:"erro"`
}

// Service consulta CEP e decide se entrega.
type Service struct {
	httpClient        *http.Client
	deliverableCities map[string]bool
}

// New retorna um Service com lista padrão de cidades atendidas (SP + Grande SP/ABC).
func New() *Service {
	cities := []string{
		"São Paulo",
		"Guarulhos",
		"Osasco",
		"Santo André",
		"São Bernardo do Campo",
		"São Caetano do Sul",
		"Diadema",
		"Mauá",
		"Ribeirão Pires",
		"Rio Grande da Serra",
		"Barueri",
		"Carapicuíba",
		"Embu das Artes",
		"Itapecerica da Serra",
		"Taboão da Serra",
		"Cotia",
		"Jandira",
		"Santana de Parnaíba",
		"Itaquaquecetuba",
		"Suzano",
		"Mogi das Cruzes",
		"Poá",
		"Ferraz de Vasconcelos",
		"Arujá",
	}
	m := make(map[string]bool, len(cities))
	for _, c := range cities {
		m[normalize(c)] = true
	}
	return &Service{
		httpClient:        &http.Client{Timeout: 6 * time.Second},
		deliverableCities: m,
	}
}

var cepRE = regexp.MustCompile(`^\d{8}$`)

// Lookup consulta um CEP no ViaCEP.
func (s *Service) Lookup(raw string) (Result, error) {
	clean := strings.NewReplacer("-", "", ".", "", " ", "").Replace(strings.TrimSpace(raw))
	if !cepRE.MatchString(clean) {
		return Result{}, errors.New("CEP deve conter 8 dígitos")
	}

	resp, err := s.httpClient.Get("https://viacep.com.br/ws/" + clean + "/json/")
	if err != nil {
		return Result{}, errors.New("falha ao consultar CEP: " + err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return Result{}, errors.New("CEP não encontrado")
	}

	var via viaCEPResponse
	if err := json.NewDecoder(resp.Body).Decode(&via); err != nil {
		return Result{}, errors.New("resposta inválida do ViaCEP")
	}
	if via.Erro || via.Localidade == "" {
		return Result{}, errors.New("CEP não encontrado")
	}

	r := Result{
		CEP:          formatCEP(clean),
		Street:       via.Logradouro,
		Neighborhood: via.Bairro,
		City:         via.Localidade,
		State:        via.UF,
		IBGE:         via.IBGE,
	}
	if s.IsDeliverable(via.Localidade, via.UF) {
		r.Deliverable = true
	} else {
		r.Reason = "No momento entregamos apenas em São Paulo, Guarulhos, ABC e cidades vizinhas."
	}
	return r, nil
}

// IsDeliverable indica se a cidade/uf está coberta.
func (s *Service) IsDeliverable(city, state string) bool {
	if strings.ToUpper(state) != "SP" {
		return false
	}
	return s.deliverableCities[normalize(city)]
}

// Cities lista as cidades atendidas (UI mostra para o cliente).
func (s *Service) Cities() []string {
	out := make([]string, 0, len(s.deliverableCities))
	for k := range s.deliverableCities {
		out = append(out, k)
	}
	return out
}

// LookupHandler godoc
// @Summary  Valida um CEP e retorna se entregamos
// @Tags     cep
// @Produce  json
// @Param    cep query string true "CEP (com ou sem hífen)"
// @Success  200 {object} Result
// @Failure  400 {object} map[string]string
// @Router   /api/cep [get]
func (s *Service) LookupHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("cep")
	if q == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"missing cep"}`))
		return
	}
	res, err := s.Lookup(q)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	rep := map[string]string{
		"á": "a", "à": "a", "ã": "a", "â": "a", "ä": "a",
		"é": "e", "ê": "e",
		"í": "i", "î": "i",
		"ó": "o", "ô": "o", "õ": "o",
		"ú": "u", "ü": "u",
		"ç": "c",
	}
	for k, v := range rep {
		s = strings.ReplaceAll(s, k, v)
	}
	return s
}

func formatCEP(clean string) string {
	if len(clean) != 8 {
		return clean
	}
	return clean[:5] + "-" + clean[5:]
}
