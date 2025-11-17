-- Migration to improve counsellor assignment system
-- Run this migration to support the refactored code

-- Add is_referral_enabled flag to counsellors table
-- This replaces the hardcoded "id >= 2" logic
ALTER TABLE counsellors 
ADD COLUMN IF NOT EXISTS is_referral_enabled BOOLEAN DEFAULT true;

-- Add comments for clarity
COMMENT ON COLUMN counsellors.is_referral_enabled IS 'Whether this counsellor can be assigned to referral leads';
COMMENT ON COLUMN counsellors.assigned_count IS 'Current number of leads assigned to this counsellor';
COMMENT ON COLUMN counsellors.max_capacity IS 'Maximum number of leads this counsellor can handle';

-- Create index for better query performance on lead lookups
CREATE INDEX IF NOT EXISTS idx_leads_email ON leads(email);
CREATE INDEX IF NOT EXISTS idx_leads_phone ON leads(phone);
CREATE INDEX IF NOT EXISTS idx_leads_created_at ON leads(created_at);
CREATE INDEX IF NOT EXISTS idx_leads_counsellor_id ON leads(counsellor_id);

-- Create index for counsellor assignment queries
CREATE INDEX IF NOT EXISTS idx_counsellors_assignment 
ON counsellors(assigned_count, id) 
WHERE assigned_count < max_capacity;

-- Update existing counsellors (customize based on your business logic)
-- Example: Set counsellor 1 to handle only website leads
UPDATE counsellors 
SET is_referral_enabled = false 
WHERE id = 1;

-- Add check constraint to ensure assigned_count doesn't exceed max_capacity
-- Note: This might fail if data is already inconsistent
-- ALTER TABLE counsellors 
-- ADD CONSTRAINT chk_capacity CHECK (assigned_count <= max_capacity);

-- Optional: Add audit columns for better tracking
ALTER TABLE leads 
ADD COLUMN IF NOT EXISTS last_modified_by VARCHAR(100),
ADD COLUMN IF NOT EXISTS notes TEXT;

-- Optional: Create a lead history table for tracking changes
CREATE TABLE IF NOT EXISTS lead_history (
    id SERIAL PRIMARY KEY,
    lead_id INTEGER NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    field_name VARCHAR(50) NOT NULL,
    old_value TEXT,
    new_value TEXT,
    changed_by VARCHAR(100),
    changed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_lead_history_lead_id ON lead_history(lead_id);
CREATE INDEX IF NOT EXISTS idx_lead_history_changed_at ON lead_history(changed_at);

-- Optional: Create a view for counsellor workload
CREATE OR REPLACE VIEW counsellor_workload AS
SELECT 
    c.id,
    c.name,
    c.assigned_count,
    c.max_capacity,
    ROUND((c.assigned_count::NUMERIC / c.max_capacity) * 100, 2) as capacity_percentage,
    c.max_capacity - c.assigned_count as available_slots,
    c.is_referral_enabled,
    COUNT(l.id) as actual_lead_count
FROM counsellors c
LEFT JOIN leads l ON l.counsellor_id = c.id
GROUP BY c.id, c.name, c.assigned_count, c.max_capacity, c.is_referral_enabled
ORDER BY capacity_percentage DESC;

COMMENT ON VIEW counsellor_workload IS 'Shows current workload distribution across counsellors';

-- Verify data consistency
-- Run this query to check if assigned_count matches actual leads
SELECT 
    c.id,
    c.name,
    c.assigned_count as recorded_count,
    COUNT(l.id) as actual_count,
    c.assigned_count - COUNT(l.id) as difference
FROM counsellors c
LEFT JOIN leads l ON l.counsellor_id = c.id
GROUP BY c.id, c.name, c.assigned_count
HAVING c.assigned_count != COUNT(l.id);

-- If there are inconsistencies, fix them with:
-- UPDATE counsellors c
-- SET assigned_count = (
--     SELECT COUNT(*) 
--     FROM leads l 
--     WHERE l.counsellor_id = c.id
-- );
