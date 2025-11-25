-- Migration 005: Add refund and error tracking columns to payment tables
-- This migration adds columns to track refunds and errors from Razorpay webhooks

ALTER TABLE registration_payment 
ADD COLUMN IF NOT EXISTS refund_id VARCHAR(255),
ADD COLUMN IF NOT EXISTS refund_amount NUMERIC(10, 2),
ADD COLUMN IF NOT EXISTS error_message TEXT;

ALTER TABLE course_payment 
ADD COLUMN IF NOT EXISTS refund_id VARCHAR(255),
ADD COLUMN IF NOT EXISTS refund_amount NUMERIC(10, 2),
ADD COLUMN IF NOT EXISTS error_message TEXT;

-- Create indexes on refund_id for quick lookups
CREATE INDEX IF NOT EXISTS idx_registration_payment_refund_id ON registration_payment(refund_id);
CREATE INDEX IF NOT EXISTS idx_course_payment_refund_id ON course_payment(refund_id);

-- Create indexes on status for filtering
CREATE INDEX IF NOT EXISTS idx_registration_payment_status ON registration_payment(status);
CREATE INDEX IF NOT EXISTS idx_course_payment_status ON course_payment(status);

