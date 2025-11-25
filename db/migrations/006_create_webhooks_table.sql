-- Migration 006: Create webhooks table for Razorpay webhook tracking
-- This table stores all incoming Razorpay webhooks for audit, debugging, and replay purposes

CREATE TABLE IF NOT EXISTS razorpay_webhooks (
    id SERIAL PRIMARY KEY,
    webhook_id VARCHAR(255) UNIQUE NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,
    status VARCHAR(50) DEFAULT 'RECEIVED',
    processed_at TIMESTAMP,
    error_message TEXT,
    retry_count INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for better query performance
CREATE INDEX IF NOT EXISTS idx_razorpay_webhooks_event_type 
ON razorpay_webhooks(event_type);

CREATE INDEX IF NOT EXISTS idx_razorpay_webhooks_status 
ON razorpay_webhooks(status);

CREATE INDEX IF NOT EXISTS idx_razorpay_webhooks_created_at 
ON razorpay_webhooks(created_at);

CREATE INDEX IF NOT EXISTS idx_razorpay_webhooks_webhook_id 
ON razorpay_webhooks(webhook_id);

-- Add comments for documentation
COMMENT ON TABLE razorpay_webhooks IS 'Stores all Razorpay webhook events for audit, debugging, and replay purposes';
COMMENT ON COLUMN razorpay_webhooks.webhook_id IS 'Unique webhook ID from Razorpay to prevent duplicate processing';
COMMENT ON COLUMN razorpay_webhooks.event_type IS 'Type of event (e.g., payment.authorized, payment.failed, refund.created)';
COMMENT ON COLUMN razorpay_webhooks.payload IS 'Complete JSON payload from Razorpay webhook';
COMMENT ON COLUMN razorpay_webhooks.status IS 'Processing status (RECEIVED, PROCESSING, PROCESSED, FAILED)';
COMMENT ON COLUMN razorpay_webhooks.processed_at IS 'Timestamp when the webhook was successfully processed';
COMMENT ON COLUMN razorpay_webhooks.error_message IS 'Error message if processing failed';
COMMENT ON COLUMN razorpay_webhooks.retry_count IS 'Number of retry attempts for failed webhooks';
