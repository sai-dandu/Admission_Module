package db

import (
	"admission-module/config"
	"database/sql"
	"fmt"
	"log"

	_ "github.com/lib/pq"
)

var DB *sql.DB

func InitDB() error {
	var err error
	connStr := config.GetDBConnString()

	DB, err = sql.Open("postgres", connStr)
	if err != nil {
		return fmt.Errorf("error opening database: %w", err)
	}

	// Test the connection
	err = DB.Ping()
	if err != nil {
		return fmt.Errorf("error connecting to database: %w", err)
	}

	// Create tables
	if err := createTables(); err != nil {
		return fmt.Errorf("error creating tables: %w", err)
	}

	return nil
}

func createTables() error {
	counsellorTable := `
	CREATE TABLE IF NOT EXISTS counsellors (
		id SERIAL PRIMARY KEY,
		name TEXT,
		email TEXT,
		assigned_count INTEGER DEFAULT 0,
		max_capacity INTEGER DEFAULT 10
	);`

	leadTable := `
	CREATE TABLE IF NOT EXISTS leads (
		id SERIAL PRIMARY KEY,
		name TEXT,
		email TEXT,
		phone TEXT,
		education TEXT,
		lead_source TEXT,
		counsellor_id INTEGER,
		payment_status TEXT,
		meet_link TEXT,
		application_status TEXT DEFAULT 'NEW',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

		CONSTRAINT fk_counsellor
			FOREIGN KEY (counsellor_id)
			REFERENCES counsellors(id)
			ON DELETE SET NULL
	);`

	paymentTable := `
	CREATE TABLE IF NOT EXISTS payments (
		id SERIAL PRIMARY KEY,
		student_id INTEGER,
		amount REAL,
		status TEXT,
		timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		order_id TEXT,
		payment_id TEXT,
		razorpay_sign TEXT,

		CONSTRAINT fk_student
			FOREIGN KEY (student_id)
			REFERENCES leads(id)
			ON DELETE CASCADE
	);`

	// Create counsellors first so leads can reference it
	if _, err := DB.Exec(counsellorTable); err != nil {
		return fmt.Errorf("error creating counsellors table: %w", err)
	}

	// Create leads next
	if _, err := DB.Exec(leadTable); err != nil {
		return fmt.Errorf("error creating leads table: %w", err)
	}

	// Create payments last
	if _, err := DB.Exec(paymentTable); err != nil {
		return fmt.Errorf("error creating payments table: %w", err)
	}

	// Insert sample counsellors if not exist
	if _, err := DB.Exec(`INSERT INTO counsellors (name, email) SELECT 'Counsellor 1', 'c1@example.com' WHERE NOT EXISTS (SELECT 1 FROM counsellors WHERE name = 'Counsellor 1')`); err != nil {
		log.Println("Warning: Error inserting sample counsellor:", err)
	}
	if _, err := DB.Exec(`INSERT INTO counsellors (name, email) SELECT 'Counsellor 2', 'c2@example.com' WHERE NOT EXISTS (SELECT 1 FROM counsellors WHERE name = 'Counsellor 2')`); err != nil {
		log.Println("Warning: Error inserting sample counsellor:", err)
	}

	return nil
}
