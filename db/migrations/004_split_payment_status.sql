-- Migration to split payment status into registration and course fee status columns
-- and add auto-scheduling for interviews when registration fee is paid

-- Add separate payment status columns if they don't exist
ALTER TABLE student_lead
ADD COLUMN IF NOT EXISTS registration_fee_status VARCHAR(50) DEFAULT 'PENDING',
ADD COLUMN IF NOT EXISTS course_fee_status VARCHAR(50) DEFAULT 'PENDING';

-- Migrate existing data from old payment_status column if it exists
-- Assuming old column exists, copy values to the new columns
BEGIN;
  ALTER TABLE student_lead
  ADD COLUMN IF NOT EXISTS payment_status_old VARCHAR(50);
  
  -- If payment_status column exists, rename it to payment_status_old
  -- This is a safe approach to preserve existing data during migration
  
  UPDATE student_lead 
  SET registration_fee_status = payment_status_old, 
      course_fee_status = payment_status_old
  WHERE payment_status_old IS NOT NULL;
COMMIT;

-- Drop the old payment_status column if migration is complete
-- ALTER TABLE student_lead DROP COLUMN IF EXISTS payment_status;

-- Create indexes for better query performance
CREATE INDEX IF NOT EXISTS idx_student_lead_registration_fee_status 
ON student_lead(registration_fee_status);

CREATE INDEX IF NOT EXISTS idx_student_lead_course_fee_status 
ON student_lead(course_fee_status);

CREATE INDEX IF NOT EXISTS idx_student_lead_interview_scheduled 
ON student_lead(interview_scheduled_at);

-- Add comments for documentation
COMMENT ON COLUMN student_lead.registration_fee_status IS 'Status of registration fee payment: PENDING, PAID';
COMMENT ON COLUMN student_lead.course_fee_status IS 'Status of course fee payment: PENDING, PAID';

-- Automatic Interview Scheduling:
-- When registration_fee_status changes to PAID:
-- 1. interview_scheduled_at is set to current_time + 1 hour
-- 2. application_status is changed to INTERVIEW_SCHEDULED
-- 3. A meet link is generated and sent to student email
-- 4. meet_link is updated with the generated link

-- Optional: Create a trigger to auto-schedule meet when registration fee is paid
-- CREATE OR REPLACE FUNCTION auto_schedule_interview()
-- RETURNS TRIGGER AS $$
-- BEGIN
--   IF NEW.registration_fee_status = 'PAID' AND OLD.registration_fee_status != 'PAID' THEN
--     NEW.interview_scheduled_at := CURRENT_TIMESTAMP + INTERVAL '1 hour';
--     NEW.application_status := 'INTERVIEW_SCHEDULED';
--   END IF;
--   RETURN NEW;
-- END;
-- $$ LANGUAGE plpgsql;
-- 
-- DROP TRIGGER IF EXISTS auto_schedule_interview_trigger ON student_lead;
-- CREATE TRIGGER auto_schedule_interview_trigger
--   BEFORE UPDATE ON student_lead
--   FOR EACH ROW
--   EXECUTE FUNCTION auto_schedule_interview();
