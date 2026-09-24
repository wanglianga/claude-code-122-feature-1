package main

import "database/sql"

// migrate 建表（幂等）。
func migrate(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS dealers (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			city TEXT NOT NULL,
			contact TEXT DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS bars (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			city TEXT NOT NULL,
			dealer_id INT REFERENCES dealers(id),
			address TEXT DEFAULT '',
			manager_name TEXT DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS users (
			id SERIAL PRIMARY KEY,
			username TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			name TEXT NOT NULL,
			role TEXT NOT NULL,
			city TEXT DEFAULT '',
			dealer_id INT,
			bar_id INT,
			created_at TIMESTAMPTZ DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS batches (
			id SERIAL PRIMARY KEY,
			batch_no TEXT UNIQUE NOT NULL,
			style TEXT NOT NULL,
			fill_date DATE NOT NULL,
			best_before DATE NOT NULL,
			status TEXT NOT NULL DEFAULT 'active',
			suspend_reason TEXT DEFAULT '',
			created_at TIMESTAMPTZ DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS kegs (
			id SERIAL PRIMARY KEY,
			keg_no TEXT UNIQUE NOT NULL,
			capacity_liters INT NOT NULL,
			batch_id INT REFERENCES batches(id),
			deposit_amount NUMERIC(10,2) NOT NULL DEFAULT 200,
			status TEXT NOT NULL DEFAULT 'in_warehouse',
			current_bar_id INT REFERENCES bars(id),
			refill_eligible BOOLEAN NOT NULL DEFAULT TRUE,
			delivered_at TIMESTAMPTZ,
			opened_at TIMESTAMPTZ,
			emptied_at TIMESTAMPTZ,
			returned_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS shipments (
			id SERIAL PRIMARY KEY,
			order_no TEXT UNIQUE NOT NULL,
			dealer_id INT REFERENCES dealers(id),
			bar_id INT REFERENCES bars(id),
			driver_id INT REFERENCES users(id),
			status TEXT NOT NULL DEFAULT 'pending',
			is_festival BOOLEAN NOT NULL DEFAULT FALSE,
			note TEXT DEFAULT '',
			receiver_name TEXT DEFAULT '',
			created_at TIMESTAMPTZ DEFAULT now(),
			started_at TIMESTAMPTZ,
			delivered_at TIMESTAMPTZ,
			received_at TIMESTAMPTZ
		)`,
		`CREATE TABLE IF NOT EXISTS shipment_kegs (
			shipment_id INT REFERENCES shipments(id),
			keg_id INT REFERENCES kegs(id),
			PRIMARY KEY (shipment_id, keg_id)
		)`,
		`CREATE TABLE IF NOT EXISTS keg_feedback (
			id SERIAL PRIMARY KEY,
			keg_id INT REFERENCES kegs(id),
			batch_id INT REFERENCES batches(id),
			bar_id INT REFERENCES bars(id),
			pour_speed TEXT DEFAULT '',
			pressure_issue BOOLEAN NOT NULL DEFAULT FALSE,
			taste_issue BOOLEAN NOT NULL DEFAULT FALSE,
			complaint BOOLEAN NOT NULL DEFAULT FALSE,
			detail TEXT DEFAULT '',
			created_by INT REFERENCES users(id),
			created_at TIMESTAMPTZ DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS pickups (
			id SERIAL PRIMARY KEY,
			pickup_no TEXT UNIQUE NOT NULL,
			bar_id INT REFERENCES bars(id),
			driver_id INT REFERENCES users(id),
			status TEXT NOT NULL DEFAULT 'booked',
			note TEXT DEFAULT '',
			expected_count INT NOT NULL DEFAULT 0,
			scanned_count INT NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ DEFAULT now(),
			completed_at TIMESTAMPTZ
		)`,
		`CREATE TABLE IF NOT EXISTS pickup_items (
			id SERIAL PRIMARY KEY,
			pickup_id INT REFERENCES pickups(id),
			keg_id INT REFERENCES kegs(id),
			scanned BOOLEAN NOT NULL DEFAULT FALSE,
			appearance TEXT DEFAULT '',
			residual_liquid TEXT DEFAULT '',
			cap_status TEXT DEFAULT '',
			scanned_by INT,
			scanned_at TIMESTAMPTZ
		)`,
		`CREATE TABLE IF NOT EXISTS wash_records (
			id SERIAL PRIMARY KEY,
			keg_id INT REFERENCES kegs(id),
			washer_id INT REFERENCES users(id),
			result TEXT NOT NULL,
			note TEXT DEFAULT '',
			created_at TIMESTAMPTZ DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS deposit_ledger (
			id SERIAL PRIMARY KEY,
			keg_id INT REFERENCES kegs(id),
			bar_id INT REFERENCES bars(id),
			entry_type TEXT NOT NULL,
			amount NUMERIC(10,2) NOT NULL,
			reason TEXT DEFAULT '',
			period TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			created_by INT,
			created_at TIMESTAMPTZ DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS cases (
			id SERIAL PRIMARY KEY,
			case_no TEXT UNIQUE NOT NULL,
			case_type TEXT NOT NULL,
			keg_id INT,
			batch_id INT,
			bar_id INT,
			status TEXT NOT NULL DEFAULT 'open',
			title TEXT NOT NULL,
			detail TEXT DEFAULT '',
			resolution TEXT DEFAULT '',
			created_at TIMESTAMPTZ DEFAULT now(),
			resolved_at TIMESTAMPTZ
		)`,
		`CREATE TABLE IF NOT EXISTS case_notes (
			id SERIAL PRIMARY KEY,
			case_id INT REFERENCES cases(id),
			user_id INT REFERENCES users(id),
			role TEXT NOT NULL,
			user_name TEXT DEFAULT '',
			note TEXT NOT NULL,
			created_at TIMESTAMPTZ DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS notifications (
			id SERIAL PRIMARY KEY,
			user_id INT,
			role TEXT DEFAULT '',
			bar_id INT,
			dealer_id INT,
			ntype TEXT NOT NULL,
			title TEXT NOT NULL,
			content TEXT DEFAULT '',
			read BOOLEAN NOT NULL DEFAULT FALSE,
			created_at TIMESTAMPTZ DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS keg_events (
			id SERIAL PRIMARY KEY,
			keg_id INT REFERENCES kegs(id),
			event TEXT NOT NULL,
			actor_id INT,
			actor_name TEXT DEFAULT '',
			detail TEXT DEFAULT '',
			created_at TIMESTAMPTZ DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_kegs_status ON kegs(status)`,
		`CREATE INDEX IF NOT EXISTS idx_kegs_batch ON kegs(batch_id)`,
		`CREATE INDEX IF NOT EXISTS idx_kegs_bar ON kegs(current_bar_id)`,
		`CREATE INDEX IF NOT EXISTS idx_feedback_batch ON keg_feedback(batch_id)`,
		`CREATE INDEX IF NOT EXISTS idx_deposit_keg ON deposit_ledger(keg_id)`,
		`CREATE INDEX IF NOT EXISTS idx_events_keg ON keg_events(keg_id)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}
	return nil
}
