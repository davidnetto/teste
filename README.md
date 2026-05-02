# Florentina — E-commerce de Plantas

Loja de plantas com cadastro de usuário, catálogo recursivo, pagamento Pix (Asaas) e entrega via Lalamove.

## ✨ Funcionalidades

- **Autenticação JWT** com bcrypt (registro, login, perfil, role admin/customer)
- **Catálogo** com **categorias recursivas** (N níveis) — produto pode pertencer a várias categorias
- **Produtos** com preço fixo OU **"sob consulta"**
- **Validação de CEP** via ViaCEP — só aceita SP, Guarulhos, ABC e Grande SP
- **Carrinho** persistido no localStorage
- **Checkout** com mapa Leaflet (origem da loja → destino) e cotação Lalamove
- **Pagamento Pix via Asaas** com QR Code, polling de status e webhook
- **Animação de fogos** quando o pagamento cai
- **Notificação por WebSocket** quando o admin marca o pedido como "pronto para entrega"
- **Painel Admin** completo (categorias em árvore, produtos, pedidos)
- **Rastreamento Lalamove** em tempo real (já existente)

## 🗂️ Estrutura

```
teste/
├── auth/              JWT, bcrypt, middleware
├── catalog/           categorias recursivas + produtos
├── cep/               ViaCEP + cidades atendidas
├── orders/            pedidos + Pix + notificações
├── payments/          cliente Asaas
├── notifier/          hub WebSocket de notificações
├── lalamove/          cliente Lalamove (existente)
├── routing/           OSRM (existente)
├── tracking/          hub WS de rastreamento (existente)
├── migrations/        schema PostgreSQL
├── docs/              swagger gerado
├── site/
│   ├── index.html         loja (com popup de CEP)
│   ├── login.html         login do cliente
│   ├── register.html      cadastro do cliente
│   ├── checkout.html      checkout + mapa + Pix + fogos
│   ├── admin.html         painel admin
│   ├── meus-pedidos.html  pedidos do cliente
│   ├── styles.css         estilos compartilhados
│   └── api.js             cliente HTTP + carrinho + helpers
├── static/tracking.html   rastreamento (existente)
└── main.go                roteamento
```

## 🚀 Setup

### 1. Pré-requisitos

- Go 1.21+
- PostgreSQL 14+

### 2. Banco de dados

```sql
CREATE DATABASE userapi;
```

A aplicação cria as tabelas automaticamente na primeira execução.

### 3. Variáveis de ambiente

Crie um arquivo `.env` (ou exporte direto no shell):

```bash
# Banco
DATABASE_URL="host=localhost port=5432 user=postgres password=postgres dbname=userapi sslmode=disable"

# JWT (TROQUE em produção!)
JWT_SECRET="troque-este-segredo-em-producao"

# Admin inicial (criado/atualizado a cada start)
ADMIN_EMAIL="admin@florentina.com.br"
ADMIN_PASSWORD="senha-forte-aqui"
ADMIN_NAME="Administrador Florentina"

# Asaas (sandbox)
ASAAS_API_KEY="$aact_..."        # pegue em https://sandbox.asaas.com → Integrações → API
ASAAS_ENV="sandbox"              # use "production" depois
ASAAS_WEBHOOK_TOKEN="defina-seu-token"   # configure este mesmo valor no painel do Asaas

# Lalamove (sandbox)
LALAMOVE_API_KEY="..."
LALAMOVE_API_SECRET="..."
LALAMOVE_MARKET="BR"

# Login com Google (opcional)
GOOGLE_CLIENT_ID="123456789-xxxxxxxxxx.apps.googleusercontent.com"
```

### Como obter o GOOGLE_CLIENT_ID

1. Acesse https://console.cloud.google.com/apis/credentials
2. Crie um projeto (ou use existente)
3. Em **OAuth consent screen** configure como External, preencha nome do app + email
4. Em **Credentials → Create credentials → OAuth client ID**:
   - Tipo: **Web application**
   - Authorized JavaScript origins: `http://localhost:8080` (e seu domínio em produção)
   - Authorized redirect URIs: não obrigatório para GSI (popup), mas pode adicionar `http://localhost:8080`
5. Copie o **Client ID** gerado e use como `GOOGLE_CLIENT_ID`

> O backend valida o ID token chamando `https://oauth2.googleapis.com/tokeninfo` — não precisa de client secret.

No Windows (PowerShell):

```powershell
$env:DATABASE_URL="host=localhost port=5432 user=postgres password=postgres dbname=userapi sslmode=disable"
$env:JWT_SECRET="troque-este-segredo-em-producao"
$env:ADMIN_EMAIL="admin@florentina.com.br"
$env:ADMIN_PASSWORD="senha-forte"
$env:ASAAS_API_KEY="$aact_..."
$env:LALAMOVE_API_KEY="..."
$env:LALAMOVE_API_SECRET="..."
```

### 4. Build & Run

```bash
go mod tidy
go build .
./userapi          # ou .\userapi.exe no Windows
```

Acesse:
- Loja: http://localhost:8080/site/index.html
- Admin: http://localhost:8080/site/admin.html
- Swagger: http://localhost:8080/swagger/index.html

## 🌐 Endpoints principais

### Auth
- `POST /api/auth/register` — `{name, email, phone, password}`
- `POST /api/auth/login` — `{email, password}` → `{token, expires_at, user}`
- `GET  /api/auth/me` — exige `Authorization: Bearer <token>`

### CEP
- `GET /api/cep?cep=01310100` → `{cep, city, state, deliverable, reason}`

### Categorias
- `GET    /api/categories` — árvore (use `?flat=1` para lista plana)
- `POST   /api/categories` — admin: `{parent_id, name, slug, icon, sort_order}`
- `PUT    /api/categories/{id}` — admin
- `DELETE /api/categories/{id}` — admin (cascata em filhas)

### Produtos
- `GET    /api/products?category=<id>&q=<busca>` — pública
- `GET    /api/products/{id|slug}` — pública
- `POST   /api/products` — admin: `{name, slug, description, price_cents, under_consult, images, stock, category_ids, weight_kg, is_active}`
- `PUT    /api/products/{id}` — admin
- `DELETE /api/products/{id}` — admin

### Pedidos
- `POST /api/orders` — público (autenticado opcional). Cria pedido + cobrança Pix Asaas
- `GET  /api/orders/{id}` — dono ou admin
- `GET  /api/orders/{id}/payment-status` — polling de status do Pix
- `GET  /api/me/orders` — pedidos do cliente autenticado
- `WS   /ws/orders/{id}` — recebe `payment_confirmed` e `ready_for_delivery`

### Admin
- `GET  /api/admin/orders` — admin
- `POST /api/admin/orders/{id}/ready` — admin: notifica cliente que o pedido está pronto

### Webhooks
- `POST /webhook/asaas` — Asaas envia confirmação de Pix (header `asaas-access-token`)
- `POST /webhook/lalamove` — Lalamove envia atualização de motorista

## 💳 Fluxo do Pix (Asaas)

1. Cliente preenche checkout e clica em "Gerar Pix"
2. Backend cria customer Asaas → cria payment (BillingType=PIX) → busca QR Code
3. Frontend mostra QR + código copia-e-cola, e inicia:
   - **WebSocket** em `/ws/orders/{id}` (instantâneo)
   - **Polling** GET `/api/orders/{id}/payment-status` a cada 5s (fallback)
4. Quando o cliente paga:
   - Asaas envia `PAYMENT_RECEIVED` para `/webhook/asaas`
   - Backend marca pedido como pago e dispara WS `payment_confirmed`
   - Frontend mostra **animação de fogos** 🎆

### Configurar webhook do Asaas

No painel sandbox: Integrações → Webhooks → adicione:
- URL: `https://seu-dominio.com/webhook/asaas`
- Token de autenticação: o mesmo valor de `ASAAS_WEBHOOK_TOKEN`
- Eventos: `PAYMENT_CONFIRMED`, `PAYMENT_RECEIVED`

Em desenvolvimento local, use `ngrok http 8080` para expor.

## 🚚 Fluxo Lalamove

1. Cliente preenche endereço no checkout
2. Frontend geocodifica via Nominatim (gratuito, OSM)
3. Mostra rota loja → destino no mapa Leaflet
4. Cliente clica "Calcular Frete" → backend chama `/v3/quotations`
5. Frontend mostra valor e total atualizado
6. Após Pix pago, o admin pode disparar a entrega manualmente (placeOrder na API Lalamove)

> A coordenada da loja está hardcoded em `site/checkout.html` como `STORE = {lat: -23.5505, lng: -46.6333}`. Ajuste para seu endereço real.

## 👨‍💼 Painel Admin

Login com as credenciais de `ADMIN_EMAIL` / `ADMIN_PASSWORD`.

- **Aba Produtos**: cadastrar/editar com toggle "Preço fixo / Sob consulta", múltiplas categorias, múltiplas imagens (URLs)
- **Aba Categorias**: criar árvore (categoria-pai dropdown lista todos os níveis com indentação)
- **Aba Pedidos**: ver todos os pedidos, status de pagamento, e botão **✅ Pronto** para notificar o cliente

## 🧪 Smoke test

```bash
# Cadastra um cliente
curl -X POST http://localhost:8080/api/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"name":"Maria","email":"maria@x.com","password":"abcdef","phone":"+5511999999999"}'

# Login admin
TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@florentina.com.br","password":"senha-forte"}' | jq -r .token)

# Cria categoria-pai
curl -X POST http://localhost:8080/api/categories \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"Plantas","icon":"🌱"}'

# Cria produto
curl -X POST http://localhost:8080/api/products \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"Espada de São Jorge","price_cents":12990,"description":"...","images":["https://..."],"stock":10}'

# Valida CEP
curl 'http://localhost:8080/api/cep?cep=01310-100'
```

## 🔒 Segurança implementada

- Senhas com **bcrypt** (cost padrão 10)
- **JWT HS256** com expiração de 24h e claims `sub/email/role/name`
- Middleware `Middleware`, `AdminOnly`, `Optional`
- **CORS** configurado (ajuste o `Access-Control-Allow-Origin` em produção)
- Webhook Asaas valida `asaas-access-token` se `ASAAS_WEBHOOK_TOKEN` estiver setado
- Validação de CEP no backend antes de criar pedido (não confia no frontend)
- Itens "sob consulta" não disparam cobrança automática

## 📝 Próximos passos sugeridos

- Trocar `Access-Control-Allow-Origin: *` por origem fixa em produção
- Adicionar rate limiting no login
- Adicionar refresh token (hoje só access token)
- Confirmar e-mail no cadastro
- Upload de imagens (S3/local) em vez de só URLs
- Geração automática do pedido Lalamove após Pix confirmado
