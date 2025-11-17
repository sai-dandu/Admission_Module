# Admission Module API Documentation

## Overview
The Admission Module is a Go-based REST API service for managing student admissions, including lead management, counselor assignments, meeting scheduling, payment processing via Razorpay, and event-driven architecture using Apache Kafka for real-time data streaming.

**Base URL:** `http://localhost:8080`  
**Version:** 1.0.0  
**Last Updated:** November 17, 2025

## Prerequisites
- Go 1.21+
- PostgreSQL 12+
- Docker & Docker Compose (for Kafka)
- Windows PowerShell or Git Bash

## Project Structure
```
admission-module/
├── cmd/server/
│   └── main.go                  # Server entry point & routing
├── config/
│   └── config.go                # Configuration management
├── db/
│   └── connection.go            # Database connection & initialization
├── http/
│   ├── http.go                  # HTTP server setup & middleware
│   ├── handlers/                # API handlers (service-oriented)
│   │   ├── lead.go              # Lead management (LeadService)
│   │   ├── payment.go           # Payment handling
│   │   ├── counsellor.go        # Counselor management
│   │   ├── meet.go              # Meeting scheduling
│   │   ├── email.go             # Email notifications
│   │   └── review.go            # Reviews & feedback
│   ├── middleware/
│   │   └── cors.go              # CORS configuration
│   └── services/                # Business logic services
│       ├── kafka.go             # Event streaming
│       ├── email.go             # Email service
│       ├── excel.go             # Excel parsing
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
├── utils/                       # Utilities
│   ├── constants.go             # Constants & enums
│   ├── request.go               # Request utilities
│   └── response.go              # Response utilities
├── logger/
│   └── logger.go                # Logging configuration
├── migrations/                  # Database migrations
│   └── 001_improve_counsellor_assignment.sql
├── static/
│   └── test-payment.html        # Testing static files
├── docker-compose.yml           # Kafka & services setup
├── go.mod & go.sum              # Dependencies
├── REFACTORING_NOTES.md         # Refactoring documentation
└── IMPLEMENTATION_SUMMARY.md    # Implementation details
```

## Configuration
Create a `.env` file in the project root:

```env
# Database
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=your_password
DB_NAME=postgres

# Razorpay
RazorpayKeyID=rzp_test_xxxxx
RazorpayKeySecret=xxxxx

# Kafka (Optional)
KAFKA_BROKERS=host.docker.internal:9092
KAFKA_TOPIC=admissions.payments

# Email (Optional)
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USER=your_email@gmail.com
SMTP_PASS=your_app_password
EMAIL_FROM=noreply@admission-module.com
```

## API Endpoints

### Authentication
No authentication required. All endpoints are publicly accessible.

### 1. Lead Management

#### Upload Leads (Batch)
**POST** `/upload-leads`
- **Content-Type:** `multipart/form-data`
- **Body:** Excel file (.xlsx) containing lead data with columns: name, email, phone, education, lead_source
- **Response (200 OK):**
  ```json
  {
    "message": "Successfully uploaded 95 leads",
    "success_count": 95,
    "failed_count": 5,
    "total_count": 100,
    "failed_leads": [
      {
        "row": 6,
        "email": "duplicate@example.com",
        "phone": "+919876543210",
        "error": "lead already exists with this email or phone"
      }
    ]
  }
  ```
- **Error (400/500):** Invalid file or processing error

**Validation Rules:**
- **Name:** Required, 1-100 characters
- **Email:** Required, valid email format
- **Phone:** Required, E.164 format (e.g., +919876543210)
- **Education:** Optional, max 200 characters
- **Lead Source:** Optional, must be "website", "referral", or empty

#### Get All Leads
**GET** `/leads`
- **Query Parameters:**
  - `created_after` (optional): Filter leads created after this date/time (RFC3339 format, e.g., `2025-11-13T10:00:00Z`)
  - `created_before` (optional): Filter leads created before this date/time (RFC3339 format, e.g., `2025-11-13T10:00:00Z`)
- **Examples:**
  - Get all leads: `GET /leads`
  - Get leads created after a specific date: `GET /leads?created_after=2025-11-13T00:00:00Z`
  - Get leads within a date range: `GET /leads?created_after=2025-11-01T00:00:00Z&created_before=2025-11-30T23:59:59Z`
- **Response (200 OK):** 
  ```json
  {
    "status": "success",
    "message": "Retrieved 10 leads successfully",
    "count": 10,
    "data": [
      {
        "id": 1,
        "name": "John Doe",
        "email": "john@example.com",
        "phone": "+919876543210",
        "education": "B.Tech",
        "lead_source": "website",
        "payment_status": "pending",
        "meet_link": "https://meet.google.com/abc-defg-hij",
        "application_status": "new",
        "created_at": "2025-11-17T15:05:34Z",
        "updated_at": "2025-11-17T15:05:34Z"
      }
    ]
  }
  ```

#### Create Lead
**POST** `/create-lead`
- **Request:**
  ```json
  {
    "name": "Jane Smith",
    "email": "jane@example.com",
    "phone": "+919123456789",
    "education": "Master's",
    "lead_source": "referral"
  }
  ```
- **Response (201 Created):**
  ```json
  {
    "message": "Lead created successfully",
    "student_id": 6,
    "counsellor_name": "Counsellor Rishi",
    "email": "jane@example.com"
  }
  ```
- **Error Responses:**
  - 400 Bad Request: Validation error (invalid email/phone format, missing required fields)
  - 409 Conflict: Lead already exists with this email or phone
  - 500 Server Error: Database error

**Validation Rules:**
- **Name:** Required, 1-100 characters
- **Email:** Required, valid email format (must be unique)
- **Phone:** Required, E.164 format (must be unique)
- **Education:** Optional, max 200 characters
- **Lead Source:** Optional, must be "website", "referral", or empty

**Counselor Assignment:**
- Automatically assigns available counselor based on lead source
- Website leads: Assigned from any available counselor
- Referral leads: Assigned from counselors with `is_referral_enabled = true`
- Counselor selected by lowest assigned_count (round-robin distribution)

### 2. Counselor Management

#### Assign Counselor to Leads by Source
**POST** `/assign-counsellor`
- **Request:**
  ```json
  {"lead_source": "Website"}
  ```
- **Response (200 OK):**
  ```json
  {"message": "Counsellors assigned successfully"}
  ```

### 3. Meeting Management

#### Schedule Meet
**POST** `/schedule-meet`
- **Request:**
  ```json
  {"student_id": 1}
  ```
- **Response (200 OK):**
  ```json
  {"meet_link": "https://meet.google.com/abc-defg-hij"}
  ```

### 4. Payment Management

#### Initiate Payment
**POST** `/initiate-payment`
- **Request:**
  ```json
  {"student_id": 1, "amount": 50000.00}
  ```
- **Response (200 OK):**
  ```json
  {
    "order_id": "order_Ju2n5n1nWPVL2Z",
    "amount": 50000.00,
    "currency": "INR",
    "receipt": "rcpt_1"
  }
  ```
- **Kafka Event:** `payment.initiated`

#### Verify Payment
**POST** `/verify-payment`
- **Request:**
  ```json
  {
    "order_id": "order_Ju2n5n1nWPVL2Z",
    "payment_id": "pay_Ju2n56TryLkl2Z",
    "razorpay_signature": "9ef4dffbfd84f1318f6739a3ce19f9d85851857ae648f114332d8401e0949a3d"
  }
  ```
- **Response (200 OK):**
  ```json
  {"message": "Payment verified successfully", "status": "PAID", "student_id": "1", "order_id": "order_Ju2n5n1nWPVL2Z"}
  ```
- **Kafka Event:** `payment.verified`

### 5. Application Action
**POST** `/application-action`
- **Request:**
  ```json
  {"student_id": 1, "status": "ACCEPTED"}
  ```
- **Response (200 OK):**
  - For ACCEPTED status:
    ```json
    {"message": "Application accepted and offer letter sent to John Doe"}
    ```
  - For REJECTED status:
    ```json
    {"message": "Application rejected and notification sent to John Doe"}
    ```
  ```

### 6. Static Files
**GET** `/static/{filename}`
- Serves static files (e.g., `/static/test-payment.html`)

## Kafka Integration

### Overview
Kafka enables event-driven architecture for real-time data streaming. Events are published for lead creation and payment lifecycle.

### Implementation
- **Service:** `http/services/kafka.go` - Thread-safe producer with retry logic (3 attempts, exponential backoff).
- **Initialization:** Producer initialized at server startup.
- **Events Published:**
  - `lead.created`: On lead upload/create.
  - `payment.initiated`: On payment order creation.
  - `payment.verified`: On payment verification.

### Event Payloads
```json
// lead.created
{
  "event": "lead.created",
  "lead_id": 1,
  "name": "John Doe",
  "email": "john@example.com",
  "phone": "+919876543210",
  "education": "Bachelor's",
  "lead_source": "Website",
  "ts": "2025-11-13T10:30:45Z"
}

// payment.initiated
{
  "event": "payment.initiated",
  "student_id": 1,
  "order_id": "order_12345abc",
  "amount": 50000,
  "currency": "INR",
  "status": "PENDING",
  "ts": "2025-11-13T10:40:10Z"
}

// payment.verified
{
  "event": "payment.verified",
  "student_id": 1,
  "order_id": "order_12345abc",
  "payment_id": "pay_12345def",
  "status": "PAID",
  "ts": "2025-11-13T10:42:55Z"
}
```

### Local Development
1. Start Kafka: `docker-compose up -d`
2. Consume events:
   ```bash
   docker exec -it <kafka-container> kafka-console-consumer.sh --bootstrap-server localhost:9092 --topic admissions.payments --from-beginning
   ```

### Error Handling
- Non-blocking: Server continues if Kafka fails.
- Retry: 3 attempts with backoff.
- Monitoring: `IsConnected()` for status.

## Database Schema

### Current Tables

```sql
CREATE TABLE leads (
  id SERIAL PRIMARY KEY,
  name VARCHAR(255) NOT NULL,
  email VARCHAR(255) NOT NULL UNIQUE,
  phone VARCHAR(20) NOT NULL UNIQUE,
  education VARCHAR(255),
  lead_source VARCHAR(50),
  counsellor_id BIGINT REFERENCES counsellors(id) ON DELETE SET NULL,
  payment_status VARCHAR(50),
  meet_link TEXT,
  application_status VARCHAR(50),
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  last_modified_by VARCHAR(100),
  notes TEXT
);

CREATE TABLE counsellors (
  id SERIAL PRIMARY KEY,
  name VARCHAR(255) NOT NULL,
  email VARCHAR(255),
  phone VARCHAR(20),
  assigned_count INTEGER DEFAULT 0,
  max_capacity INTEGER NOT NULL,
  is_referral_enabled BOOLEAN DEFAULT true,
  created_at TIMESTAMP,
  updated_at TIMESTAMP
);

CREATE TABLE payments (
  id SERIAL PRIMARY KEY,
  student_id INTEGER NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
  amount DECIMAL(10, 2),
  status VARCHAR(50),
  order_id VARCHAR(255) UNIQUE,
  payment_id VARCHAR(255),
  razorpay_sign VARCHAR(255),
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE lead_history (
  id SERIAL PRIMARY KEY,
  lead_id INTEGER NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
  field_name VARCHAR(50) NOT NULL,
  old_value TEXT,
  new_value TEXT,
  changed_by VARCHAR(100),
  changed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE VIEW counsellor_workload AS
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
```

### Indexes for Performance

```sql
CREATE INDEX idx_leads_email ON leads(email);
CREATE INDEX idx_leads_phone ON leads(phone);
CREATE INDEX idx_leads_created_at ON leads(created_at);
CREATE INDEX idx_leads_counsellor_id ON leads(counsellor_id);
CREATE INDEX idx_counsellors_assignment ON counsellors(assigned_count, id) WHERE assigned_count < max_capacity;
CREATE INDEX idx_lead_history_lead_id ON lead_history(lead_id);
CREATE INDEX idx_lead_history_changed_at ON lead_history(changed_at);
```

## Database Migrations

### Latest Migration: Counselor Assignment Improvement
**File:** `migrations/001_improve_counsellor_assignment.sql`

**Changes:**
- Add `is_referral_enabled` column to counselors
- Create performance indexes for fast queries
- Add optional audit tracking (lead_history table)
- Create counselor_workload monitoring view

**Running Migration:**
```bash
# Using psql
psql -U username -d database_name -f migrations/001_improve_counsellor_assignment.sql

# Or execute the SQL file in your database client
```

**Data Consistency Check:**
```sql
-- Verify assigned_count matches actual leads
SELECT 
    c.id, c.name, c.assigned_count as recorded_count,
    COUNT(l.id) as actual_count,
    c.assigned_count - COUNT(l.id) as difference
FROM counsellors c
LEFT JOIN leads l ON l.counsellor_id = c.id
GROUP BY c.id, c.name, c.assigned_count
HAVING c.assigned_count != COUNT(l.id);
```

**Fix Inconsistencies:**
```sql
UPDATE counsellors c
SET assigned_count = (
    SELECT COUNT(*) FROM leads l WHERE l.counsellor_id = c.id
);
```

## Running the Application

### Development Setup
1. **Navigate to project root:**
   ```bash
   cd admission-module
   ```

2. **Install dependencies:**
   ```bash
   go mod tidy
   ```

3. **Configure environment:**
   - Create/update `.env` file with your database and service credentials
   - See Configuration section above for all environment variables

4. **Apply database migrations:**
   ```bash
   psql -U username -d database_name -f migrations/001_improve_counsellor_assignment.sql
   ```

5. **Start dependencies (optional):**
   ```bash
   docker-compose up -d
   ```

6. **Run server in development:**
   ```bash
   go run ./cmd/server
   ```

### Production Build

**Build executable:**
```bash
go build -o admission-server.exe ./cmd/server
```

**Run with environment variables:**
```bash
# Windows PowerShell
$env:DB_HOST="your-db-host"
$env:DB_PORT="5432"
$env:DB_USER="postgres"
$env:DB_PASSWORD="your-password"
$env:DB_NAME="admission_db"
./admission-server.exe

# Or using .env file
# Ensure .env is in the same directory as executable
./admission-server.exe
```

### Docker Deployment

**Build Docker image:**
```bash
docker build -t admission-module:latest .
```

**Run container:**
```bash
docker run -p 8080:8080 \
  -e DB_HOST=postgres \
  -e DB_PORT=5432 \
  -e DB_USER=postgres \
  -e DB_PASSWORD=secret \
  -e DB_NAME=admission_db \
  -e KAFKA_BROKERS=kafka:9092 \
  admission-module:latest
```

## Testing

### Using Postman
1. Import `admission_module_postman_collection.json` into Postman
2. Update variables (base URL, student_id, etc.) as needed
3. Execute test requests

### cURL Examples

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
curl -X GET "http://localhost:8080/leads"
```

**Get Leads with Filter:**
```bash
curl -X GET "http://localhost:8080/leads?created_after=2025-11-17T00:00:00Z"
```

**Upload Leads from Excel:**
```bash
curl -X POST http://localhost:8080/upload-leads \
  -F "file=@leads.xlsx"
```

**Initiate Payment:**
```bash
curl -X POST http://localhost:8080/initiate-payment \
  -H "Content-Type: application/json" \
  -d '{"student_id": 1, "amount": 50000}'
```

### Unit Tests
```bash
go test ./...
```

### Integration Tests
```bash
# Run tests with coverage
go test -v -cover ./...

# Generate coverage report
go test -v -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Monitoring & Debugging

### Server Logs
- Server logs output to terminal
- Look for:
  - Startup messages (database connection, Kafka initialization)
  - Request logs (timestamp, method, path, status)
  - Error logs (with context and stack traces)

### Health Check
```bash
curl -X GET http://localhost:8080/leads
# Returns structured response with status "success"
```

### Database Connections
```bash
# Check PostgreSQL connection
psql -U postgres -d admission_db -c "SELECT version();"

# Check leads count
psql -U postgres -d admission_db -c "SELECT COUNT(*) FROM leads;"
```

### Kafka Monitoring
```bash
# View Docker logs
docker-compose logs -f kafka

# Connect to Kafka container and list topics
docker exec -it <kafka-container> kafka-topics.sh --bootstrap-server localhost:9092 --list

# Consume events from topic
docker exec -it <kafka-container> kafka-console-consumer.sh \
  --bootstrap-server localhost:9092 \
  --topic admissions.payments \
  --from-beginning
```

## Architecture

### Service-Oriented Design
The application follows a service-oriented architecture with clear separation of concerns:

```
HTTP Request → Handler (method validation) → Service → Database
              ↓ (validation)      ↓ (transaction)
              Logging & Response  Persistence
```

### LeadService Structure
```go
type LeadService struct {
    db *sql.DB  // Database connection
}

// Methods
- UploadLeads(w, r)          // Bulk lead import from Excel
- GetLeads(w, r)             // Retrieve leads with filters
- CreateLead(w, r)           // Create single lead
- processAndInsertLead()     // Transaction-safe processing
- assignCounsellorTx()       // Counselor assignment logic
- deduplicateLeads()         // Remove duplicates from upload
```

### Response Types
All API responses use structured types:

**LeadResponse** (for individual leads)
```go
type LeadResponse struct {
    ID                int    `json:"id"`
    Name              string `json:"name"`
    Email             string `json:"email"`
    Phone             string `json:"phone"`
    Education         string `json:"education"`
    LeadSource        string `json:"lead_source"`
    PaymentStatus     string `json:"payment_status"`
    MeetLink          string `json:"meet_link"`
    ApplicationStatus string `json:"application_status"`
    CreatedAt         string `json:"created_at"`      // RFC3339 format
    UpdatedAt         string `json:"updated_at"`      // RFC3339 format
}
```

**GetLeadsResponse** (for list endpoints)
```go
type GetLeadsResponse struct {
    Status  string            `json:"status"`        // "success"
    Message string            `json:"message"`       // Descriptive message
    Count   int               `json:"count"`         // Number of records
    Data    []LeadResponse    `json:"data"`          // Array of leads
}
```

### Middleware & Security
- **CORS:** Configured for cross-origin requests
- **Content-Type:** Enforced (application/json for API endpoints)
- **Input Validation:** Regex patterns for email and phone
- **SQL Injection:** Protected via parameterized queries
- **Rate Limiting:** Configurable per deployment

## Validation & Business Rules

### Email Validation
```regex
^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$
```
- Valid: `user@example.com`, `test.user+tag@domain.co.uk`
- Invalid: `invalid.email@`, `@nodomain.com`

### Phone Validation (E.164 Format)
```regex
^\+?[1-9]\d{1,14}$
```
- Valid: `+919876543210`, `+12025550123`, `+441234567890`
- Invalid: `9876543210`, `(123) 456-7890`, `+1`, `+0123456789`

**Note:** E.164 format is the international standard:
- Country code (1-3 digits)
- Leading + (optional in regex but recommended)
- Max 15 total digits
- No spaces, dashes, or special characters

### Lead Source Constants
```go
// Valid values
constants.SourceWebsite   // "website"
constants.SourceReferral  // "referral"
// Empty string is also allowed (optional field)
```

## Error Handling
All errors follow consistent format:
```json
{
  "error": "Descriptive error message"
}
```

**HTTP Status Codes:**
- `200 OK` - Successful GET request
- `201 Created` - Successful resource creation
- `400 Bad Request` - Validation error, invalid JSON, or missing required fields
- `405 Method Not Allowed` - Wrong HTTP method used
- `409 Conflict` - Resource already exists (duplicate lead)
- `500 Internal Server Error` - Database or server error

**Common Error Responses:**

1. **Validation Errors (400)**
   ```json
   {
     "error": "invalid phone format (use E.164 format, e.g., +919876543210)"
   }
   ```

2. **Duplicate Lead (409)**
   ```json
   {
     "error": "lead already exists with this email or phone"
   }
   ```

3. **Invalid JSON (400)**
   ```json
   {
     "error": "Invalid JSON: unexpected EOF"
   }
   ```

## Transaction Safety & Concurrency

### Lead Creation Process
All lead operations (upload/create) are transaction-safe:

1. **Begins transaction** with `ReadCommitted` isolation level
2. **Validates lead data** (email, phone, required fields)
3. **Checks for duplicates** within transaction
4. **Assigns counselor** with row-level locking (`FOR UPDATE SKIP LOCKED`)
5. **Inserts lead** record
6. **Updates counselor** count atomically
7. **Commits transaction** on success, **rolls back** on any error

**Benefits:**
- ✅ Prevents race conditions during concurrent lead creation
- ✅ Ensures counselor capacity constraints are respected
- ✅ Automatic rollback on any error (no partial updates)
- ✅ Non-blocking with `SKIP LOCKED` (no lock contention)

### Counselor Assignment Algorithm
```sql
-- Website leads: Any available counselor
SELECT id FROM counsellors 
WHERE assigned_count < max_capacity 
ORDER BY assigned_count ASC, id ASC 
LIMIT 1 FOR UPDATE SKIP LOCKED

-- Referral leads: Only referral-enabled counselors
SELECT id FROM counsellors 
WHERE is_referral_enabled = true 
AND assigned_count < max_capacity 
ORDER BY assigned_count ASC, id ASC 
LIMIT 1 FOR UPDATE SKIP LOCKED
```

**Distribution Strategy:**
- Round-robin by `assigned_count` (lowest first)
- Deterministic by `id` (consistent results)
- Lock-free with `SKIP LOCKED` (prevents blocking)
- Transaction-safe with `FOR UPDATE` (prevents race conditions)

### Bulk Upload Deduplication
- **Within file:** Removes duplicate email+phone combinations
- **Database:** Enforces unique constraints on email and phone
- **Detailed feedback:** Returns row number and reason for each failure
- **Atomic operations:** Each lead processed in separate transaction

## Troubleshooting

### Common Issues
- **DB Connection:** Verify PostgreSQL running and credentials.
- **Kafka Refused:** Start with `docker-compose up -d`, wait 10s.
- **Port In Use:** Change port in `main.go` or kill process.
- **Module Errors:** Run `go mod tidy`.

### Kafka Issues
- Ensure `KAFKA_BROKERS` set correctly.
- Check Docker network.

## Future Enhancements
- Add more events (counselor assignments, application changes).
- Schema registry for events.
- Consumer applications (notifications, analytics).
- Monitoring with Prometheus/Grafana.
- SASL/SSL for Kafka.
- Circuit breaker for outages.
