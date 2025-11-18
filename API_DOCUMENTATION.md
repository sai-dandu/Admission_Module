# Admission Module API Documentation

## Overview
The Admission Module is a Go-based REST API service for managing student admissions, including lead management, counselor assignments, meeting scheduling, separate payment processing for registration and course fees via Razorpay, and event-driven architecture using Apache Kafka.

**Base URL:** `http://localhost:8080`  
**Version:** 2.0.0  
**Last Updated:** November 18, 2025

---

## Prerequisites
- Go 1.24+
- PostgreSQL 12+
- Docker & Docker Compose (optional, for Kafka)
- Windows PowerShell or Git Bash

---

## Project Structure
```
admission-module/
├── cmd/server/
│   └── main.go                  # Server entry point & routing
├── config/
│   └── config.go                # Configuration management
├── db/
│   └── connection.go            # Database connection & schema initialization
├── http/
│   ├── http.go                  # HTTP server setup & middleware
│   ├── handlers/                # API endpoint handlers
│   │   ├── lead.go              # Lead management (create, list, bulk upload)
│   │   ├── payment.go           # Payment initiation & verification
│   │   ├── counsellor.go        # Counselor management
│   │   ├── meet.go              # Meeting scheduling
│   │   ├── review.go            # Reviews & feedback
│   ├── middleware/
│   │   └── cors.go              # CORS configuration
│   ├── response/                # Response utilities
│   └── services/                # Business logic & integrations
│       ├── payment.go           # Payment processing (Razorpay, registration/course fees)
│       ├── notification.go      # Email notifications
│       ├── kafka.go             # Event streaming
│       ├── excel.go             # Excel file parsing
│       ├── google_meet.go       # Google Meet integration
│       └── offer_letter.go      # Offer letter generation
├── models/                      # Data models
│   ├── lead.go                  # Lead & LeadResponse structs
│   ├── payment.go               # Payment models
│   └── counsellor.go            # Counselor models
├── errors/                      # Error handling
│   ├── common.go                # Common errors
│   ├── errors.go                # Error types
│   └── validation.go            # Validation errors
├── utils/                       # Utility functions
│   ├── constants.go             # Constants & enums
│   ├── request.go               # Request utilities
│   ├── response.go              # Response utilities
│   ├── data_converter.go        # Data conversion & mapping
│   ├── validation.go            # Input validation
│   └── query_parser.go          # Query parameter parsing
├── logger/
│   └── logger.go                # Logging configuration
├── docker-compose.yml           # Kafka & dependencies setup
├── go.mod & go.sum              # Go dependencies
└── README.md                    # Project overview
```

---

## Configuration

### Environment Variables
Create a `.env` file in the project root:

```env
# Database
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=your_password
DB_NAME=admission_db

# Razorpay Payment Gateway
RazorpayKeyID=rzp_test_xxxxx
RazorpayKeySecret=your_secret_key

# Email Service (SMTP)
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USER=your_email@gmail.com
SMTP_PASS=your_app_password
EMAIL_FROM=noreply@admission-module.com

# Kafka (Optional)
KAFKA_BROKERS=localhost:9092
KAFKA_TOPIC=admissions.payments

# Server
SERVER_PORT=8080
```

---

## Database Schema

### Tables

#### 1. `student_lead` - Main student record
```sql
CREATE TABLE student_lead (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) NOT NULL UNIQUE,
    phone VARCHAR(20) NOT NULL UNIQUE,
    education VARCHAR(255),
    lead_source VARCHAR(100),
    counselor_id INTEGER REFERENCES counselor(id) ON DELETE SET NULL,
    registration_fee_status VARCHAR(50) DEFAULT 'PENDING',  -- PENDING, PAID
    course_fee_status VARCHAR(50) DEFAULT 'PENDING',         -- PENDING, PAID
    meet_link TEXT,
    application_status VARCHAR(50) DEFAULT 'NEW',
    registration_payment_id INTEGER,
    selected_course_id INTEGER REFERENCES course(id) ON DELETE SET NULL,
    course_payment_id INTEGER,
    interview_scheduled_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

#### 2. `registration_payment` - Registration fee payments
```sql
CREATE TABLE registration_payment (
    id SERIAL PRIMARY KEY,
    student_id INTEGER NOT NULL UNIQUE REFERENCES student_lead(id) ON DELETE CASCADE,
    amount NUMERIC(10, 2) NOT NULL,
    status VARCHAR(50) DEFAULT 'PENDING',                    -- PENDING, PAID
    order_id VARCHAR(255) UNIQUE,                            -- Razorpay order ID
    payment_id VARCHAR(255),                                 -- Razorpay payment ID
    razorpay_sign TEXT,                                      -- Payment signature
    timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

#### 3. `course_payment` - Course fee payments
```sql
CREATE TABLE course_payment (
    id SERIAL PRIMARY KEY,
    student_id INTEGER NOT NULL REFERENCES student_lead(id) ON DELETE CASCADE,
    course_id INTEGER NOT NULL REFERENCES course(id) ON DELETE CASCADE,
    amount NUMERIC(10, 2) NOT NULL,
    status VARCHAR(50) DEFAULT 'PENDING',                    -- PENDING, PAID
    order_id VARCHAR(255) UNIQUE,                            -- Razorpay order ID
    payment_id VARCHAR(255),                                 -- Razorpay payment ID
    razorpay_sign TEXT,                                      -- Payment signature
    timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT unique_student_course UNIQUE(student_id, course_id)
);
```

#### 4. `counselor` - Counselor profiles
```sql
CREATE TABLE counselor (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) NOT NULL,
    phone VARCHAR(20),
    assigned_count INTEGER DEFAULT 0,
    max_capacity INTEGER DEFAULT 10,
    is_referral_enabled BOOLEAN DEFAULT false,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

#### 5. `course` - Available courses
```sql
CREATE TABLE course (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    fee NUMERIC(10, 2) NOT NULL,
    duration VARCHAR(100),
    is_active INTEGER DEFAULT 1,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

### Performance Indexes
```sql
CREATE INDEX idx_student_lead_registration_fee_status ON student_lead(registration_fee_status);
CREATE INDEX idx_student_lead_course_fee_status ON student_lead(course_fee_status);
CREATE INDEX idx_student_lead_interview_scheduled ON student_lead(interview_scheduled_at);
CREATE INDEX idx_registration_payment_student_id ON registration_payment(student_id);
CREATE INDEX idx_registration_payment_order_id ON registration_payment(order_id);
CREATE INDEX idx_course_payment_student_id ON course_payment(student_id);
CREATE INDEX idx_course_payment_order_id ON course_payment(order_id);
```

---

## API Endpoints

### Authentication
⚠️ **No authentication required.** All endpoints are publicly accessible.

### Response Format

All successful responses follow this structure:
```json
{
  "status": "success",
  "message": "Descriptive message",
  "data": {}
}
```

All error responses:
```json
{
  "status": "error",
  "error": "Error description"
}
```

---

## Lead Management Endpoints

### 1. Create Lead
**POST** `/create-lead`

Creates a new student lead and assigns a counselor.

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

**Response (201 Created):**
```json
{
  "status": "success",
  "message": "Lead created successfully",
  "data": {
    "student_id": 1,
    "name": "John Doe",
    "email": "john@example.com",
    "phone": "+919876543210",
    "counsellor_name": "Counselor Rishi",
    "counsellor_email": "rishi@university.edu"
  }
}
```

**Validation Rules:**
- **name**: Required, 1-255 characters
- **email**: Required, valid email format, must be unique
- **phone**: Required, E.164 format (e.g., `+919876543210`), must be unique
- **education**: Optional, max 255 characters
- **lead_source**: Optional, must be "website", "referral", or omitted

**Counselor Assignment Logic:**
- Website leads: Assigned from any available counselor
- Referral leads: Assigned from counselors with `is_referral_enabled = true`
- Selection: Counselor with lowest `assigned_count` (round-robin)
- Returns error if no available counselors

**Error Responses:**
- `400 Bad Request`: Validation failed
- `409 Conflict`: Email or phone already exists
- `500 Internal Server Error`: Database error

---

### 2. Get All Leads
**GET** `/leads`

Retrieves all leads with optional date filtering.

**Query Parameters:**
- `created_after` (optional): RFC3339 format (e.g., `2025-11-13T10:00:00Z`)
- `created_before` (optional): RFC3339 format

**Examples:**
```bash
# Get all leads
GET /leads

# Get leads after specific date
GET /leads?created_after=2025-11-13T00:00:00Z

# Get leads within date range
GET /leads?created_after=2025-11-01T00:00:00Z&created_before=2025-11-30T23:59:59Z
```

**Response (200 OK):**
```json
{
  "status": "success",
  "message": "Retrieved 5 leads successfully",
  "count": 5,
  "data": [
    {
      "id": 1,
      "name": "John Doe",
      "email": "john@example.com",
      "phone": "+919876543210",
      "education": "B.Tech",
      "lead_source": "website",
      "meet_link": "https://meet.google.com/abc-defg-hij",
      "application_status": "NEW",
      "selected_course_id": null,
      "interview_scheduled_at": null,
      "created_at": "2025-11-17T15:05:34Z",
      "updated_at": "2025-11-17T15:05:34Z"
    }
  ]
}
```

---

### 3. Upload Leads (Bulk)
**POST** `/upload-leads`

Upload multiple leads from an Excel file (.xlsx).

**Request:**
- Content-Type: `multipart/form-data`
- File: Excel file with columns: name, email, phone, education, lead_source

**Excel Format:**
| name | email | phone | education | lead_source |
|------|-------|-------|-----------|-------------|
| John Doe | john@example.com | +919876543210 | B.Tech | website |
| Jane Smith | jane@example.com | +919123456789 | M.Tech | referral |

**Response (200 OK):**
```json
{
  "status": "success",
  "message": "Successfully uploaded 100 leads",
  "data": {
    "total_count": 100,
    "success_count": 98,
    "failed_count": 2,
    "failed_leads": [
      {
        "row": 5,
        "email": "duplicate@example.com",
        "phone": "+919876543210",
        "error": "lead already exists with this email or phone"
      },
      {
        "row": 12,
        "email": "invalid@",
        "phone": "+919111111111",
        "error": "invalid email format"
      }
    ]
  }
}
```

**Features:**
- Deduplicates leads within file
- Validates each lead independently
- Returns detailed error messages with row numbers
- Continues processing despite failures
- Assigns counselors to successful leads

---

## Payment Management Endpoints

### Payment Flow
The system supports two separate payment types:
1. **Registration Fee**: Fixed fee to register, schedules interview automatically
2. **Course Fee**: Variable fee per course, stores course selection

### 1. Initiate Payment
**POST** `/initiate-payment`

Creates a Razorpay order for registration or course fee payment.

**Request for Registration Fee:**
```json
{
  "student_id": 1,
  "payment_type": "REGISTRATION"
}
```

**Request for Course Fee:**
```json
{
  "student_id": 1,
  "payment_type": "COURSE_FEE",
  "course_id": 2
}
```

**Response (200 OK):**
```json
{
  "status": "success",
  "message": "Payment order created",
  "data": {
    "order_id": "order_Rh9Vc899yylv78",
    "amount": 1870.0,
    "currency": "INR",
    "receipt": "rcpt_1_REGISTRATION"
  }
}
```

**Payment Type Details:**

**REGISTRATION:**
- Fixed amount: ₹1,870
- Creates `registration_payment` record
- Updates `registration_fee_status` to PENDING
- On verification: Auto-schedules interview 1 hour later

**COURSE_FEE:**
- Requires `course_id` parameter
- Amount: From course fee table
- Creates `course_payment` record
- Updates `course_fee_status` to PENDING
- On verification: Stores `selected_course_id`

**Behavior:**
- If PENDING payment exists: Updates with new `order_id` (allows retry)
- If PAID payment exists: Returns error (can't re-pay)
- First attempt: Creates new payment record

**Error Responses:**
- `400 Bad Request`: Missing required fields or invalid payment_type
- `404 Not Found`: Student or course not found
- `409 Conflict`: Course fee already paid
- `500 Internal Server Error`: Payment creation failed

---

### 2. Verify Payment
**POST** `/verify-payment`

Verifies Razorpay payment signature and updates payment status to PAID.

**Request:**
```json
{
  "order_id": "order_Rh9Vc899yylv78",
  "payment_id": "pay_Rh9Vc899yylv78",
  "razorpay_signature": "9ef4dffbfd84f1318f6739a3ce19f9d85851857ae648f114332d8401e0949a3d"
}
```

**Response (200 OK):**
```json
{
  "status": "success",
  "message": "Payment verified successfully",
  "data": {
    "student_id": 1,
    "order_id": "order_Rh9Vc899yylv78",
    "payment_id": "pay_Rh9Vc899yylv78",
    "status": "PAID"
  }
}
```

**Updates on Success:**

**For Registration Payment:**
- Sets `registration_payment.status` = PAID
- Sets `student_lead.registration_fee_status` = PAID
- Sets `student_lead.interview_scheduled_at` = NOW + 1 hour
- Sets `student_lead.application_status` = INTERVIEW_SCHEDULED
- Stores `registration_payment_id` in student_lead

**For Course Payment:**
- Sets `course_payment.status` = PAID
- Sets `student_lead.course_fee_status` = PAID
- Stores `selected_course_id` and `course_payment_id` in student_lead
- Sends confirmation email to student

**Error Responses:**
- `400 Bad Request`: Missing required fields
- `404 Not Found`: Payment not found
- `401 Unauthorized`: Invalid signature (payment rejected)
- `500 Internal Server Error`: Verification failed

---

## Counselor Management Endpoints

### Assign Counselor
**POST** `/assign-counsellor`

Manually assigns counselors to leads (typically runs after bulk upload).

**Request:**
```json
{
  "lead_source": "website"
}
```

**Response (200 OK):**
```json
{
  "status": "success",
  "message": "Counsellors assigned successfully"
}
```

---

## Meeting Management Endpoints

### Schedule Meet
**POST** `/schedule-meet`

Schedules Google Meet for a student and sends meeting link.

**Request:**
```json
{
  "student_id": 1
}
```

**Response (200 OK):**
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

---

## Application Management Endpoints

### Application Action
**POST** `/application-action`

Updates application status (ACCEPTED/REJECTED) and sends notification.

**Request:**
```json
{
  "student_id": 1,
  "status": "ACCEPTED"
}
```

**Response (200 OK - ACCEPTED):**
```json
{
  "status": "success",
  "message": "Application accepted and offer letter sent to John Doe"
}
```

**Response (200 OK - REJECTED):**
```json
{
  "status": "success",
  "message": "Application rejected and notification sent to John Doe"
}
```

**Behavior:**
- **ACCEPTED**: Generates offer letter, sends to student email
- **REJECTED**: Sends rejection notification

---

## Static Files

### Serve Static Files
**GET** `/static/{filename}`

Serves static files (test pages, assets).

**Example:**
```bash
GET /static/test-payment.html
```

---

## Kafka Integration

### Event-Driven Architecture
Kafka enables real-time event streaming for admission workflow.

### Events Published

#### 1. `lead.created`
Published when a lead is created (single or bulk).

```json
{
  "event": "lead.created",
  "student_id": 1,
  "name": "John Doe",
  "email": "john@example.com",
  "phone": "+919876543210",
  "education": "B.Tech",
  "lead_source": "website",
  "counselor_id": 5,
  "counselor_name": "Rishi",
  "ts": "2025-11-18T10:30:45Z"
}
```

#### 2. `payment.initiated`
Published when payment order is created.

```json
{
  "event": "payment.initiated",
  "student_id": 1,
  "order_id": "order_Rh9Vc899yylv78",
  "amount": 1870.0,
  "currency": "INR",
  "payment_type": "REGISTRATION",
  "status": "PENDING",
  "ts": "2025-11-18T10:40:10Z"
}
```

#### 3. `payment.verified`
Published when payment is verified and PAID.

```json
{
  "event": "payment.verified",
  "student_id": 1,
  "order_id": "order_Rh9Vc899yylv78",
  "payment_id": "pay_Rh9Vc899yylv78",
  "payment_type": "REGISTRATION",
  "status": "PAID",
  "ts": "2025-11-18T10:42:55Z"
}
```

### Kafka Setup

**Start Kafka:**
```bash
docker-compose up -d
```

**Consume Events:**
```bash
docker exec -it <kafka-container> kafka-console-consumer.sh \
  --bootstrap-server localhost:9092 \
  --topic admissions.payments \
  --from-beginning
```

### Configuration
- **Producer:** Initialized at server startup
- **Retry Logic:** 3 attempts with exponential backoff
- **Non-blocking:** Server continues if Kafka fails
- **Topic:** `admissions.payments`

---

## Input Validation Rules

### Email Format
```regex
^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$
```

**Valid Examples:**
- `user@example.com`
- `first.last@domain.co.uk`
- `user+tag@email.com`

**Invalid Examples:**
- `invalid.email@`
- `@nodomain.com`
- `no-at-sign.com`

### Phone Format (E.164)
```regex
^\+?[1-9]\d{1,14}$
```

**Valid Examples:**
- `+919876543210` (India)
- `+12025550123` (USA)
- `+441234567890` (UK)
- `19876543210` (with + optional)

**Invalid Examples:**
- `9876543210` (missing country code)
- `(123) 456-7890` (formatting characters)
- `+1` (incomplete)
- `+0123456789` (leading zero invalid)

### Lead Source
```
Valid values: "website", "referral"
If omitted: Treated as general lead (no source preference)
```

---

## Error Handling

### HTTP Status Codes
| Code | Meaning | Example |
|------|---------|---------|
| 200 | Success | Lead retrieved successfully |
| 201 | Created | Lead created successfully |
| 400 | Bad Request | Invalid email format, missing field |
| 404 | Not Found | Student ID not found, course not found |
| 409 | Conflict | Email already exists, payment already paid |
| 500 | Server Error | Database error, service unavailable |

### Error Response Format
```json
{
  "status": "error",
  "error": "Specific error message"
}
```

### Common Errors

**Invalid Email (400):**
```json
{
  "status": "error",
  "error": "invalid email format"
}
```

**Duplicate Email (409):**
```json
{
  "status": "error",
  "error": "lead already exists with this email or phone"
}
```

**Invalid Phone (400):**
```json
{
  "status": "error",
  "error": "invalid phone format (use E.164 format, e.g., +919876543210)"
}
```

**Student Not Found (404):**
```json
{
  "status": "error",
  "error": "student not found"
}
```

**Payment Already Paid (409):**
```json
{
  "status": "error",
  "error": "course payment already completed"
}
```

---

## Running the Application

### Development
```bash
# Install dependencies
go mod tidy

# Create .env file with credentials
# (See Configuration section)

# Run server
go run ./cmd/server

# Server starts on http://localhost:8080
```

### Production Build
```bash
# Build executable
go build -o admission-server.exe ./cmd/server

# Run with environment variables
$env:DB_HOST="prod-db.example.com"
$env:RazorpayKeyID="rzp_live_xxxxx"
./admission-server.exe
```

### Docker
```bash
# Build image
docker build -t admission-module:latest .

# Run container
docker run -p 8080:8080 \
  -e DB_HOST=postgres \
  -e DB_USER=postgres \
  -e RazorpayKeyID=rzp_test_xxxxx \
  admission-module:latest
```

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
curl -X GET "http://localhost:8080/leads"
```

**Get Leads with Filter:**
```bash
curl -X GET "http://localhost:8080/leads?created_after=2025-11-17T00:00:00Z"
```

**Upload Leads:**
```bash
curl -X POST http://localhost:8080/upload-leads \
  -F "file=@leads.xlsx"
```

**Initiate Registration Payment:**
```bash
curl -X POST http://localhost:8080/initiate-payment \
  -H "Content-Type: application/json" \
  -d '{
    "student_id": 1,
    "payment_type": "REGISTRATION"
  }'
```

**Initiate Course Fee Payment:**
```bash
curl -X POST http://localhost:8080/initiate-payment \
  -H "Content-Type: application/json" \
  -d '{
    "student_id": 1,
    "payment_type": "COURSE_FEE",
    "course_id": 2
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

### Using Postman
1. Import `admission_module_postman_collection.json`
2. Update environment variables
3. Execute test requests

---

## Troubleshooting

### Issue: Connection Refused
**Solution:** 
- Verify PostgreSQL is running
- Check `DB_HOST` and `DB_PORT` in `.env`
- Verify database exists

### Issue: Payment Order Creation Fails
**Solution:**
- Verify Razorpay credentials in `.env`
- Check `RazorpayKeyID` and `RazorpayKeySecret`
- Ensure using test credentials for development

### Issue: Kafka Connection Failed
**Solution:**
- Start Kafka: `docker-compose up -d`
- Wait 10 seconds for Kafka to initialize
- Check `KAFKA_BROKERS` environment variable

### Issue: Duplicate Lead Error on Retry
**Solution:**
- This is expected behavior - email and phone are unique
- Use different email/phone or delete the existing lead
- For payments: You can retry with the same student_id if status is PENDING

### Issue: Email Not Sending
**Solution:**
- Verify SMTP credentials in `.env`
- Check `EMAIL_FROM` address format
- Enable "Less Secure App Access" for Gmail

---

## Architecture

### Request Flow
```
HTTP Request
    ↓
Handler (method validation, request parsing)
    ↓
Service (business logic, database operations)
    ↓
Database / External Services (Razorpay, Kafka, Email)
    ↓
Response
```

### Key Design Patterns

**Service-Oriented:**
- Each domain has dedicated service (PaymentService, NotificationService)
- Clear separation between HTTP handlers and business logic

**Transaction-Safe:**
- All multi-step operations use database transactions
- Automatic rollback on any error
- Row-level locking prevents race conditions

**Event-Driven:**
- Events published to Kafka for real-time processing
- Non-blocking (failures don't affect main flow)
- Async event handling by consumers

---

## Future Enhancements
- SMS notifications via Twilio
- Batch email sending via SendGrid
- Payment analytics dashboard
- Interview scheduling calendar
- Document upload/management
- Application tracking system
- Admin portal for managing counselors/courses
- API rate limiting
- Request authentication/authorization
