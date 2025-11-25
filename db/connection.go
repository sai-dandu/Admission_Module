package db

import (
	"admission-module/config"
	"database/sql"
	"fmt"
	"log"
	"time"

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
		refund_id VARCHAR(255),
		refund_amount NUMERIC(10, 2),
		error_message TEXT,
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
		refund_id VARCHAR(255),
		refund_amount NUMERIC(10, 2),
		error_message TEXT,
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

	// Razorpay webhook logs table
	webhookLogsTable := `
	CREATE TABLE IF NOT EXISTS razorpay_webhook_logs (
		id BIGSERIAL PRIMARY KEY,
		webhook_id VARCHAR(255) UNIQUE NOT NULL,
		event_type VARCHAR(100) NOT NULL,
		order_id VARCHAR(255),
		payment_id VARCHAR(255),
		refund_id VARCHAR(255),
		student_id INTEGER,
		amount_paise BIGINT,
		currency VARCHAR(10),
		status VARCHAR(50),
		payload JSONB NOT NULL,
		signature VARCHAR(255),
		signature_valid BOOLEAN,
		processing_status VARCHAR(50) DEFAULT 'PENDING',
		processed_at TIMESTAMP,
		error_message TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);`

	// Razorpay webhooks table
	razorpayWebhooksTable := `
	CREATE TABLE IF NOT EXISTS razorpay_webhooks (
		id SERIAL PRIMARY KEY,
		webhook_id VARCHAR(255) UNIQUE NOT NULL,
		event_type VARCHAR(100) NOT NULL,
		payload JSONB NOT NULL,
		status VARCHAR(50) DEFAULT 'RECEIVED',
		processed_at TIMESTAMP,
		error_message TEXT,
		retry_count INTEGER DEFAULT 0,
		signature_valid BOOLEAN DEFAULT false,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
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

	// Create webhook logs table
	if _, err := DB.Exec(webhookLogsTable); err != nil {
		return fmt.Errorf("error creating razorpay_webhook_logs table: %w", err)
	}

	// Create razorpay_webhooks table - with UNIQUE constraint on webhook_id
	if _, err := DB.Exec(razorpayWebhooksTable); err != nil {
		return fmt.Errorf("error creating razorpay_webhooks table: %w", err)
	}

	// Apply schema migrations
	if err := applyMigrations(); err != nil {
		log.Printf("Warning: Error applying migrations: %v", err)
	}
	log.Println("[DB] ✓ Migrations applied")

	// Insert default dummy data if empty
	if err := insertDefaultData(); err != nil {
		log.Printf("Warning: Error inserting default data: %v", err)
	}

	return nil
}

func applyMigrations() error {
	// Drop old payment_status column if it exists
	DB.Exec(`ALTER TABLE student_lead DROP COLUMN IF EXISTS payment_status;`)

	// Add new payment status columns if they don't exist
	DB.Exec(`ALTER TABLE student_lead ADD COLUMN IF NOT EXISTS registration_fee_status VARCHAR(50) DEFAULT 'PENDING';`)
	DB.Exec(`ALTER TABLE student_lead ADD COLUMN IF NOT EXISTS course_fee_status VARCHAR(50) DEFAULT 'PENDING';`)

	// Add refund tracking columns to payment tables
	DB.Exec(`ALTER TABLE registration_payment ADD COLUMN IF NOT EXISTS refund_id VARCHAR(255);`)
	DB.Exec(`ALTER TABLE registration_payment ADD COLUMN IF NOT EXISTS refund_amount NUMERIC(10, 2);`)
	DB.Exec(`ALTER TABLE course_payment ADD COLUMN IF NOT EXISTS refund_id VARCHAR(255);`)
	DB.Exec(`ALTER TABLE course_payment ADD COLUMN IF NOT EXISTS refund_amount NUMERIC(10, 2);`)

	// Create performance indexes
	indexQueries := []string{
		`CREATE INDEX IF NOT EXISTS idx_student_lead_registration_fee_status ON student_lead(registration_fee_status);`,
		`CREATE INDEX IF NOT EXISTS idx_student_lead_course_fee_status ON student_lead(course_fee_status);`,
		`CREATE INDEX IF NOT EXISTS idx_student_lead_interview_scheduled ON student_lead(interview_scheduled_at);`,
		`CREATE INDEX IF NOT EXISTS idx_registration_payment_student_id ON registration_payment(student_id);`,
		`CREATE INDEX IF NOT EXISTS idx_registration_payment_order_id ON registration_payment(order_id);`,
		`CREATE INDEX IF NOT EXISTS idx_course_payment_student_id ON course_payment(student_id);`,
		`CREATE INDEX IF NOT EXISTS idx_course_payment_order_id ON course_payment(order_id);`,
		`CREATE INDEX IF NOT EXISTS idx_razorpay_webhook_logs_event_type ON razorpay_webhook_logs(event_type);`,
		`CREATE INDEX IF NOT EXISTS idx_razorpay_webhook_logs_order_id ON razorpay_webhook_logs(order_id);`,
		`CREATE INDEX IF NOT EXISTS idx_razorpay_webhook_logs_payment_id ON razorpay_webhook_logs(payment_id);`,
		`CREATE INDEX IF NOT EXISTS idx_razorpay_webhook_logs_student_id ON razorpay_webhook_logs(student_id);`,
		`CREATE INDEX IF NOT EXISTS idx_razorpay_webhook_logs_processing_status ON razorpay_webhook_logs(processing_status);`,
		`CREATE INDEX IF NOT EXISTS idx_razorpay_webhook_logs_created_at ON razorpay_webhook_logs(created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_razorpay_webhooks_event_type ON razorpay_webhooks(event_type);`,
		`CREATE INDEX IF NOT EXISTS idx_razorpay_webhooks_status ON razorpay_webhooks(status);`,
		`CREATE INDEX IF NOT EXISTS idx_razorpay_webhooks_webhook_id ON razorpay_webhooks(webhook_id);`,
		`CREATE INDEX IF NOT EXISTS idx_razorpay_webhooks_created_at ON razorpay_webhooks(created_at DESC);`,
	}

	for _, query := range indexQueries {
		DB.Exec(query)
	}

	return nil
}

func insertDefaultData() error {
	now := time.Now().Format("2006-01-02 15:04:05")

	// Check if counselors exist
	var counselorCount int
	err := DB.QueryRow("SELECT COUNT(*) FROM counselor").Scan(&counselorCount)
	if err != nil {
		return fmt.Errorf("error checking counselor count: %w", err)
	}

	if counselorCount == 0 {
		log.Println("[DB] No counselors found, inserting default counselors...")
		counselorQueries := []string{
			fmt.Sprintf(`INSERT INTO counselor (name, email, phone, assigned_count, max_capacity, is_referral_enabled, created_at, updated_at) 
				VALUES ('Dr. Rishi Kumar', 'rishi@university.edu', '+919876543210', 0, 15, true, '%s', '%s')`, now, now),
			fmt.Sprintf(`INSERT INTO counselor (name, email, phone, assigned_count, max_capacity, is_referral_enabled, created_at, updated_at) 
				VALUES ('Prof. Priya Sharma', 'priya@university.edu', '+919876543211', 0, 15, false, '%s', '%s')`, now, now),
			fmt.Sprintf(`INSERT INTO counselor (name, email, phone, assigned_count, max_capacity, is_referral_enabled, created_at, updated_at) 
				VALUES ('Ms. Anjali Verma', 'anjali@university.edu', '+919876543212', 0, 12, true, '%s', '%s')`, now, now),
		}

		for _, query := range counselorQueries {
			if _, err := DB.Exec(query); err != nil {
				log.Printf("[DB] Warning: Error inserting counselor: %v", err)
			}
		}
	}

	// Check if courses exist
	var courseCount int
	err = DB.QueryRow("SELECT COUNT(*) FROM course").Scan(&courseCount)
	if err != nil {
		return fmt.Errorf("error checking course count: %w", err)
	}

	if courseCount == 0 {
		log.Println("[DB] No courses found, inserting default courses...")
		courseQueries := []string{
			fmt.Sprintf(`INSERT INTO course (name, description, fee, duration, is_active, created_at, updated_at) 
				VALUES ('B.Tech Computer Science', 'Bachelor of Technology in Computer Science - 4 year program covering programming, databases, and software development', 125000.00, '4 Years', 1, '%s', '%s')`, now, now),
			fmt.Sprintf(`INSERT INTO course (name, description, fee, duration, is_active, created_at, updated_at) 
				VALUES ('M.Tech Information Technology', 'Master of Technology in IT - 2 year advanced program with specializations in AI, Cloud Computing', 75000.00, '2 Years', 1, '%s', '%s')`, now, now),
			fmt.Sprintf(`INSERT INTO course (name, description, fee, duration, is_active, created_at, updated_at) 
				VALUES ('MBA Business Administration', 'Master of Business Administration - 2 year program focusing on management, finance, and entrepreneurship', 200000.00, '2 Years', 1, '%s', '%s')`, now, now),
			fmt.Sprintf(`INSERT INTO course (name, description, fee, duration, is_active, created_at, updated_at) 
				VALUES ('B.S Electronics Engineering', 'Bachelor of Science in Electronics Engineering - 4 year program with focus on circuit design and embedded systems', 110000.00, '4 Years', 1, '%s', '%s')`, now, now),
			fmt.Sprintf(`INSERT INTO course (name, description, fee, duration, is_active, created_at, updated_at) 
				VALUES ('Diploma Data Science', 'Diploma in Data Science - 1 year intensive program covering analytics, machine learning, and big data', 50000.00, '1 Year', 1, '%s', '%s')`, now, now),
		}

		for _, query := range courseQueries {
			if _, err := DB.Exec(query); err != nil {
				log.Printf("[DB] Warning: Error inserting course: %v", err)
			}
		}
	}

	return nil
}
