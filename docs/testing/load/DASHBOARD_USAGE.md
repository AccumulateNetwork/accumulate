# Gap Recovery Dashboard - Usage Guide

## 🚀 Quick Start

### Launch Dashboard Only (Recommended)
```bash
go run test/load/load_test_with_pause.go -dashboard -open
```
This will:
- Start the dashboard on http://localhost:8080
- Open it in your browser automatically
- NOT generate automatic load (you control everything)

### Launch with Automatic Load Generation
```bash
go run test/load/load_test_with_pause.go -open
```
This will:
- Start automatic transaction generation
- Monitor gaps automatically
- Open dashboard in browser

### Custom Port
```bash
go run test/load/load_test_with_pause.go -dashboard -port 8888
```

## 📊 Dashboard Features

### Real-Time Metrics
- **Current TPS**: Transactions per second
- **Total Transactions**: Running count
- **Successful**: Successfully processed
- **Cross-Chain**: Cross-partition transactions
- **Active Gaps**: Currently detected gaps
- **Gaps Recovered**: Total recovered gaps

### Partition Controls
Each partition (DN, BVN0, BVN1, BVN2) has:
- **Status Indicator**: Green (active) or Red (paused)
- **Pause Button**: Simulates network isolation
- **Resume Button**: Restores connectivity

### Manual Testing
- **Generate Test Transactions**: Creates 100 test transactions
- **Event Log**: Shows all pause/resume/gap events

## 🧪 Testing Gap Recovery

### Step 1: Start Dashboard
```bash
go run test/load/load_test_with_pause.go -dashboard -open
```

### Step 2: Generate Some Traffic
Click "🚀 Generate Test Transactions" button a few times

### Step 3: Create a Gap
1. Click "⏸️ Pause" on BVN0
2. Wait 10-30 seconds (other partitions continue)
3. Click "▶️ Resume" on BVN0

### Step 4: Observe Recovery
- Watch "Active Gaps" increase when paused
- See "Gaps Recovered" increment after resume
- Check Event Log for gap detection messages

## 🎮 Interactive Controls

### Partition States
- **Green Card + Green Dot**: Partition is active
- **Red Card + Red Pulsing Dot**: Partition is paused

### Button Actions
- **Pause**: Drops all CCC messages (in/out)
- **Resume**: Restores message flow, triggers recovery
- **Generate Test Transactions**: Adds 100 transactions

### TPS Chart
- Shows last 60 seconds of TPS history
- Updates every second
- Useful for seeing impact of pauses

## 🔍 What to Look For

### When You Pause a Partition:
1. Status indicator turns red and pulses
2. Event log shows "⏸️ Paused [partition]"
3. Active gaps counter may increase
4. TPS may drop (if pausing major partition)

### When You Resume a Partition:
1. Status indicator turns green
2. Event log shows "▶️ Resumed [partition]"
3. Gap recovery begins automatically
4. "Gaps Recovered" counter increases

### Successful Gap Recovery:
- No messages lost (just delayed)
- Sequence integrity maintained
- Automatic recovery without retries

## ⚠️ Important Notes

1. **Requires Testnet Build**: The CCC pause endpoints only work with:
   ```bash
   go build -tags testnet
   ```

2. **Dashboard Port**: Default is 8080, change with `-port` flag

3. **Browser Compatibility**: Works best with Chrome, Firefox, or Safari

4. **Stop the Dashboard**: Press Ctrl+C in the terminal

## 📝 Example Test Scenario

```bash
# Terminal 1: Start devnet with testnet build
./test/load/devnet_manager.sh restart

# Terminal 2: Launch dashboard
go run test/load/load_test_with_pause.go -dashboard -open

# In Browser:
# 1. Click "Generate Test Transactions" 5 times
# 2. Pause BVN0
# 3. Wait 20 seconds
# 4. Resume BVN0
# 5. Watch gaps recover in metrics
```

The dashboard provides real-time feedback on gap creation and recovery!