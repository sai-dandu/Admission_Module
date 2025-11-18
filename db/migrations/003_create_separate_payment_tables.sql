-- Migration to create separate payment tables for registration and course fees
-- This separates payment concerns and improves data organization

-- Create registration_payment table
-- Stores registration fee payments (one per student)
CREATE TABLE IF NOT EXISTS registration_payment (
    id SERIAL PRIMARY KEY,
    student_id INTEGER NOT NULL UNIQUE,
    amount NUMERIC(10, 2) NOT NULL,
    status VARCHAR(50) DEFAULT 'PENDING',
    order_id VARCHAR(255) UNIQUE,
    payment_id VARCHAR(255),
    razorpay_sign TEXT,
    timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_student_reg_payment
        FOREIGN KEY (student_id)
        REFERENCES student_lead(id)
        ON DELETE CASCADE
);

-- Create course_payment table
-- Stores course fee payments (multiple per student, one per course)
CREATE TABLE IF NOT EXISTS course_payment (
    id SERIAL PRIMARY KEY,
    student_id INTEGER NOT NULL,
    course_id INTEGER NOT NULL,
    amount NUMERIC(10, 2) NOT NULL,
    status VARCHAR(50) DEFAULT 'PENDING',
    order_id VARCHAR(255) UNIQUE,
    payment_id VARCHAR(255),
    razorpay_sign TEXT,
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

-- Create indexes for better query performance
CREATE INDEX IF NOT EXISTS idx_registration_payment_student_id 
ON registration_payment(student_id);

CREATE INDEX IF NOT EXISTS idx_registration_payment_order_id 
ON registration_payment(order_id);

CREATE INDEX IF NOT EXISTS idx_registration_payment_status 
ON registration_payment(status);

CREATE INDEX IF NOT EXISTS idx_course_payment_student_id 
ON course_payment(student_id);

CREATE INDEX IF NOT EXISTS idx_course_payment_order_id 
ON course_payment(order_id);

CREATE INDEX IF NOT EXISTS idx_course_payment_status 
ON course_payment(status);

CREATE INDEX IF NOT EXISTS idx_course_payment_student_course 
ON course_payment(student_id, course_id);

-- Optional: Update student_lead to remove direct payment references if migrating data
-- ALTER TABLE student_lead DROP CONSTRAINT IF EXISTS fk_registration_payment;
-- ALTER TABLE student_lead DROP CONSTRAINT IF EXISTS fk_course_payment;
-- ALTER TABLE student_lead DROP COLUMN IF EXISTS registration_payment_id;
-- ALTER TABLE student_lead DROP COLUMN IF EXISTS course_payment_id;

-- Add comments for documentation
COMMENT ON TABLE registration_payment IS 'Stores registration fee payments. One record per student.';
COMMENT ON TABLE course_payment IS 'Stores course fee payments. Multiple records per student (one per course).';
COMMENT ON COLUMN registration_payment.student_id IS 'Foreign key to student_lead. UNIQUE constraint ensures one registration payment per student.';
COMMENT ON COLUMN course_payment.student_id IS 'Foreign key to student_lead.';
COMMENT ON COLUMN course_payment.course_id IS 'Foreign key to course.';
COMMENT ON COLUMN course_payment.unique_student_course IS 'Ensures a student cannot have duplicate course payments for the same course.';
