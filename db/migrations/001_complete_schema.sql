-- ============================================
-- Complete Schema Migration - All tables
-- ============================================
-- This migration consolidates all schema setup:
-- - Base tables (counselor, course, student_lead)
-- - Payment tables (registration_payment, course_payment)
-- - DLQ messages for failed event processing
-- - Webhook audit and tracking

-- ============================================
-- 1. BASE TABLES
-- ============================================

-- Counselor table
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
);

-- Course table
CREATE TABLE IF NOT EXISTS course (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    fee NUMERIC(10, 2) NOT NULL,
    duration VARCHAR(100),
    is_active INTEGER DEFAULT 1,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Student Lead table
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
);

-- ============================================
-- 2. PAYMENT TABLES
-- ============================================

-- Registration Payment table
CREATE TABLE IF NOT EXISTS registration_payment (
    id SERIAL PRIMARY KEY,
    student_id INTEGER NOT NULL UNIQUE,
    amount NUMERIC(10, 2) NOT NULL,
    status VARCHAR(50) DEFAULT 'PENDING',
    order_id VARCHAR(255) UNIQUE,
    payment_id VARCHAR(255),
    razorpay_sign TEXT,
    error_message TEXT,
    timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_student_reg_payment
        FOREIGN KEY (student_id)
        REFERENCES student_lead(id)
        ON DELETE CASCADE
);

-- Course Payment table
CREATE TABLE IF NOT EXISTS course_payment (
    id SERIAL PRIMARY KEY,
    student_id INTEGER NOT NULL,
    course_id INTEGER NOT NULL,
    amount NUMERIC(10, 2) NOT NULL,
    status VARCHAR(50) DEFAULT 'PENDING',
    order_id VARCHAR(255) UNIQUE,
    payment_id VARCHAR(255),
    razorpay_sign TEXT,
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
);

-- ============================================
-- 3. MESSAGE QUEUE TABLES
-- ============================================

-- DLQ Messages table for failed event processing
CREATE TABLE IF NOT EXISTS dlq_messages (
    id SERIAL PRIMARY KEY,
    message_id UUID UNIQUE,
    topic VARCHAR(255) NOT NULL,
    key TEXT,
    value JSONB NOT NULL,
    error_message TEXT,
    retry_count INT DEFAULT 0,
    max_retries INT DEFAULT 3,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_retry_at TIMESTAMP,
    resolved BOOLEAN DEFAULT FALSE,
    resolved_at TIMESTAMP,
    notes TEXT
);

-- ============================================
-- 4. WEBHOOK TABLES
-- ============================================

-- Razorpay Webhooks table for audit and tracking
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
);

-- ============================================
-- 5. INDEXES FOR PERFORMANCE
-- ============================================

-- Student Lead indexes
CREATE INDEX IF NOT EXISTS idx_student_lead_email ON student_lead(email);
CREATE INDEX IF NOT EXISTS idx_student_lead_phone ON student_lead(phone);
CREATE INDEX IF NOT EXISTS idx_student_lead_created_at ON student_lead(created_at);
CREATE INDEX IF NOT EXISTS idx_student_lead_counselor_id ON student_lead(counselor_id);

-- Counselor indexes
CREATE INDEX IF NOT EXISTS idx_counselor_assignment 
ON counselor(assigned_count, id) 
WHERE assigned_count < max_capacity;

-- Payment indexes
CREATE INDEX IF NOT EXISTS idx_registration_payment_student ON registration_payment(student_id);
CREATE INDEX IF NOT EXISTS idx_registration_payment_order ON registration_payment(order_id);
CREATE INDEX IF NOT EXISTS idx_course_payment_student ON course_payment(student_id);
CREATE INDEX IF NOT EXISTS idx_course_payment_course ON course_payment(course_id);
CREATE INDEX IF NOT EXISTS idx_course_payment_order ON course_payment(order_id);

-- DLQ indexes
CREATE INDEX IF NOT EXISTS idx_dlq_created_at ON dlq_messages(created_at);
CREATE INDEX IF NOT EXISTS idx_dlq_resolved ON dlq_messages(resolved);
CREATE INDEX IF NOT EXISTS idx_dlq_topic ON dlq_messages(topic);
CREATE INDEX IF NOT EXISTS idx_dlq_unresolved ON dlq_messages(resolved) WHERE resolved = FALSE;

-- Webhook indexes
CREATE INDEX IF NOT EXISTS idx_razorpay_webhooks_event_type 
ON razorpay_webhooks(event_type);

CREATE INDEX IF NOT EXISTS idx_razorpay_webhooks_status 
ON razorpay_webhooks(status);

CREATE INDEX IF NOT EXISTS idx_razorpay_webhooks_webhook_id 
ON razorpay_webhooks(webhook_id);

CREATE INDEX IF NOT EXISTS idx_razorpay_webhooks_created_at 
ON razorpay_webhooks(created_at DESC);

-- ============================================
-- 6. COMMENTS FOR DOCUMENTATION
-- ============================================

COMMENT ON TABLE counselor IS 'Admission counselors who guide and manage student leads';
COMMENT ON TABLE course IS 'Educational programs offered by the institution';
COMMENT ON TABLE student_lead IS 'Student applicants and their admission progress';
COMMENT ON TABLE registration_payment IS 'Registration fee payments from students';
COMMENT ON TABLE course_payment IS 'Course-specific fee payments';
COMMENT ON TABLE dlq_messages IS 'Dead Letter Queue for messages that failed event processing';
COMMENT ON TABLE razorpay_webhooks IS 'Audit log of all Razorpay webhook events';

COMMENT ON COLUMN counselor.is_referral_enabled IS 'Whether this counselor can be assigned to referral leads';
COMMENT ON COLUMN student_lead.registration_fee_status IS 'Status of registration fee payment (PENDING, PAID)';
COMMENT ON COLUMN student_lead.course_fee_status IS 'Status of course fee payment (PENDING, PAID)';
COMMENT ON COLUMN razorpay_webhooks.webhook_id IS 'Unique webhook ID from Razorpay to prevent duplicate processing';
COMMENT ON COLUMN razorpay_webhooks.signature_valid IS 'Whether the webhook signature was validated successfully';
