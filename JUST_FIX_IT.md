# Just Fix It - No More Plans

## The Requirement
Destination requests height X → Source adjusts "height last sent" to X-1

## The Fix

### 1. Add height tracking (1 field)
### 2. Adjust height on request (1 line)  
### 3. Use adjusted height when sending (find where)

## Implementation

### Add to conductor.go:
```go
lastSentHeight map[string]uint64  // destination -> last height sent
```

### Fix HandleRecoveryRequest:
```go
cc.lastSentHeight[req.Requester] = req.FromNumber - 1
```

### Wire to send logic:
```go
// Find where sending happens and use adjusted height
```

**That's it. Stop planning. Start fixing.**