// Package migrations contém as migrations SQL idempotentes do sistema.
package migrations

import "database/sql"

// Run cria todas as tabelas se ainda não existirem.
func Run(db *sql.DB) error {
	stmts := []string{
		`CREATE EXTENSION IF NOT EXISTS pgcrypto`,

		// ---- users (já existia) ----
		`CREATE TABLE IF NOT EXISTS users (
			id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name       TEXT NOT NULL,
			email      TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,

		// ---- customers (autenticação) ----
		`CREATE TABLE IF NOT EXISTS customers (
			id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name          TEXT NOT NULL,
			email         TEXT NOT NULL UNIQUE,
			phone         TEXT,
			password_hash TEXT NOT NULL,
			role          TEXT NOT NULL DEFAULT 'customer',
			created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_customers_email ON customers(email)`,

		// google login (idempotente)
		`ALTER TABLE customers ADD COLUMN IF NOT EXISTS google_sub TEXT`,
		`ALTER TABLE customers ADD COLUMN IF NOT EXISTS picture TEXT`,
		`ALTER TABLE customers ALTER COLUMN password_hash DROP NOT NULL`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_customers_google_sub ON customers(google_sub) WHERE google_sub IS NOT NULL`,

		// ---- categories (recursivas via parent_id) ----
		`CREATE TABLE IF NOT EXISTS categories (
			id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			parent_id  UUID REFERENCES categories(id) ON DELETE CASCADE,
			name       TEXT NOT NULL,
			slug       TEXT NOT NULL,
			icon       TEXT,
			sort_order INT NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_categories_parent ON categories(parent_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_categories_slug_parent ON categories(slug, COALESCE(parent_id, '00000000-0000-0000-0000-000000000000'::uuid))`,

		// ---- products (preço NULL = sob consulta) ----
		`CREATE TABLE IF NOT EXISTS products (
			id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name        TEXT NOT NULL,
			slug        TEXT NOT NULL UNIQUE,
			description TEXT NOT NULL DEFAULT '',
			price_cents INT,
			images      JSONB NOT NULL DEFAULT '[]'::jsonb,
			stock       INT NOT NULL DEFAULT 0,
			weight_kg   NUMERIC(8,3),
			is_active   BOOLEAN NOT NULL DEFAULT TRUE,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_products_active ON products(is_active)`,

		// ---- product_categories (M:N) ----
		`CREATE TABLE IF NOT EXISTS product_categories (
			product_id  UUID NOT NULL REFERENCES products(id)  ON DELETE CASCADE,
			category_id UUID NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
			PRIMARY KEY (product_id, category_id)
		)`,

		// ---- orders ----
		`CREATE TABLE IF NOT EXISTS orders (
			id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			customer_id     UUID REFERENCES customers(id) ON DELETE SET NULL,
			customer_name   TEXT NOT NULL,
			customer_email  TEXT NOT NULL,
			customer_phone  TEXT NOT NULL,
			cep             TEXT NOT NULL,
			street          TEXT NOT NULL,
			number          TEXT,
			complement      TEXT,
			neighborhood    TEXT,
			city            TEXT NOT NULL,
			state           TEXT NOT NULL,
			lat             NUMERIC(10,7),
			lng             NUMERIC(10,7),
			subtotal_cents  INT NOT NULL DEFAULT 0,
			shipping_cents  INT NOT NULL DEFAULT 0,
			total_cents     INT NOT NULL DEFAULT 0,
			status          TEXT NOT NULL DEFAULT 'pending_payment',
			payment_status  TEXT NOT NULL DEFAULT 'pending',
			lalamove_quotation_id TEXT,
			lalamove_order_id     TEXT,
			notes           TEXT,
			created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_orders_customer ON orders(customer_id)`,
		`CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status)`,

		// ---- order_items ----
		`CREATE TABLE IF NOT EXISTS order_items (
			id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			order_id      UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
			product_id    UUID REFERENCES products(id) ON DELETE SET NULL,
			product_name  TEXT NOT NULL,
			quantity      INT NOT NULL DEFAULT 1,
			unit_cents    INT,
			subtotal_cents INT,
			under_consult BOOLEAN NOT NULL DEFAULT FALSE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_order_items_order ON order_items(order_id)`,

		// ---- payments (Asaas) ----
		`CREATE TABLE IF NOT EXISTS payments (
			id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			order_id        UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
			provider        TEXT NOT NULL DEFAULT 'asaas',
			provider_payment_id TEXT,
			method          TEXT NOT NULL DEFAULT 'PIX',
			amount_cents    INT NOT NULL,
			status          TEXT NOT NULL DEFAULT 'PENDING',
			pix_qr_code     TEXT,
			pix_payload     TEXT,
			pix_expires_at  TIMESTAMPTZ,
			paid_at         TIMESTAMPTZ,
			raw_payload     JSONB,
			created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_payments_order ON payments(order_id)`,
		`CREATE INDEX IF NOT EXISTS idx_payments_provider_id ON payments(provider_payment_id)`,
	}

	for _, q := range stmts {
		if _, err := db.Exec(q); err != nil {
			return err
		}
	}

	// Garantir que existe ao menos um admin (controlado por env).
	return nil
}
