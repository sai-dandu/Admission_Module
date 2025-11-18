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
	// Renamed: counsellors -> counselor
	counselorTable := `
	CREATE TABLE IF NOT EXISTS counselor (
		id SERIAL PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		email VARCHAR(255) NOT NULL,
		phone VARCHAR(20),
		assigned_count INTEGER DEFAULT 0,
		max_capacity INTEGER DEFAULT 10,
		is_referral_enabled BOOLEAN DEFAULT false,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);`

	// Renamed: courses -> course
	courseTable := `
	CREATE TABLE IF NOT EXISTS course (
		id SERIAL PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		description TEXT,
		fee NUMERIC(10, 2) NOT NULL,
		duration VARCHAR(100),
		is_active INTEGER DEFAULT 1,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);`

	// Renamed: leads -> student_lead
	leadTable := `
	CREATE TABLE IF NOT EXISTS student_lead (
		id SERIAL PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		email VARCHAR(255) NOT NULL,
		phone VARCHAR(20) NOT NULL,
		education VARCHAR(255),
		lead_source VARCHAR(100),
		counselor_id INTEGER,
		registration_fee_status VARCHAR(50) DEFAULT 'PENDING',
		course_fee_status VARCHAR(50) DEFAULT 'PENDING',
		meet_link TEXT,
		application_status VARCHAR(50) DEFAULT 'NEW',
		registration_payment_id INTEGER,
		selected_course_id INTEGER,
		course_payment_id INTEGER,
		interview_scheduled_at TIMESTAMP,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

		CONSTRAINT fk_counselor
			FOREIGN KEY (counselor_id)
			REFERENCES counselor(id)
			ON DELETE SET NULL,
		CONSTRAINT fk_selected_course
			FOREIGN KEY (selected_course_id)
			REFERENCES course(id)
			ON DELETE SET NULL
	);`

	// Registration fee payment table
	registrationPaymentTable := `
	CREATE TABLE IF NOT EXISTS registration_payment (
		id SERIAL PRIMARY KEY,
		student_id INTEGER NOT NULL UNIQUE,
		amount NUMERIC(10, 2) NOT NULL,
		status VARCHAR(50) DEFAULT 'PENDING',
		order_id VARCHAR(255) UNIQUE,
		payment_id VARCHAR(255),
		razorpay_sign TEXT,
		timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

		CONSTRAINT fk_student_reg_payment
			FOREIGN KEY (student_id)
			REFERENCES student_lead(id)
			ON DELETE CASCADE
	);`

	// Course fee payment table
	coursePaymentTable := `
	CREATE TABLE IF NOT EXISTS course_payment (
		id SERIAL PRIMARY KEY,
		student_id INTEGER NOT NULL,
		course_id INTEGER NOT NULL,
		amount NUMERIC(10, 2) NOT NULL,
		status VARCHAR(50) DEFAULT 'PENDING',
		order_id VARCHAR(255) UNIQUE,
		payment_id VARCHAR(255),
		razorpay_sign TEXT,
		timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

		CONSTRAINT fk_student_course_payment
			FOREIGN KEY (student_id)
			REFERENCES student_lead(id)
			ON DELETE CASCADE,
		CONSTRAINT fk_course_payment_course
			FOREIGN KEY (course_id)
			REFERENCES course(id)
			ON DELETE CASCADE,
		CONSTRAINT unique_student_course
			UNIQUE(student_id, course_id)
	);`

	// Create counselor table first (referenced by student_lead)
	if _, err := DB.Exec(counselorTable); err != nil {
		return fmt.Errorf("error creating counselor table: %w", err)
	}

	// Create course table
	if _, err := DB.Exec(courseTable); err != nil {
		return fmt.Errorf("error creating course table: %w", err)
	}

	// Create student_lead table
	if _, err := DB.Exec(leadTable); err != nil {
		return fmt.Errorf("error creating student_lead table: %w", err)
	}

	// Create registration_payment table
	if _, err := DB.Exec(registrationPaymentTable); err != nil {
		return fmt.Errorf("error creating registration_payment table: %w", err)
	}

	// Create course_payment table
	if _, err := DB.Exec(coursePaymentTable); err != nil {
		return fmt.Errorf("error creating course_payment table: %w", err)
	}

	// Apply schema migrations
	if err := applyMigrations(); err != nil {
		log.Printf("Warning: Error applying migrations: %v", err)
	}

	return nil
}

func applyMigrations() error {
	// Drop old payment_status column if it exists
	DB.Exec(`ALTER TABLE student_lead DROP COLUMN IF EXISTS payment_status;`)

	// Add new payment status columns if they don't exist
	DB.Exec(`ALTER TABLE student_lead ADD COLUMN IF NOT EXISTS registration_fee_status VARCHAR(50) DEFAULT 'PENDING';`)
	DB.Exec(`ALTER TABLE student_lead ADD COLUMN IF NOT EXISTS course_fee_status VARCHAR(50) DEFAULT 'PENDING';`)

	// Create performance indexes
	indexQueries := []string{
		`CREATE INDEX IF NOT EXISTS idx_student_lead_registration_fee_status ON student_lead(registration_fee_status);`,
		`CREATE INDEX IF NOT EXISTS idx_student_lead_course_fee_status ON student_lead(course_fee_status);`,
		`CREATE INDEX IF NOT EXISTS idx_student_lead_interview_scheduled ON student_lead(interview_scheduled_at);`,
		`CREATE INDEX IF NOT EXISTS idx_registration_payment_student_id ON registration_payment(student_id);`,
		`CREATE INDEX IF NOT EXISTS idx_registration_payment_order_id ON registration_payment(order_id);`,
		`CREATE INDEX IF NOT EXISTS idx_course_payment_student_id ON course_payment(student_id);`,
		`CREATE INDEX IF NOT EXISTS idx_course_payment_order_id ON course_payment(order_id);`,
	}

	for _, query := range indexQueries {
		DB.Exec(query)
	}

	return nil
}
