-- Migration: Add new fields to student_lead table and create course table
-- This migration is automatically applied on server startup via db/connection.go

-- Step 1: Create Course Table
CREATE TABLE IF NOT EXISTS course (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    fee NUMERIC(10, 2) NOT NULL,
    duration TEXT,
    is_active INTEGER DEFAULT 1,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Step 2: Add new columns to student_lead table
-- Note: These are added automatically if not exist
-- ALTER TABLE student_lead ADD COLUMN IF NOT EXISTS registration_payment_id INTEGER;
-- ALTER TABLE student_lead ADD COLUMN IF NOT EXISTS selected_course_id INTEGER;
-- ALTER TABLE student_lead ADD COLUMN IF NOT EXISTS course_payment_id INTEGER;
-- ALTER TABLE student_lead ADD COLUMN IF NOT EXISTS interview_scheduled_at TIMESTAMP;

-- Step 3: Add foreign key for selected course
-- ALTER TABLE student_lead ADD CONSTRAINT fk_selected_course
-- FOREIGN KEY (selected_course_id) REFERENCES course(id) ON DELETE SET NULL;

-- Step 4: Update Payment table schema
-- ALTER TABLE payment ADD COLUMN IF NOT EXISTS payment_type TEXT DEFAULT 'REGISTRATION';
-- ALTER TABLE payment ADD COLUMN IF NOT EXISTS related_course_id INTEGER;
-- ALTER TABLE payment ADD CONSTRAINT fk_related_course
-- FOREIGN KEY (related_course_id) REFERENCES course(id) ON DELETE SET NULL;

-- Step 5: Update counselor table
-- ALTER TABLE counselor ADD COLUMN IF NOT EXISTS is_referral_enabled BOOLEAN DEFAULT false;

-- Insert sample courses
INSERT INTO course (name, description, fee, duration) 
SELECT 'Bachelor of Computer Science', 'Learn programming and software development', 150000, '4 Years' 
WHERE NOT EXISTS (SELECT 1 FROM course WHERE name = 'Bachelor of Computer Science');

INSERT INTO course (name, description, fee, duration) 
SELECT 'Master of Business Administration', 'Advanced business management program', 200000, '2 Years' 
WHERE NOT EXISTS (SELECT 1 FROM course WHERE name = 'Master of Business Administration');

-- Insert sample counselors
INSERT INTO counselor (name, email, is_referral_enabled) 
SELECT 'Counselor 1', 'c1@example.com', false 
WHERE NOT EXISTS (SELECT 1 FROM counselor WHERE name = 'Counselor 1');

INSERT INTO counselor (name, email, is_referral_enabled) 
SELECT 'Counselor 2', 'c2@example.com', true 
WHERE NOT EXISTS (SELECT 1 FROM counselor WHERE name = 'Counselor 2');

-- Final schema verification queries
-- SELECT * FROM information_schema.tables WHERE table_name IN ('student_lead', 'course', 'payment', 'counselor');
-- SELECT * FROM information_schema.columns WHERE table_name = 'student_lead';
-- SELECT * FROM information_schema.columns WHERE table_name = 'payment';
