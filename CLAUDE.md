# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

```bash
go mod tidy
go build .
./userapi          # Linux/Mac
.\userapi.exe      # Windows
```

Requer um arquivo `.env` na raiz (ou variáveis de ambiente exportadas). A aplicação aplica as migrações automaticamente na inicialização.

Regenerar docs Swagger após alterar anotações:
```bash
swag init
```

Docker:
```bash
docker build -t florentina .
docker run -p 8080:8080 --env-file .env florentina
```

URLs após subir: loja em `/site/index.html`, admin em `/site/admin.html`, Swagger em `/swagger/index.html`.

## Variáveis de ambiente obrigatórias

```
DATABASE_URL        # PostgreSQL connection string
JWT_SECRET          # segredo HS256
ADMIN_EMAIL         # admin criado/atualizado a cada start
ADMIN_PASSWORD
ASAAS_API_KEY       # pagamentos Pix
ASAAS_ENV           # "sandbox" ou "production"
ASAAS_WEBHOOK_TOKEN # valida header asaas-access-token no webhook
LALAMOVE_API_KEY
LALAMOVE_API_SECRET
LALAMOVE_MARKET     # padrão "BR"
GOOGLE_CLIENT_ID    # opcional, habilita login Google
CORS_ALLOWED_ORIGINS # padrão "*"
PORT                # padrão "8080"
```

## Arquitetura

### Backend (Go)

O `main.go` é o único ponto de roteamento — todas as rotas estão registradas lá via `http.NewServeMux()` nativo (sem framework de roteamento externo). Cada pacote expõe handlers diretamente como métodos ou funções retornando `http.HandlerFunc`.

**Pacotes internos:**

| Pacote | Responsabilidade |
|--------|-----------------|
| `auth` | JWT HS256, bcrypt, middlewares (`Middleware`, `AdminOnly`, `Optional`), Google OAuth via tokeninfo |
| `catalog` | Categorias recursivas (árvore via `parent_id` auto-referencial) e produtos com M:N em `product_categories` |
| `orders` | Ciclo completo: criação → cobrança Pix Asaas → webhook de confirmação → notificação WS |
| `payments` | Cliente HTTP do Asaas (customer + payment + QR Code) |
| `cep` | Consulta ViaCEP + whitelist de cidades entregáveis (Grande SP/ABC) |
| `notifier` | Hub WebSocket para notificar cliente sobre `payment_confirmed` e `ready_for_delivery` |
| `lalamove` | Cliente da API Lalamove v3 (cotação, pedido, motorista, webhook) |
| `tracking` | Hub WebSocket separado para rastreamento em tempo real do motorista |
| `routing` | Integração OSRM para calcular rota geográfica na simulação de tracking |
| `migrations` | DDL idempotente executado a cada start via `migrations.Run(db)` |

### Middlewares de auth

- `authSvc.Middleware` — exige JWT válido, injeta `*auth.Claims` no contexto
- `authSvc.AdminOnly` — wraps Middleware + verifica `role == "admin"`
- `authSvc.Optional` — injeta claims se houver token, mas não bloqueia anônimos
- Recuperar usuário do contexto: `auth.FromContext(r.Context())`

### Banco de dados

Schema em `migrations/schema.go`, executado com `migrations.Run(db)` — todas as statements são idempotentes (`CREATE IF NOT EXISTS`, `ALTER ... ADD COLUMN IF NOT EXISTS`). Para adicionar colunas novas, basta append nessa slice.

Tabelas principais: `customers`, `categories` (recursiva), `products`, `product_categories` (M:N), `orders`, `order_items`, `payments`.

Produtos com `price_cents = NULL` são "sob consulta" — o fluxo de pedido não gera cobrança Asaas nesses casos.

### Frontend

Todos os arquivos ficam em `site/`. O `api.js` é incluído em todas as páginas e provê:
- `API.request/get/post/put/patch/del` — cliente HTTP com JWT automático
- `API.setSession / logout / isAdmin` — gestão de sessão via `localStorage`
- `Cart.*` — carrinho persistido em `localStorage`

A coordenada da loja está hardcoded em `site/checkout.html` como `const STORE = {lat, lng}` — ajustar para o endereço real.

### Fluxo Pix

`POST /api/orders` → cria pedido no DB → cria customer Asaas (idempotente por email) → cria payment PIX → retorna `{order, payment: {pix_qr_code, pix_copy_paste}}`. Confirmação chega via `POST /webhook/asaas` (valida header `asaas-access-token`) → atualiza `payment_status` → dispara WS via `notifier.Hub`. Frontend mantém polling paralelo em `/api/orders/{id}/payment-status` como fallback.

### WebSockets

Dois hubs independentes:
- `notifier.Hub` — `/ws/orders/{id}` — notificações de pedido (pagamento, pronto)
- `tracking.Hub` — `/ws/tracking/{orderId}` — posição do motorista Lalamove em tempo real

## Padrões do código

- Respostas JSON via `writeJSON(w, status, v)` e erros via `writeError(w, status, msg)` definidos em `main.go`
- Roteamento por método com `methodGate(map[string]http.HandlerFunc{...})`
- Novos endpoints admin: envolver com `authSvc.AdminOnly(...)`
- Migrações novas: append em `migrations/schema.go`, sempre idempotentes
