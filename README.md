# APIGate Proxy: client Integration Guide

This is the official, high-performance proxy server for connecting your applications to the **APIGate Decision Engine**.

As an APIGate customer, you should deploy this lightweight service within your infrastructure (e.g., as a sidecar, a separate container, or a background process). It acts as a secure buffer between your application and the APIGate cloud.

## 🎯 Why deploy this proxy?

Instead of integrating APIGate directly into your app, pointing your traffic to this local proxy gives you immediate benefits:

1.  **🚀 Instant Latency (Nanoseconds)**
    *   The proxy locally caches allows/blocks.
    *   It intelligently "pre-fetches" decisions for active users in the background.
    *   **Result**: Your app gets instant decisions without waiting for a network round-trip to our cloud.

2.  **🔒 Privacy & Security**
    *   **Local Encryption**: User emails are hashed (HMAC-SHA256) inside this proxy using *your* private key.
    *   **Result**: Raw user emails **never** leave your infrastructure. APIGate only sees anonymized hashes.

3.  **⚡ Optimization & Stability**
    *   **Smart Batching**: Groups multiple requests into single network calls, reducing overhead.
    *   **Resilience**: If the internet connection flickers, the proxy gracefully handles fallbacks so your app stays online.

---

## 🛠 Deployment Guide

### 1. Requirements
*   A server or container running Linux/macOS/Windows.
*   **Go 1.21+** (if building from source).
*   Your **Project API Key** (from the [APIGate Dashboard](https://apigate.in) > Settings).

### 2. Installation

Clone the repository to your server:
```bash
git clone https://github.com/apigate-in/apigate-proxy.git
cd apigate-proxy
```

### 3. Configuration

Create a `.env` file from the sample:
```bash
cp .env.sample .env
```

Edit the `.env` file with your specific credentials:

```ini
# Port for your app to connect to (default: 8080)
PORT=8080

# The APIGate Cloud Endpoint (Do not change)
UPSTREAM_BASE_URL=https://api.apigate.in

# Your Project API Key
UPSTREAM_API_KEY=your_project_api_key_here

# 🔐 SECURITY CRITICAL:
# Set a random 32-character string here. Keep it secret.
# This key is used to hash user emails locally.
# If you lose this, you cannot look up previous user logs by email.
EMAIL_ENCRYPTION_KEY=change_this_to_a_secure_random_string
```

### 4. Start the Service

```bash
go run main.go
# OR build a binary
go build -o apigate-proxy . && ./apigate-proxy
```

---

## 🔌 Connecting Your Application

Once the proxy is running (e.g., at `localhost:8080`), update your application to query the proxy instead of the APIGate cloud directly.

### API Specification

**Endpoint**: `POST /api/allow`

**Request Body**:
```json
{
  "ip_address": "192.168.1.50",
  "email": "user@customer.com",
  "user_agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)..."
}
```

**Response**:
```json
{
  "allow": true,
  "status": "success",
  "message": "Allowed (Live Check)"
}
```

### Example (Node.js)

```javascript
const axios = require('axios');

async function checkUser(ip, email, ua) {
  try {
    // Connect to LOCAL proxy, not the cloud
    const response = await axios.post('http://localhost:8080/api/allow', {
      ip_address: ip,
      email: email,
      user_agent: ua
    });
    
    if (!response.data.allow) {
      throw new Error("Action Blocked by APIGate");
    }
    
    return true; // proceed
  } catch (error) {
    console.error("Check failed:", error.message);
    return false;
  }
}
```

---

## 📡 Logging

APIGate provides detailed analytics and attack reports, but **you must send the traffic logs** for this to work.

**Why use this endpoint?**
While direct API calls are async, sending a request for every user action can quickly hit API rate limits. This proxy **batches** your logs efficiently and uploads them in bulk, preventing rate-limit errors and reducing network overhead.

**Endpoint**: `POST /api/log`

**When to call**: Call this endpoint after every request your application handles (or asynchronously).

**Payload**:

```json
{
  "ip_address": "192.168.1.5",
  "email": "user@example.com",
  "user_agent": "Mozilla/5.0...",
  "http_method": "POST",
  "endpoint": "/v1/login",
  "response_code": 200,
  "track_request": true
}
```


---

## 🔐 Utilities

### Email Privacy Helper
If you need to manually encrypt an email to match what is stored in APIGate (e.g. for debugging or manual lookups), you can use this helper endpoint.

**Endpoint**: `GET /api/encrypt-email`

**Query Parameters**:
*   `email`: The email address to encrypt.

**Example Request**:
`GET http://localhost:8080/api/encrypt-email?email=test@example.com`

**Response**:
```json
{
  "email": "test@example.com",
  "encrypted": "a57b1bd46defbcd6cd774817c30c4721"
}
```

---

## License

MIT License. Copyright (c) 2025 APIGate.
