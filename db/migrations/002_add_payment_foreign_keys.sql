-- Migration to add foreign key constraints for payment tracking
-- Run this migration to add foreign keys linking payment records to student_lead table

-- Add foreign key constraint for registration_payment_id
ALTER TABLE student_lead
ADD CONSTRAINT fk_registration_payment
FOREIGN KEY (registration_payment_id)
REFERENCES payment(id)
ON DELETE SET NULL;

-- Add foreign key constraint for course_payment_id
ALTER TABLE student_lead
ADD CONSTRAINT fk_course_payment
FOREIGN KEY (course_payment_id)
REFERENCES payment(id)
ON DELETE SET NULL;

-- Create indexes for better query performance on payment lookups
CREATE INDEX IF NOT EXISTS idx_student_lead_registration_payment_id 
ON student_lead(registration_payment_id);

CREATE INDEX IF NOT EXISTS idx_student_lead_course_payment_id 
ON student_lead(course_payment_id);

-- Optional: Add constraint to ensure payment_type matches the payment being stored
-- This ensures registration payments are stored in registration_payment_id
-- and course payments are stored in course_payment_id
-- Note: This would require a trigger or application-level validation

COMMENT ON COLUMN student_lead.registration_payment_id IS 'Foreign key to payment table for registration fee payment';
COMMENT ON COLUMN student_lead.course_payment_id IS 'Foreign key to payment table for course fee payment';
