# Devnet Mode - Critical Warnings

## ⚠️ DO NOT USE DEVNET FOR PRODUCTION-LIKE TESTING

### What is Devnet Mode?

Devnet mode (`accumulated run devnet`) runs **all validators in a single process** for quick local development and consensus testing.

### Critical Limitations

1. **Single Process Architecture**
   - All 12+ validators run in ONE process
   - Not representative of distributed systems
   - Cannot test real-world failure scenarios

2. **Missing HTTP API Services**
   - Only bootstrap node gets HTTP service (metrics port only)
   - BVN validator nodes do NOT expose HTTP APIs
   - Cannot test API endpoints (ports 8080-8083)
   - Health checks fail because APIs never start

3. **No Realistic Testing**
   - Cannot test network partitions
   - Cannot test individual node failures
   - Cannot test network latency
   - Cannot test resource constraints per node

### When to NEVER Use Devnet

❌ Load testing
❌ Performance testing
❌ Production-like testing
❌ API endpoint testing
❌ Multi-node behavior testing
❌ Failure scenario testing
❌ Docker deployments expecting real nodes

### When Devnet is Acceptable

✅ User explicitly requests "devnet" mode
✅ Quick consensus logic testing
✅ Throw-away local development
✅ Internal protocol testing

### Proper Alternatives

#### For Load Testing
Use **distributed Docker deployment**:
```bash
docker compose -f test/docker/docker-compose.distributed.yml up
```
This creates 12 separate containers with proper HTTP APIs.

#### For Production-Like Testing
Deploy individual nodes with proper configuration:
```bash
accumulated run --node=0 --work-dir=/path/to/node0
```

#### For Kubernetes
Use separate pods per validator with proper service exposure.

## History

This warning was added after discovering devnet mode was incorrectly used for load testing, resulting in:
- Hours of debugging why APIs weren't accessible
- Confusion about single vs. multi-container deployment
- Misunderstanding of network architecture

Issue #3862 - March 2026
