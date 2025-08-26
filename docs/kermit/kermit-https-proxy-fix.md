# Kermit Healing Container HTTPS Proxy Fix

## Problem Summary

The Kermit healing containers (v1.4.0-alpha.5) are failing with "unexpected end of JSON input" errors because they expect the API to be available at:

```
https://kermit.accumulatenetwork.io/v3
```

However, the API server only responds on:

```
http://kermit-api.accumulate.defidevs.io:16692/v3
```

## Root Cause Analysis

1. **DNS Configuration**: `kermit.accumulatenetwork.io` is a CNAME pointing to `kermit-api.accumulate.defidevs.io`
2. **Port Status**:
   - Port 443 (HTTPS): Open but not serving API content
   - Port 16692: Open and serving working HTTP API
   - Port 80: Filtered
   - Port 8081: Filtered
3. **Version Mismatch**: Healing containers use old endpoint configuration

## Solution 1: HTTPS Reverse Proxy (Recommended)

Set up an HTTPS reverse proxy on port 443 to forward requests to the HTTP API server on port 16692.

### Option A: Nginx Reverse Proxy

1. **Install Nginx** (on kermit-api.accumulate.defidevs.io):
```bash
sudo apt update
sudo apt install nginx certbot python3-certbot-nginx -y
```

2. **Create Nginx Configuration**:
```bash
sudo tee /etc/nginx/sites-available/kermit-api << 'EOF'
server {
    listen 80;
    server_name kermit.accumulatenetwork.io kermit-api.accumulate.defidevs.io;
    
    # Redirect HTTP to HTTPS
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name kermit.accumulatenetwork.io kermit-api.accumulate.defidevs.io;
    
    # SSL configuration (will be managed by certbot)
    ssl_certificate /etc/letsencrypt/live/kermit.accumulatenetwork.io/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/kermit.accumulatenetwork.io/privkey.pem;
    
    # Proxy all requests to the local API server
    location / {
        proxy_pass http://127.0.0.1:16692;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # Handle JSON-RPC requests
        proxy_set_header Content-Type application/json;
        proxy_buffering off;
        proxy_request_buffering off;
    }
}
EOF
```

3. **Enable the Site**:
```bash
sudo ln -s /etc/nginx/sites-available/kermit-api /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
```

4. **Obtain SSL Certificate**:
```bash
sudo certbot --nginx -d kermit.accumulatenetwork.io -d kermit-api.accumulate.defidevs.io
```

5. **Test the Configuration**:
```bash
curl -s -X POST https://kermit.accumulatenetwork.io/v3 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"node-info","params":{},"id":1}' | jq .
```

### Option B: AWS Application Load Balancer

If running on AWS, configure an Application Load Balancer:

1. **Create Target Group**:
   - Target: EC2 instance on port 16692
   - Health check: `/v3` with POST method

2. **Create Load Balancer**:
   - Listener: HTTPS (443) → Target Group
   - SSL Certificate: Request/import certificate for `kermit.accumulatenetwork.io`

3. **Update DNS**:
   - Point `kermit.accumulatenetwork.io` CNAME to ALB DNS name

## Solution 2: Update Healing Containers (Alternative)

Update the healing containers to use a newer version with the correct endpoint.

### Step 1: Build Updated Container

```bash
# On the healing server
cd /path/to/accumulate
git checkout main  # or latest stable branch
docker build -t accumulate:latest .
```

### Step 2: Update Container Commands

```bash
# Stop existing containers
docker stop kermit-heal-anchors kermit-heal-synthetic
docker rm kermit-heal-anchors kermit-heal-synthetic

# Start with updated image
docker run -d --name kermit-heal-anchors --restart unless-stopped \
  -v "${HOME}/.accumulate/cache:/data" --entrypoint debug \
  accumulate:latest \
  heal anchor Kermit --max-response-age 5m \
  --cached-scan /data/kermit-bootstrap.json --peer-db /data/kermit-peerdb.json --continuous

docker run -d --name kermit-heal-synthetic --restart unless-stopped \
  -m 4g -v "${HOME}/.accumulate/cache:/data" --entrypoint debug \
  accumulate:latest \
  heal synth Kermit --cached-scan /data/kermit-bootstrap.json --peer-db /data/kermit-peerdb.json \
  --light-db /data/kermit.db --continuous --since 2h
```

## Validation

After implementing either solution, validate that the healing containers work:

1. **Test API Endpoint**:
```bash
curl -s -X POST https://kermit.accumulatenetwork.io/v3 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"node-info","params":{},"id":1}' | jq .
```

2. **Test Network Scan**:
```bash
/root/debug network scan Kermit -j > /tmp/test-kermit.json
cat /tmp/test-kermit.json | jq .
```

3. **Check Healing Container Logs**:
```bash
docker logs kermit-heal-anchors --tail 20
docker logs kermit-heal-synthetic --tail 20
```

4. **Verify Continuous Operation**:
```bash
# Should show containers running without restarts
docker ps | grep kermit-heal
```

## Monitoring

Set up monitoring to ensure the HTTPS proxy continues working:

```bash
# Add to crontab for regular health checks
*/5 * * * * curl -s -f https://kermit.accumulatenetwork.io/v3 > /dev/null || echo "API endpoint down" | mail -s "Kermit API Alert" admin@example.com
```

## Troubleshooting

### Common Issues

1. **SSL Certificate Errors**:
   - Ensure certificate covers both domains
   - Check certificate expiration
   - Verify nginx SSL configuration

2. **Proxy Timeouts**:
   - Increase nginx proxy timeouts
   - Check API server health on port 16692

3. **JSON-RPC Errors**:
   - Verify Content-Type headers are preserved
   - Check proxy buffering settings

### Debug Commands

```bash
# Check nginx status
sudo systemctl status nginx
sudo nginx -t

# Check SSL certificate
openssl s_client -connect kermit.accumulatenetwork.io:443 -servername kermit.accumulatenetwork.io

# Test direct API server
curl -s http://127.0.0.1:16692/v3 -X POST -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"node-info","params":{},"id":1}'

# Check port connectivity
nmap -p 443,16692 kermit-api.accumulate.defidevs.io
```

## Conclusion

**Recommendation**: Implement Solution 1 (HTTPS Reverse Proxy) as it maintains compatibility with existing healing containers while providing a robust, production-ready solution.

The HTTPS proxy approach ensures:
- ✅ Existing healing containers continue working without changes
- ✅ Proper SSL/TLS termination
- ✅ Scalable and maintainable solution
- ✅ Supports both domain names
- ✅ Can be monitored and managed independently
