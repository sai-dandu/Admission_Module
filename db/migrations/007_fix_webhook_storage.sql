-- Migration 007: Fix webhook storage issues and add proper constraints

-- Drop the existing table if it has issues (backup first if needed)
-- This ensures a clean state for webhook storage
DROP TABLE IF EXISTS razorpay_webhooks CASCADE;

-- Recreate razorpay_webhooks table with proper constraints
CREATE TABLE razorpay_webhooks (
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

-- Create indexes for optimal query performance
CREATE INDEX IF NOT EXISTS idx_razorpay_webhooks_event_type 
ON razorpay_webhooks(event_type);

CREATE INDEX IF NOT EXISTS idx_razorpay_webhooks_status 
ON razorpay_webhooks(status);

CREATE INDEX IF NOT EXISTS idx_razorpay_webhooks_webhook_id 
ON razorpay_webhooks(webhook_id);

CREATE INDEX IF NOT EXISTS idx_razorpay_webhooks_created_at 
ON razorpay_webhooks(created_at DESC);

-- Add comments for documentation
COMMENT ON TABLE razorpay_webhooks IS 'Stores all Razorpay webhook events with proper constraint handling for idempotency';
COMMENT ON COLUMN razorpay_webhooks.webhook_id IS 'Unique webhook ID from Razorpay (UNIQUE constraint prevents duplicates)';
COMMENT ON COLUMN razorpay_webhooks.event_type IS 'Type of event (e.g., payment.authorized, payment.captured, payment.failed)';
COMMENT ON COLUMN razorpay_webhooks.payload IS 'Complete JSON payload from Razorpay webhook';
COMMENT ON COLUMN razorpay_webhooks.status IS 'Processing status (RECEIVED, PROCESSED, FAILED)';
COMMENT ON COLUMN razorpay_webhooks.signature_valid IS 'Whether the webhook signature was validated successfully';
COMMENT ON COLUMN razorpay_webhooks.retry_count IS 'Number of times this webhook was received (for duplicate detection)';
