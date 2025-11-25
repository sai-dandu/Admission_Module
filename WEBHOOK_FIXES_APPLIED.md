# Webhook Issues - Fixed & Troubleshooting

## Issues Fixed

### 1. **JSONB Query Issue** ✅
**Problem**: The original code used `payload->>'order_id'` in SQL which is PostgreSQL-specific and fragile.
**Solution**: Changed to use `webhook_id` directly for tracking, which is unique and reliable.

```go
// Before (BROKEN)
UPDATE razorpay_webhooks SET status = $1 WHERE payload->>'order_id' = $3

// After (FIXED)
UPDATE razorpay_webhooks SET status = $1 WHERE webhook_id = $3
```

### 2. **Function Signature Mismatch** ✅
**Problem**: `updateWebhookProcessingStatus()` didn't return an error, but callers tried to handle errors.
**Solution**: Changed return type to `error` and added proper error handling.

```go
// Before (BROKEN)
func updateWebhookProcessingStatus(orderID, processingStatus, errorMsg string)

// After (FIXED)
func updateWebhookProcessingStatus(webhookID, processingStatus, errorMsg string) error
```

### 3. **Missing Error Handling in Event Handlers** ✅
**Problem**: Webhook status updates were silently failing.
**Solution**: Added error checking and logging for all webhook status updates.

### 4. **Duplicate Webhook Processing** ✅
**Problem**: No protection against duplicate webhook IDs being inserted.
**Solution**: Added `ON CONFLICT (webhook_id) DO NOTHING` to insert statement.

```go
// Now handles duplicates gracefully
INSERT INTO razorpay_webhooks ... ON CONFLICT (webhook_id) DO NOTHING
```

### 5. **Error Message Truncation** ✅
**Problem**: Long error messages could exceed database column limits.
**Solution**: Added truncation to 500 characters before storing.

```go
if len(errorMsg) > 500 {
    errorMsg = errorMsg[:500]
}
```

### 6. **Payload JSON Encoding** ✅
**Problem**: Payloads were stored as `[]byte` instead of `string`.
**Solution**: Convert to string when storing in database.

```go
// Before
db.DB.Exec("...", payloadJSON)  // []byte

// After
db.DB.Exec("...", string(payloadJSON))  // string
```

## Verification Checklist

### Database Setup
```bash
# Check if razorpay_webhooks table exists
psql -U postgres -d admission-db -c \
  "SELECT table_name FROM information_schema.tables 
   WHERE table_name = 'razorpay_webhooks';"

# Expected output:
# table_name
# ──────────────────
# razorpay_webhooks
```

### Payment Table Columns
```bash
# Check if payment tables have required columns
psql -U postgres -d admission-db -c \
  "SELECT column_name FROM information_schema.columns 
   WHERE table_name = 'registration_payment' 
   AND column_name IN ('error_message', 'refund_id', 'refund_amount');"

# Should return 3 columns:
# column_name
# ─────────────────
# error_message
# refund_id
# refund_amount
```

### Environment Configuration
```bash
# Verify RAZORPAY_WEBHOOK_SECRET is set
# PowerShell:
echo $env:RAZORPAY_WEBHOOK_SECRET

# Bash:
echo $RAZORPAY_WEBHOOK_SECRET

# .env file should have:
RAZORPAY_WEBHOOK_SECRET=your_secret_from_razorpay_dashboard
```

## Testing Webhooks

### 1. **Create Test Payment Order**
```bash
curl -X POST http://localhost:8080/initiate-payment \
  -H "Content-Type: application/json" \
  -d '{
    "student_id": 1,
    "payment_type": "REGISTRATION"
  }'

# Response will include order_id, e.g.:
# {"order_id": "order_ABC123", ...}
```

### 2. **Generate Valid HMAC Signature**

For testing, you need to generate the exact HMAC-SHA256 signature:

```bash
#!/bin/bash

# Get your webhook secret from .env or Razorpay dashboard
SECRET="your_webhook_secret"

# Create the exact JSON payload
PAYLOAD='{"id":"evt_test","event":"payment.captured","created_at":1622592763,"contains":["payment"],"payload":{"payment":{"id":"pay_test","order_id":"order_ABC123","amount":187000,"currency":"INR","status":"captured","method":"upi"}}}'

# Generate HMAC-SHA256 signature
SIGNATURE=$(echo -n "$PAYLOAD" | openssl dgst -sha256 -hmac "$SECRET" | cut -d' ' -f2)

echo "Signature: $SIGNATURE"
```

### 3. **Send Webhook with Valid Signature**
```bash
#!/bin/bash

SECRET="your_webhook_secret"
PAYLOAD='{"id":"evt_test","event":"payment.captured","created_at":1622592763,"contains":["payment"],"payload":{"payment":{"id":"pay_test","order_id":"order_ABC123","amount":187000,"currency":"INR","status":"captured"}}}'
SIGNATURE=$(echo -n "$PAYLOAD" | openssl dgst -sha256 -hmac "$SECRET" | cut -d' ' -f2)

curl -X POST http://localhost:8080/razorpay/webhook \
  -H "Content-Type: application/json" \
  -H "X-Razorpay-Signature: $SIGNATURE" \
  -d "$PAYLOAD"

# Expected response (200 OK):
# {"status":"processed","event":"payment.captured","order_id":"order_ABC123","payment_id":"pay_test"}
```

### 4. **Verify Webhook Was Processed**
```bash
# Check webhook logs
psql -U postgres -d admission-db -c \
  "SELECT webhook_id, event_type, status, error_message 
   FROM razorpay_webhooks 
   ORDER BY created_at DESC LIMIT 5;"

# Check payment status updated
psql -U postgres -d admission-db -c \
  "SELECT status, order_id, payment_id 
   FROM registration_payment 
   WHERE order_id = 'order_ABC123';"

# Should show status = 'PAID'
```

## Common Problems & Solutions

### Problem 1: "Invalid webhook signature"
**Cause**: Signature mismatch  
**Solution**:
```bash
# 1. Verify RAZORPAY_WEBHOOK_SECRET matches Razorpay dashboard
# 2. Check .env file:
cat .env | grep RAZORPAY_WEBHOOK_SECRET

# 3. Verify payload is NOT modified after receiving
# 4. Test with the exact payload and secret combination
```

### Problem 2: "payment not found for order_id"
**Cause**: Order doesn't exist in database  
**Solution**:
```bash
# 1. Verify payment was created first
psql -U postgres -d admission-db -c \
  "SELECT * FROM registration_payment WHERE order_id = 'order_ABC123';"

# 2. If empty, first create payment via /initiate-payment endpoint
# 3. Then send webhook with correct order_id
```

### Problem 3: "table razorpay_webhooks does not exist"
**Cause**: Database migration not run  
**Solution**:
```bash
# Run migration 006
psql -U postgres -d admission-db < db/migrations/006_create_webhooks_table.sql

# Verify table created
psql -U postgres -d admission-db -c \
  "SELECT table_name FROM information_schema.tables 
   WHERE table_name = 'razorpay_webhooks';"
```

### Problem 4: "column error_message does not exist"
**Cause**: Database columns not added  
**Solution**:
```bash
# Run migration 005
psql -U postgres -d admission-db < db/migrations/005_add_refund_tracking.sql

# Verify columns added
psql -U postgres -d admission-db -c \
  "SELECT column_name FROM information_schema.columns 
   WHERE table_name = 'registration_payment';"
```

### Problem 5: Webhook processed but payment not updated
**Cause**: Transaction failed or error in processPaymentCaptured  
**Solution**:
```bash
# Check webhook logs for errors
psql -U postgres -d admission-db -c \
  "SELECT error_message FROM razorpay_webhooks 
   WHERE event_type = 'payment.captured' 
   ORDER BY created_at DESC LIMIT 1;"

# Check payment status
psql -U postgres -d admission-db -c \
  "SELECT status, error_message FROM registration_payment 
   WHERE order_id = 'order_ABC123';"

# Look at server logs
tail -f logs/server.log | grep -i webhook
```

### Problem 6: Duplicate webhook processing
**Cause**: Same webhook received multiple times  
**Solution**:
✅ **Already fixed!** The webhook processing is now idempotent:
- Webhook ID is unique (prevents duplicate DB inserts)
- Payment status check before updating (ignores if already PAID)
- No duplicate Kafka events published

## Complete Testing Flow

```bash
#!/bin/bash

set -e  # Exit on error

echo "=== Webhook Testing Flow ==="

# 1. Create payment order
echo "1. Creating payment order..."
RESPONSE=$(curl -s -X POST http://localhost:8080/initiate-payment \
  -H "Content-Type: application/json" \
  -d '{"student_id": 1, "payment_type": "REGISTRATION"}')

ORDER_ID=$(echo $RESPONSE | grep -o '"order_id":"[^"]*' | cut -d'"' -f4)
echo "   Order ID: $ORDER_ID"

# 2. Check payment status (should be PENDING)
echo "2. Checking initial payment status..."
STATUS=$(psql -U postgres -d admission-db -t -c \
  "SELECT status FROM registration_payment WHERE order_id = '$ORDER_ID';")
echo "   Status: $STATUS (expected: PENDING)"

# 3. Send webhook
echo "3. Sending webhook..."
SECRET="your_webhook_secret"
PAYLOAD="{\"id\":\"evt_test\",\"event\":\"payment.captured\",\"created_at\":1622592763,\"contains\":[\"payment\"],\"payload\":{\"payment\":{\"id\":\"pay_test\",\"order_id\":\"$ORDER_ID\",\"amount\":187000,\"currency\":\"INR\",\"status\":\"captured\"}}}"
SIGNATURE=$(echo -n "$PAYLOAD" | openssl dgst -sha256 -hmac "$SECRET" | cut -d' ' -f2)

curl -s -X POST http://localhost:8080/razorpay/webhook \
  -H "Content-Type: application/json" \
  -H "X-Razorpay-Signature: $SIGNATURE" \
  -d "$PAYLOAD"

# 4. Check payment status (should be PAID)
echo "4. Checking payment status after webhook..."
STATUS=$(psql -U postgres -d admission-db -t -c \
  "SELECT status FROM registration_payment WHERE order_id = '$ORDER_ID';")
echo "   Status: $STATUS (expected: PAID)"

# 5. Check webhook logs
echo "5. Checking webhook logs..."
psql -U postgres -d admission-db -c \
  "SELECT webhook_id, event_type, status, error_message 
   FROM razorpay_webhooks 
   WHERE event_type = 'payment.captured' 
   ORDER BY created_at DESC LIMIT 1;"

echo "=== Test Complete ==="
```

## Monitoring

### Real-Time Webhook Logs
```bash
# Follow webhook processing
tail -f logs/server.log | grep -i "webhook\|payment"
```

### Database Queries for Monitoring
```bash
# Recent webhooks
psql -U postgres -d admission-db -c \
  "SELECT webhook_id, event_type, status, created_at 
   FROM razorpay_webhooks 
   ORDER BY created_at DESC LIMIT 10;"

# Failed webhooks
psql -U postgres -d admission-db -c \
  "SELECT webhook_id, event_type, error_message, created_at 
   FROM razorpay_webhooks 
   WHERE status = 'FAILED' 
   ORDER BY created_at DESC LIMIT 5;"

# Payment status summary
psql -U postgres -d admission-db -c \
  "SELECT status, COUNT(*) as count 
   FROM registration_payment 
   GROUP BY status;"
```

## Production Deployment

Before going to production:

1. **✅ Verify all database tables and columns exist**
   ```bash
   psql -U postgres -d admission-db -c "\dt razorpay*"
   ```

2. **✅ Verify webhook secret is set correctly**
   ```bash
   echo $env:RAZORPAY_WEBHOOK_SECRET  # Should not be empty
   ```

3. **✅ Configure webhook URL in Razorpay Dashboard**
   - Settings → Webhooks → Add Webhook
   - URL: `https://yourdomain.com/razorpay/webhook`
   - Secret: From RAZORPAY_WEBHOOK_SECRET

4. **✅ Test with Razorpay test webhooks**
   - Razorpay Dashboard has "Test" button for each webhook

5. **✅ Enable logging and monitoring**
   - Monitor `/razorpay/webhook` endpoint
   - Alert on high failure rates
   - Check error_message column for details

6. **✅ Test with actual payments (sandbox mode)**
   - Create test card in Razorpay
   - Complete full payment flow
   - Verify webhook processing

---

**All webhook issues are now fixed!** The implementation is production-ready.

