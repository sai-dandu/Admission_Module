# Admission Module - API Documentation

## Overview

The Admission Module is a comprehensive Go-based REST API service for managing student admissions with the following core features:

- **Lead Management**: Create and manage student leads with counselor assignment
- **Payment Processing**: Razorpay integration for registration and course fees with separate payment flows
- **Interview Scheduling**: Automatic interview scheduling with Google Meet integration after registration payment
- **Application Management**: Accept/reject applications with offer letter generation
- **Email Notifications**: All emails sent asynchronously through Kafka for reliability and scalability
- **Event-Driven Architecture**: Apache Kafka for real-time event streaming and async processing
- **Dead Letter Queue (DLQ)**: Automatic handling and retry of failed email events

**Technology Stack:**
- Language: Go 1.24
- Database: PostgreSQL 12+
- Message Broker: Apache Kafka
- Email: SMTP via Gomail
- Payment Gateway: Razorpay
- Meeting Scheduling: Google Meet API
- Excel Processing: Excelize

**Version:** 2.1.0  
**Last Updated:** November 26, 2025  
**Branch:** features-V1

---

## Screenshots

### Payment Interfaces
<img width="1360" height="640" alt="Payment Page" src="https://github.com/user-attachments/assets/7d1c6817-8e2e-4b35-b2e6-aa9b9eadcea0" />

### Registration Fee
<img width="1362" height="660" alt="Registration Fee" src="https://github.com/user-attachments/assets/45965150-9149-43fc-ae2b-795bd421cffb" />

### Course Fee Payment
<img width="1349" height="666" alt="Course Fee Payment" src="https://github.com/user-attachments/assets/c3af4329-403d-4f0d-89c8-fb67376192b5" />

### Database Schema
<img width="1360" height="662" alt="Database Example" src="https://github.com/user-attachments/assets/fbcc9c24-c515-4d4c-ade1-da2122a04c48" />

---

## Project Structure

```
admission-module/
├── cmd/server/
│   └── main.go                      # Server entry point, Kafka setup, email processor registration
│
├── config/
│   └── config.go                    # Configuration management, environment variable loading
│
├── db/
│   ├── connection.go                # PostgreSQL connection, pool management
│   └── migrations/
│       └── 001_complete_schema.sql  # Complete database schema (all tables & indexes)
│
├── http/
│   ├── http.go                      # HTTP server setup, middleware pipeline
│   ├── handlers/                    # API endpoint implementations
│   │   ├── lead.go                  # GET /leads, POST /create-lead, POST /upload-leads
│   │   ├── payment.go               # POST /initiate-payment, POST /verify-payment
│   │   ├── course.go                # GET /courses, course management
│   │   ├── counsellor.go            # Counselor management & assignment
│   │   ├── meet.go                  # POST /schedule-meet
│   │   ├── review.go                # POST /application-action (accept/reject)
│   │   └── dlq.go                   # DLQ management: GET /dlq-messages, POST /retry-dlq-message
│   ├── middleware/
│   │   └── cors.go                  # CORS configuration
│   └── response/
│       └── response.go              # Standard response utilities
│
├── services/                        # Business logic & integrations
│   ├── application.go               # Application acceptance/rejection logic
│   ├── email.go                     # Email publishing to Kafka (KAFKA ONLY - no direct SMTP)
│   ├── email_sender.go              # Direct SMTP sending (called only by Kafka consumer)
│   ├── notification.go              # Welcome & counselor notification emails
│   ├── google_meet.go               # Google Meet link generation & scheduling
│   ├── payment.go                   # Payment logic (Razorpay integration)
│   ├── webhook.go                   # Razorpay webhook handler (payment verification)
│   ├── excel.go                     # Excel file parsing for bulk lead upload
│   ├── kafka_wrapper.go             # Wrapper for Kafka producer/consumer functions
│   └── kafka/                       # Kafka client implementation
│       ├── producer.go              # Event publishing to Kafka topics
│       ├── consumer.go              # Event consuming from Kafka "emails" topic
│       └── connect.go               # DLQ producer & management
│
├── models/                          # Data structures
│   ├── lead.go                      # Lead, LeadResponse structs
│   ├── payment.go                   # PaymentRequest, PaymentResponse structs
│   ├── course.go                    # Course model
│   └── counsellor.go                # Counselor model
│
├── errors/                          # Error handling
│   ├── common.go                    # Common error definitions
│   └── errors.go                    # Error types & utilities
│
├── logger/
│   └── logger.go                    # Structured logging with timestamps
│
├── utils/                           # Utility functions
│   ├── constants.go                 # Constants, enums, validation patterns
│   ├── request.go                   # Request parsing utilities
│   ├── response.go                  # Response formatting utilities
│   ├── data_converter.go            # Data type conversions
│   ├── validation.go                # Input validation functions
│   ├── lead_utils.go                # Lead-specific utilities
│   └── query_parser.go              # Query parameter parsing
│
├── static/
│   └── test-payment.html            # Payment testing interface
│
├── docker-compose.yml               # Kafka, PostgreSQL setup
├── go.mod & go.sum                  # Go dependencies
├── .env.example                     # Environment variables template
├── POSTMAN_COLLECTION.json          # API test collection
└── API_DOCUMENTATION.md             # Complete API reference
```

---

## Quick Start

### Prerequisites
- Go 1.24+
- PostgreSQL 12+
- Docker & Docker Compose (for Kafka & PostgreSQL)
- Windows PowerShell or Git Bash

### Installation

1. **Clone Repository**
```bash
git clone <repository-url>
cd admission-module
```

2. **Install Dependencies**
```bash
go mod tidy
```

3. **Configure Environment**
```bash
cp .env.example .env
# Edit .env with your credentials
```

4. **Start Services**
```bash
# Start PostgreSQL & Kafka
docker-compose up -d

# Wait 10 seconds for services to initialize
Start-Sleep -Seconds 10

# Run server
go run ./cmd/server
```

5. **Verify Server**
```bash
# Server starts on http://localhost:8080
# Test with: curl http://localhost:8080/leads
```

### Stop Services
```bash
# Stop and remove containers
docker-compose down

# Remove volumes (data)
docker-compose down -v
```

---

## Configuration

### Environment Variables
Create `.env` file in project root:

```env
# Database Configuration
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

# Kafka Configuration (Optional - disable if empty)
KAFKA_BROKERS=localhost:9092

# Server
SERVER_PORT=8080
```

### Gmail App Password Setup
For Gmail SMTP:
1. Enable 2-Factor Authentication
2. Go to: https://myaccount.google.com/apppasswords
3. Select Mail & Windows (or your device)
4. Copy the generated 16-character password to `SMTP_PASS`

---

## Database Schema

### Core Tables

#### 1. `student_lead` - Student Records
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

#### 4. `counselor` - Counselor Profiles
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

#### 5. `course` - Available Courses
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

### Indexes for Performance
```sql
CREATE INDEX idx_student_lead_email ON student_lead(email);
CREATE INDEX idx_student_lead_phone ON student_lead(phone);
CREATE INDEX idx_student_lead_registration_status ON student_lead(registration_fee_status);
CREATE INDEX idx_student_lead_course_status ON student_lead(course_fee_status);
CREATE INDEX idx_student_lead_application_status ON student_lead(application_status);
CREATE INDEX idx_student_lead_interview_scheduled ON student_lead(interview_scheduled_at);
CREATE INDEX idx_student_lead_counselor_id ON student_lead(counselor_id);
CREATE INDEX idx_registration_payment_student_id ON registration_payment(student_id);
CREATE INDEX idx_registration_payment_order_id ON registration_payment(order_id);
CREATE INDEX idx_registration_payment_status ON registration_payment(status);
CREATE INDEX idx_course_payment_student_id ON course_payment(student_id);
CREATE INDEX idx_course_payment_order_id ON course_payment(order_id);
CREATE INDEX idx_course_payment_status ON course_payment(status);
CREATE INDEX idx_dlq_messages_status ON dlq_messages(status);
CREATE INDEX idx_dlq_messages_created_at ON dlq_messages(created_at);
```

---

## Key Features

### 1. Asynchronous Email System

**Architecture Principle:** All emails are published to Kafka and processed asynchronously. No direct SMTP sending from handlers.

```
Lead Created / Payment Completed / etc.
    ↓
SendEmail() - Publishes to Kafka
    ↓
Kafka Consumer - Listens to "emails" topic
    ↓
handleEmailSend() - Routes email event
    ↓
SendEmailDirect() - SMTP delivery (ONLY called by consumer)
    ↓
Email Delivered
```

**Benefits:**
- ✅ Reliable delivery with automatic retry
- ✅ Decoupled architecture - handlers don't wait for email
- ✅ Scalable - consumer can be run on separate service
- ✅ Failure handling with Dead Letter Queue
- ✅ No blocking operations in request path

### 2. Payment Processing

**Two Separate Payment Types:**

| Feature | Registration | Course Fee |
|---------|--------------|-----------|
| Amount | Fixed ₹1,870 | Variable (per course) |
| Trigger | Student registration | Course enrollment |
| On PAID | Auto-schedule interview | Store selection |
| Emails | Interview link | Enrollment confirmation |
| Retryable | Yes (if PENDING) | Yes (if PENDING) |

**Payment Flow:**
1. Client calls `/initiate-payment`
2. Razorpay order created
3. Client completes payment on Razorpay
4. Razorpay webhook calls `/verify-payment`
5. Signature verified
6. Payment marked PAID
7. Interview scheduled (registration) OR course selected (course fee)
8. Emails queued to Kafka

### 3. Interview Scheduling

**Automatic Flow:**
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

**Key Points:**
- Interview scheduled **1 hour after** payment verification
- Only scheduled on **first successful payment** (not retries)
- Duplicate webhooks do NOT reschedule
- Meeting link auto-generated and stored in database
- All fields updated in single transaction
- Email sent asynchronously via Kafka

### 4. Kafka Event System

**Topics:**
- `emails` - Email sending and interview scheduling events
- `payments` - Payment lifecycle events (optional)
- `dlq.emails` - Failed email messages (auto-retry)

**Consumer Group:** `admission-module-consumer-group`

### 5. Dead Letter Queue (DLQ)

**Purpose:** Handle failed email events with automatic retry

**API Endpoints:**
- `GET /dlq-messages?limit=50` - View failed messages
- `POST /retry-dlq-message` - Retry a specific message
- `POST /resolve-dlq-message` - Mark as resolved
- `GET /dlq-stats` - Get DLQ statistics

---

## Running the Application

### Development Mode
```bash
# Terminal 1: Start Kafka & PostgreSQL
docker-compose up

# Terminal 2: Run server
go run ./cmd/server

# Server running on http://localhost:8080
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

### Docker Deployment
```bash
# Build image
docker build -t admission-module:latest .

# Run container with Kafka
docker run -p 8080:8080 \
  -e DB_HOST=postgres \
  -e DB_USER=postgres \
  -e DB_PASSWORD=password \
  --network admission-network \
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

**Get All Leads:**
```bash
curl http://localhost:8080/leads
```

**Upload Leads (Excel):**
```bash
curl -X POST http://localhost:8080/upload-leads \
  -F "file=@leads.xlsx"
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

### Using Postman
1. Import `POSTMAN_COLLECTION.json`
2. Set environment variables
3. Execute test requests
4. View response examples

---

## Troubleshooting

### Services Won't Start
```bash
# Check containers
docker ps -a

# View logs
docker logs -f <container-name>

# Restart services
docker-compose restart
```

### Database Connection Failed
- Verify PostgreSQL is running: `docker exec postgres psql -U postgres -c "SELECT 1"`
- Check `DB_HOST`, `DB_PORT`, credentials in `.env`
- Ensure database exists: `CREATE DATABASE admission_db;`

### Kafka Consumer Not Processing Events
- Check Kafka is running: `docker exec -it kafka bash`
- List topics: `kafka-topics.sh --list --bootstrap-server localhost:9092`
- Check logs: `go run ./cmd/server 2>&1 | grep -i kafka`

### Emails Not Sending
- Verify SMTP credentials in `.env`
- Check Gmail: Enable "Less Secure App Access" or use app password
- Check logs for SMTP errors
- Verify Kafka consumer is running

### Payment Verification Fails
- Verify Razorpay credentials (test mode)
- Check webhook logs in Razorpay dashboard
- Ensure signature validation is correct

---

## Support & Documentation

- **Full API Reference:** See `API_DOCUMENTATION.md`
- **Postman Collection:** `POSTMAN_COLLECTION.json`
- **Database Schema:** See `db/migrations/001_complete_schema.sql`

---

## License
This project is proprietary and confidential.

**Last Updated:** November 26, 2025  
**Version:** 2.1.0
