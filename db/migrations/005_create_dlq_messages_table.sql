-- Migration: 005_create_dlq_messages_table.sql
-- Description: Create a table to store messages that failed processing (Dead Letter Queue)

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

-- Create indexes for performance
CREATE INDEX IF NOT EXISTS idx_dlq_created_at ON dlq_messages(created_at);
CREATE INDEX IF NOT EXISTS idx_dlq_resolved ON dlq_messages(resolved);
CREATE INDEX IF NOT EXISTS idx_dlq_topic ON dlq_messages(topic);
CREATE INDEX IF NOT EXISTS idx_dlq_unresolved ON dlq_messages(resolved) WHERE resolved = FALSE;
