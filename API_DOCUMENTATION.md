# Admission Module - Complete API Reference

## Overview

**Base URL:** `http://localhost:8080`  
**Version:** 2.2.0  
**Last Updated:** November 26, 2025 (Payment Restrictions Update)  
**Go Version:** 1.24  
**Database:** PostgreSQL 12+  
**Message Broker:** Apache Kafka  

The Admission Module is a production-ready REST API for managing student admissions with real-time event processing through Kafka.

---

## Table of Contents
1. [Project Structure](#project-structure)
2. [Configuration](#configuration)
3. [Database Schema](#database-schema)
4. [API Endpoints](#api-endpoints)
5. [Email System (Kafka)](#email-system-kafka)
6. [Payment Processing](#payment-processing)
7. [Error Handling](#error-handling)
8. [Testing](#testing)

---

## Project Structure

### Directory Layout
```
admission-module/
├── cmd/server/main.go               # Server entry & Kafka setup
├── config/config.go                 # Environment configuration
├── db/
│   ├── connection.go                # Database connection
│   └── migrations/001_*.sql         # Schema definition
├── http/
│   ├── http.go                      # Server & middleware
│   └── handlers/                    # API endpoints
│       ├── lead.go                  # Lead management
│       ├── payment.go               # Payment endpoints
│       ├── course.go                # Course management
│       ├── meet.go                  # Meeting scheduling
│       ├── review.go                # Application decisions
│       └── dlq.go                   # DLQ management
├── services/                        # Business logic
│   ├── email.go                     # Email → Kafka publisher
│   ├── email_sender.go              # SMTP sender (consumer only)
│   ├── notification.go              # Welcome/assignment emails
│   ├── application.go               # Accept/reject logic
│   ├── google_meet.go               # Meet link generation
│   ├── payment.go                   # Payment logic
│   ├── webhook.go                   # Razorpay webhook
│   ├── excel.go                     # Excel parsing
│   ├── kafka_wrapper.go             # Kafka wrapper functions
│   └── kafka/
│       ├── producer.go              # Event publishing
│       ├── consumer.go              # Event consuming
│       └── connect.go               # DLQ management
├── models/                          # Data structures
├── utils/                           # Utility functions
└── logger/logger.go                 # Logging
```

---

## Configuration

### Environment Variables

Create `.env` in project root:

```env
# Database
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=your_password
DB_NAME=admission_db

# Razorpay (Test Credentials)
RazorpayKeyID=rzp_test_xxxxx
RazorpayKeySecret=your_secret_key

# Email (SMTP)
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USER=your_email@gmail.com
SMTP_PASS=your_app_password
EMAIL_FROM=noreply@admission-module.com

# Kafka (Optional - leave empty to disable)
KAFKA_BROKERS=localhost:9092

# Server
SERVER_PORT=8080
```

### Credential Setup

**Gmail SMTP Password:**
1. Enable 2-Factor Authentication
2. Go to https://myaccount.google.com/apppasswords
3. Select "Mail" and "Windows Computer"
4. Copy 16-character password to `SMTP_PASS`

**Razorpay Test Credentials:**
- Go to https://dashboard.razorpay.com/
- Navigate to Settings → API Keys
- Copy Key ID and Key Secret (test mode)

---

## Database Schema

### Tables

#### 1. `student_lead` - Primary Student Record
```sql
CREATE TABLE student_lead (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) NOT NULL UNIQUE,
    phone VARCHAR(20) NOT NULL UNIQUE,
    education VARCHAR(255),
    lead_source VARCHAR(100),
    counselor_id INTEGER REFERENCES counselor(id),
    registration_fee_status VARCHAR(50) DEFAULT 'PENDING',
    course_fee_status VARCHAR(50) DEFAULT 'PENDING',
    meet_link TEXT,
    application_status VARCHAR(50) DEFAULT 'NEW',
    selected_course_id INTEGER REFERENCES course(id),
    interview_scheduled_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

#### 2. `registration_payment` - Registration Fees
```sql
CREATE TABLE registration_payment (
    id SERIAL PRIMARY KEY,
    student_id INTEGER NOT NULL UNIQUE REFERENCES student_lead(id) ON DELETE CASCADE,
    amount NUMERIC(10, 2) NOT NULL,
    status VARCHAR(50) DEFAULT 'PENDING',
    order_id VARCHAR(255) UNIQUE,
    payment_id VARCHAR(255),
    razorpay_sign TEXT,
    error_message TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

#### 3. `course_payment` - Course Fees
```sql
CREATE TABLE course_payment (
    id SERIAL PRIMARY KEY,
    student_id INTEGER NOT NULL REFERENCES student_lead(id) ON DELETE CASCADE,
    course_id INTEGER NOT NULL REFERENCES course(id) ON DELETE CASCADE,
    amount NUMERIC(10, 2) NOT NULL,
    status VARCHAR(50) DEFAULT 'PENDING',
    order_id VARCHAR(255) UNIQUE,
    payment_id VARCHAR(255),
    razorpay_sign TEXT,
    error_message TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT unique_student_course UNIQUE(student_id, course_id)
);
```

#### 4. `counselor` - Staff Profiles
```sql
CREATE TABLE counselor (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) NOT NULL UNIQUE,
    phone VARCHAR(20),
    assigned_count INTEGER DEFAULT 0,
    max_capacity INTEGER DEFAULT 10,
    is_referral_enabled BOOLEAN DEFAULT false,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

#### 5. `course` - Available Programs
```sql
CREATE TABLE course (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    fee NUMERIC(10, 2) NOT NULL,
    duration VARCHAR(100),
    is_active INTEGER DEFAULT 1,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

#### 6. `dlq_messages` - Dead Letter Queue
```sql
CREATE TABLE dlq_messages (
    id SERIAL PRIMARY KEY,
    original_topic VARCHAR(255),
    message_key VARCHAR(255),
    message_value TEXT,
    error_message TEXT,
    status VARCHAR(50) DEFAULT 'FAILED',
    retry_count INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

---

## API Endpoints

### Response Format

**Success Response:**
```json
{
  "status": "success",
  "message": "Operation completed",
  "data": {}
}
```

**Error Response:**
```json
{
  "status": "error",
  "error": "Error description"
}
```

---

## Lead Management

### 1. Create Lead
**POST** `/create-lead`

Creates a new student lead and auto-assigns counselor.

**Request:**
```json
{
  "name": "John Doe",
  "email": "john@example.com",
  "phone": "+919876543210",
  "education": "B.Tech Computer Science",
  "lead_source": "website"
}
```

**Response (201):**
```json
{
  "status": "success",
  "message": "Lead created successfully",
  "data": {
    "student_id": 1,
    "counselor_name": "Rishi",
    "email": "john@example.com"
  }
}
```

**Validation:**
- **name:** Required, 1-255 characters
- **email:** Required, valid format, globally unique
- **phone:** Required, E.164 format (+919876543210), globally unique
- **education:** Optional, max 255 characters
- **lead_source:** Optional, "website" or "referral"

**Emails Sent:**
- Welcome email to student
- Counselor assignment notification

---

### 2. Get All Leads
**GET** `/leads`

Retrieves all leads with optional date filtering.

**Query Parameters:**
- `created_after` (optional): RFC3339 format
- `created_before` (optional): RFC3339 format

**Response (200):**
```json
{
  "status": "success",
  "message": "Retrieved 5 leads",
  "count": 5,
  "data": [
    {
      "id": 1,
      "name": "John Doe",
      "email": "john@example.com",
      "phone": "+919876543210",
      "education": "B.Tech",
      "lead_source": "website",
      "counselor_name": "Rishi",
      "meet_link": "https://meet.google.com/abc-defg-hij",
      "application_status": "NEW",
      "created_at": "2025-11-17T15:05:34Z"
    }
  ]
}
```

---

### 3. Upload Leads (Bulk)
**POST** `/upload-leads`

Upload leads from Excel file (.xlsx).

**Request:**
- Content-Type: `multipart/form-data`
- Field: `file` (Excel file)

**Excel Format:**
| name | email | phone | education | lead_source |
|------|-------|-------|-----------|-------------|
| John Doe | john@example.com | +919876543210 | B.Tech | website |

**Response (200):**
```json
{
  "status": "success",
  "message": "Uploaded 100 leads successfully",
  "data": {
    "total_count": 100,
    "success_count": 98,
    "failed_count": 2,
    "failed_leads": [
      {
        "row": 5,
        "email": "duplicate@example.com",
        "error": "lead already exists"
      }
    ]
  }
}
```

---

## Payment Management

### Payment Types & Restrictions

| Aspect | Registration | Course Fee |
|--------|--------------|-----------|
| Amount | Fixed ₹1,870 | Variable (per course) |
| Trigger | Student registration | Course enrollment |
| When PAID | Auto-schedule interview | Store course selection |
| Database Table | registration_payment | course_payment |
| **Restriction** | None | ⚠️ **REQUIRED:** Registration fee must be PAID first |

### Payment Flow & Business Rules

**Critical Restrictions Implemented:**
1. **Course Fee Payment Restriction** ✅
   - Student CANNOT pay course fee until registration fee status is `PAID`
   - Error: `"Registration payment status is PENDING/FAILED. Please complete registration fee payment before proceeding with course fee payment"`

2. **Interview Scheduling Restriction** ✅
   - System CANNOT schedule interview unless registration fee is `PAID`
   - Error: `"Interview cannot be scheduled. Registration payment status is PENDING/FAILED. Please complete registration fee payment first"`

3. **Application Action Restriction** ✅
   - Application status updates (Accept/Reject) ONLY allowed after registration fee is `PAID`
   - Error: `"Application status cannot be updated. Registration payment status is PENDING/FAILED. Please complete registration fee payment first"`

### 1. Initiate Payment
**POST** `/initiate-payment`

Creates Razorpay order for payment.

**Payment Type Requirements:**
- `REGISTRATION`: No prerequisites
- `COURSE_FEE`: Registration fee must be `PAID` ⚠️ (enforced by API)

**Request (Registration):**
```json
{
  "student_id": 1,
  "payment_type": "REGISTRATION"
}
```

**Request (Course Fee):**
```json
{
  "student_id": 1,
  "payment_type": "COURSE_FEE",
  "course_id": 2
}
```

**Response (200):**
```json
{
  "status": "success",
  "message": "Payment order created successfully",
  "data": {
    "order_id": "order_Rh9Vc899yylv78",
    "amount": 1870.0,
    "currency": "INR",
    "receipt": "rcpt_1_REGISTRATION",
    "payment_type": "REGISTRATION",
    "student_id": 1,
    "message": "Please complete the payment using Razorpay"
  }
}
```

**Error (400) - If course fee requested but registration fee not PAID:**
```json
{
  "status": "error",
  "error": "Registration payment status is PENDING. Please complete registration fee payment before proceeding with course fee payment"
}
```

---

### 2. Verify Payment
**POST** `/verify-payment`

Verifies Razorpay signature. Database is updated ONLY when webhook arrives from Razorpay.

**Request:**
```json
{
  "order_id": "order_Rh9Vc899yylv78",
  "payment_id": "pay_Rh9Vc899yylv78",
  "razorpay_signature": "9ef4dffbfd84f1318f6739a3ce19f9d85851857ae648f114332d8401e0949a3d"
}
```

**Response (200) - Signature Valid:**
```json
{
  "status": "success",
  "message": "Payment verified successfully",
  "data": {
    "status": "success"
  }
}
```

⚠️ **Important Note:**
- This endpoint performs **client-side verification only**
- The actual database update happens when the webhook from Razorpay arrives (`payment.captured` event)
- The webhook will:
  1. Verify signature again (server-side)
  2. Update database status to `PAID`
  3. Publish `payment.verified` event to Kafka
  4. Auto-schedule interview (if registration payment)
- Status remains `PENDING` until webhook confirms

---

## Meeting & Application

### 1. Schedule Meeting
**POST** `/schedule-meet`

Schedules Google Meet and sends link to student.

**Prerequisite:** Registration fee payment status must be `PAID` ⚠️

**Request:**
```json
{
  "student_id": 1
}
```

**Response (200):**
```json
{
  "status": "success",
  "message": "Meeting scheduled successfully",
  "data": {
    "meet_link": "https://meet.google.com/abc-defg-hij",
    "student_id": 1,
    "scheduled_at": "2025-11-18T10:30:00Z"
  }
}
```

**Error (400) - If registration fee not PAID:**
```json
{
  "status": "error",
  "error": "Interview cannot be scheduled. Registration payment status is PENDING. Please complete registration fee payment first"
}
```

---

### 2. Application Decision
**POST** `/application-action`

Accept or reject application with automated notifications.

**Prerequisite:** Registration fee payment status must be `PAID` ⚠️

**Request (Accept):**
```json
{
  "student_id": 1,
  "status": "ACCEPTED",
  "selected_course_id": 2
}
```

**Request (Reject):**
```json
{
  "student_id": 1,
  "status": "REJECTED"
}
```

**Response (Accept - 200):**
```json
{
  "status": "success",
  "message": "Application accepted successfully",
  "data": {
    "student_id": 1,
    "student_name": "John Doe",
    "student_email": "john@example.com",
    "selected_course": "Advanced Python",
    "course_id": 2,
    "course_fee": 5000.0,
    "next_step": "Please proceed with course fee payment",
    "payment_details": {
      "payment_type": "COURSE_FEE",
      "amount": 5000.0,
      "currency": "INR",
      "course_id": 2
    }
  }
}
```

**Response (Reject - 200):**
```json
{
  "status": "success",
  "message": "Application rejected successfully",
  "data": {
    "student_id": 1,
    "student_name": "John Doe",
    "student_email": "john@example.com",
    "result": "rejected",
    "notification": "Rejection email has been sent to the student"
  }
}
```

**Error (400) - If registration fee not PAID:**
```json
{
  "status": "error",
  "error": "Application status cannot be updated. Registration payment status is PENDING. Please complete registration fee payment first"
}
```

**Actions:**

**ACCEPTED:**
- Validates registration fee is PAID
- Stores course selection in `selected_course_id`
- Updates `application_status` = ACCEPTED
- Sends acceptance email via Kafka
- Includes next step: course fee payment details

**REJECTED:**
- Validates registration fee is PAID
- Updates `application_status` = REJECTED
- Sends rejection email via Kafka

---

## DLQ Management

### 1. Get DLQ Messages
**GET** `/api/dlq/messages?limit=50`

Retrieves failed email events.

**Response (200):**
```json
{
  "status": "success",
  "data": [
    {
      "id": 1,
      "topic": "emails",
      "key": "student-1",
      "error_message": "SMTP timeout",
      "status": "FAILED",
      "retry_count": 0,
      "created_at": "2025-11-26T14:50:00Z"
    }
  ]
}
```

---

### 2. Retry Failed Message
**POST** `/api/dlq/messages/retry/{message_id}`

Retries a failed email event.

**Response (200):**
```json
{
  "status": "success",
  "message": "Message retried successfully"
}
```

---

### 3. Resolve Message
**POST** `/api/dlq/messages/resolve/{message_id}`

Marks a message as resolved (acknowledged).

**Request:**
```json
{
  "notes": "Manually resolved"
}
```

**Response (200):**
```json
{
  "status": "success",
  "message": "Message resolved successfully"
}
```

---

### 4. Get DLQ Statistics
**GET** `/api/dlq/stats`

Retrieves DLQ statistics and metrics.

**Response (200):**
```json
{
  "status": "success",
  "data": {
    "total_failed_messages": 5,
    "messages_by_status": {
      "FAILED": 2,
      "RETRIED": 2,
      "RESOLVED": 1
    }
  }
}
```

---

## Email System (Kafka)

### Architecture

**Principle:** All emails are asynchronous through Kafka. No direct SMTP from handlers.

```
Event Trigger
    ↓
SendEmail() → Publish to Kafka "emails" topic
    ↓
Kafka Consumer → Listen to "emails" topic
    ↓
handleEmailSend() → Extract event data
    ↓
SendEmailDirect() → SMTP delivery (ONLY called by consumer)
    ↓
Email Delivered
```

### Email Events

#### 1. Welcome Email
**Trigger:** Lead created  
**Recipient:** Student  

#### 2. Counselor Assignment Notification
**Trigger:** Lead created  
**Recipient:** Assigned counselor  

#### 3. Interview Scheduling Email
**Trigger:** Registration payment marked PAID  
**Recipient:** Student  

**Complete Interview Scheduling Flow:**

```
Registration Payment Webhook
    ↓
Payment marked PAID
    ↓
interview_scheduled_at set to NOW + 1 hour ✅
application_status = 'INTERVIEW_SCHEDULED' ✅
    ↓
scheduleInterviewAfterPayment() publishes event to Kafka
    ↓
Kafka Consumer → handleInterviewSchedule()
    ↓
ScheduleMeet(studentID, email) called
    ↓
Generate Google Meet link
Send email via Kafka
Update student_lead.meet_link ✅
    ↓
student_lead updated with:
  - interview_scheduled_at: timestamp
  - meet_link: https://meet.google.com/xxx
  - application_status: INTERVIEW_SCHEDULED
```

**Event JSON:**
```json
{
  "event": "email.send",
  "email_type": "interview_scheduled",
  "recipient": "john@example.com",
  "subject": "Meeting Scheduled for Nov 26, 2025 3:53 PM",
  "body": "Dear John Doe,\n\nYour interview has been scheduled!\n\nGoogle Meet Link: https://meet.google.com/abc-defg-hij\nDate & Time: Nov 26, 2025 3:53 PM IST\n\nPlease join 5 minutes before.",
  "ts": "2025-11-18T10:35:00Z"
}
```

**Key Points:**
- Only on **first successful payment**
- Duplicate webhooks do NOT reschedule
- Meeting link auto-generated and stored in database
- Interview time set immediately in webhook
- Email queued to Kafka immediately
- All fields (`interview_scheduled_at`, `meet_link`, `application_status`) updated in single transaction

#### 4. Application Acceptance Email
**Trigger:** Application accepted  
**Recipient:** Student  
**With:** Offer letter PDF attachment  

#### 5. Application Rejection Email
**Trigger:** Application rejected  
**Recipient:** Student  

#### 6. Course Enrollment Confirmation
**Trigger:** Course fee payment marked PAID  
**Recipient:** Student  

---

### Kafka Topics

| Topic | Events | Purpose |
|-------|--------|---------|
| `emails` | `email.send`, `interview.schedule` | Email notifications & interview scheduling |
| `payments` | `payment.initiated`, `payment.verified` | Payment lifecycle |
| `dlq.emails` | Failed events | Dead Letter Queue |

---

### Kafka Setup

**Start Kafka with Docker Compose:**
```bash
docker-compose up -d
```

**Monitor Email Events:**
```bash
docker exec -it kafka kafka-console-consumer.sh \
  --bootstrap-server localhost:9092 \
  --topic emails \
  --from-beginning
```

---

## Payment Restrictions & Business Rules

### Restriction 1: Course Fee Payment Requires Registration Fee
**Endpoint:** `POST /initiate-payment`

Students cannot initiate course fee payments until the registration fee status is `PAID`.

| Condition | Result |
|-----------|--------|
| Registration status: NOT_INITIATED | ❌ Error: "Registration payment not initiated. Please pay the registration fee first" |
| Registration status: PENDING | ❌ Error: "Registration payment status is PENDING. Please complete registration fee payment before proceeding with course fee payment" |
| Registration status: FAILED | ❌ Error: "Registration payment status is FAILED. Please complete registration fee payment before proceeding with course fee payment" |
| Registration status: PAID | ✅ Course fee payment allowed |

### Restriction 2: Interview Scheduling Requires Registration Fee
**Endpoint:** `POST /schedule-meet`

The system cannot schedule an interview unless the registration fee is `PAID`.

| Condition | Result |
|-----------|--------|
| Registration status: NOT_INITIATED | ❌ Error: "Registration payment record not found. Please complete registration fee payment first" |
| Registration status: PENDING | ❌ Error: "Interview cannot be scheduled. Registration payment status is PENDING. Please complete registration fee payment first" |
| Registration status: FAILED | ❌ Error: "Interview cannot be scheduled. Registration payment status is FAILED. Please complete registration fee payment first" |
| Registration status: PAID | ✅ Interview scheduling allowed |

### Restriction 3: Application Actions Require Registration Fee
**Endpoint:** `POST /application-action`

Application status updates (Accept/Reject) are only allowed after the registration fee is `PAID`.

| Condition | Result |
|-----------|--------|
| Registration status: NOT_INITIATED | ❌ Error: "Registration payment record not found. Please complete registration fee payment first" |
| Registration status: PENDING | ❌ Error: "Application status cannot be updated. Registration payment status is PENDING. Please complete registration fee payment first" |
| Registration status: FAILED | ❌ Error: "Application status cannot be updated. Registration payment status is FAILED. Please complete registration fee payment first" |
| Registration status: PAID | ✅ Application action (Accept/Reject) allowed |

---

### Email Format
```regex
^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$
```

### Phone Format (E.164)
```regex
^\+?[1-9]\d{1,14}$
```

### Lead Source
```
Valid: "website", "referral"
```

---

## Error Handling

### HTTP Status Codes

| Code | Meaning |
|------|---------|
| 200 | Success |
| 201 | Created |
| 400 | Bad Request |
| 404 | Not Found |
| 409 | Conflict |
| 500 | Server Error |

---

## Testing

### Using cURL

**Create Lead:**
```bash
curl -X POST http://localhost:8080/create-lead \
  -H "Content-Type: application/json" \
  -d '{
    "name": "John Doe",
    "email": "john@example.com",
    "phone": "+919876543210",
    "education": "B.Tech",
    "lead_source": "website"
  }'
```

**Get Leads:**
```bash
curl http://localhost:8080/leads
```

**Initiate Payment:**
```bash
curl -X POST http://localhost:8080/initiate-payment \
  -H "Content-Type: application/json" \
  -d '{
    "student_id": 1,
    "payment_type": "REGISTRATION"
  }'
```

**Verify Payment:**
```bash
curl -X POST http://localhost:8080/verify-payment \
  -H "Content-Type: application/json" \
  -d '{
    "order_id": "order_Rh9Vc899yylv78",
    "payment_id": "pay_Rh9Vc899yylv78",
    "razorpay_signature": "9ef4dffbfd84f1318f6739a3ce19f9d85851857ae648f114332d8401e0949a3d"
  }'
```

**Accept Application:**
```bash
curl -X POST http://localhost:8080/application-action \
  -H "Content-Type: application/json" \
  -d '{"student_id": 1, "status": "ACCEPTED"}'
```

---

## Version History

| Version | Date | Changes |
|---------|------|---------|
| 2.2.0 | Nov 26, 2025 | **Payment Restrictions Implemented:** Course fee requires registration fee PAID, Interview scheduling requires registration fee PAID, Application actions require registration fee PAID |
| 2.1.0 | Nov 26, 2025 | Complete Kafka email system, interview scheduling automation |
| 2.0.0 | Nov 18, 2025 | Kafka integration, async emails |
| 1.0.0 | Nov 1, 2025 | Initial release |

---

**Last Updated:** November 26, 2025  
**Repository:** Admission_Module (features-V1 branch)
