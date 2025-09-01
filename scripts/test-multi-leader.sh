#!/bin/bash

echo "👑 Phase 1 Multi-Instance Leader Election Test"
echo "=============================================="

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Configuration
INSTANCES=3
BASE_PORT=8080
PIDS=()

# Cleanup function
cleanup() {
    echo -e "\n${BLUE}🧹 Cleaning up instances...${NC}"
    for pid in "${PIDS[@]}"; do
        kill $pid 2>/dev/null || true
    done
    wait 2>/dev/null || true
    echo -e "${GREEN}✅ All instances stopped${NC}"
}

# Trap cleanup on exit
trap cleanup EXIT

# Check if Redis is running
if ! podman exec redis-timeout-poc redis-cli ping >/dev/null 2>&1; then
    echo -e "${RED}❌ Redis not running. Starting...${NC}"
    podman run -d --name redis-timeout-poc -p 6379:6379 redis:7-alpine
    sleep 3
fi

# Clear Redis state
echo -e "${BLUE}🧹 Clearing Redis state...${NC}"
podman exec redis-timeout-poc redis-cli FLUSHDB >/dev/null

# Function to show leader status across all instances
show_leader_status() {
    echo -e "\n${BLUE}👑 Leader Election Status:${NC}"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    
    leader_count=0
    for i in $(seq 1 $INSTANCES); do
        port=$((BASE_PORT + i - 1))
        status=$(curl -s "http://localhost:$port/status" 2>/dev/null || echo '{"error":"not responding"}')
        
        is_leader=$(echo "$status" | jq -r '.is_leader // false' 2>/dev/null || echo "false")
        pod_id=$(echo "$status" | jq -r '.pod_id // "unknown"' 2>/dev/null || echo "unknown")
        
        if [ "$is_leader" = "true" ]; then
            echo -e "  Instance $i (Port $port): ${GREEN}👑 LEADER${NC} (Pod: $pod_id)"
            leader_count=$((leader_count + 1))
        else
            echo -e "  Instance $i (Port $port): ${YELLOW}📋 Follower${NC} (Pod: $pod_id)"
        fi
    done
    
    # Redis leader info
    redis_leader=$(podman exec redis-timeout-poc redis-cli GET timeout:leader 2>/dev/null || echo "(none)")
    redis_ttl=$(podman exec redis-timeout-poc redis-cli TTL timeout:leader 2>/dev/null || echo "-1")
    
    echo -e "\n${YELLOW}Redis Leader Key:${NC} $redis_leader"
    echo -e "${YELLOW}Leader TTL:${NC} $redis_ttl seconds"
    
    if [ $leader_count -eq 1 ]; then
        echo -e "\n${GREEN}✅ Exactly 1 leader (CORRECT)${NC}"
    elif [ $leader_count -eq 0 ]; then
        echo -e "\n${YELLOW}⚠️  No leaders found (transitioning...)${NC}"
    else
        echo -e "\n${RED}❌ Multiple leaders detected ($leader_count) - PROBLEM!${NC}"
    fi
    
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
}

# Function to wait for user input
wait_for_input() {
    echo -e "\n${YELLOW}Press Enter to continue...${NC}"
    read
}

echo -e "${BLUE}🚀 Starting $INSTANCES Phase 1 instances...${NC}"
echo ""

# Start multiple instances
for i in $(seq 1 $INSTANCES); do
    port=$((BASE_PORT + i - 1))
    pod_id="phase1-instance-$i"
    
    echo -e "${YELLOW}Starting instance $i on port $port (Pod: $pod_id)...${NC}"
    
    # Set environment for this instance
    env \
        REDIS_ADDR=localhost:6379 \
        TIMEOUT_INTERVAL_SECONDS=5 \
        LEADER_ELECTION_TTL_SECONDS=10 \
        LEADER_ELECTION_INTERVAL_SECONDS=3 \
        PORT=$port \
        LOG_LEVEL=info \
        POD_ID=$pod_id \
        ./bin/phase1 > "phase1-instance-$i.log" 2>&1 &
    
    PIDS+=($!)
    sleep 1
done

echo -e "${GREEN}✅ All instances started${NC}"
echo ""
echo "Instance details:"
for i in $(seq 1 $INSTANCES); do
    port=$((BASE_PORT + i - 1))
    echo "  Instance $i: http://localhost:$port (PID: ${PIDS[$((i-1))]})"
done

# Wait for instances to start
echo -e "\n${YELLOW}⏳ Waiting for instances to initialize...${NC}"
sleep 5

# Test 1: Initial Leader Election
echo -e "\n${BLUE}🎬 TEST 1: Initial Leader Election${NC}"
echo "=================================="
echo ""
echo "Multiple instances are competing for leadership..."

show_leader_status
wait_for_input

# Test 2: Leader Stability
echo -e "\n${BLUE}🎬 TEST 2: Leader Stability Over Time${NC}"
echo "===================================="
echo ""
echo "Monitoring leader stability for 30 seconds..."

for i in {1..10}; do
    echo -e "${YELLOW}⏱️  Check $i (${i}0s elapsed)${NC}"
    show_leader_status
    sleep 3
done

wait_for_input

# Test 3: Leader Failure Simulation
echo -e "\n${BLUE}🎬 TEST 3: Leader Failure & Re-election${NC}"
echo "======================================"
echo ""

# Find current leader
current_leader_port=""
for i in $(seq 1 $INSTANCES); do
    port=$((BASE_PORT + i - 1))
    status=$(curl -s "http://localhost:$port/status" 2>/dev/null || echo '{}')
    is_leader=$(echo "$status" | jq -r '.is_leader // false' 2>/dev/null || echo "false")
    
    if [ "$is_leader" = "true" ]; then
        current_leader_port=$port
        current_leader_instance=$i
        break
    fi
done

if [ -n "$current_leader_port" ]; then
    echo -e "${YELLOW}Current leader is Instance $current_leader_instance (Port: $current_leader_port)${NC}"
    echo -e "${RED}🔥 Simulating leader failure by killing it...${NC}"
    
    # Kill the leader
    leader_pid=${PIDS[$((current_leader_instance-1))]}
    kill $leader_pid 2>/dev/null || true
    
    echo -e "${YELLOW}⏳ Waiting for re-election...${NC}"
    sleep 5
    
    show_leader_status
    
    echo -e "\n${BLUE}✅ Re-election should have occurred with a new leader${NC}"
else
    echo -e "${RED}❌ No leader found to kill${NC}"
fi

wait_for_input

# Test 4: Add Agent Message and Test Timeout
echo -e "\n${BLUE}🎬 TEST 4: Timeout Processing with Multiple Instances${NC}"
echo "=================================================="
echo ""
echo "Only the leader should process timeouts, even with multiple instances running"

# Track a conversation
CONV_ID="multi_leader_test_$(date +%s)"
echo -e "${YELLOW}📝 Tracking conversation: $CONV_ID${NC}"

# Use any instance to track the message (API should work on all)
curl -X POST "http://localhost:$BASE_PORT/conversations/$CONV_ID/agent-message" \
  -H "Content-Type: application/json" \
  -d "{\"agent_id\":\"agent_multi\",\"message_id\":\"msg_$(date +%s)\"}" \
  | jq '.' 2>/dev/null || echo "Message tracked"

echo -e "\n${YELLOW}⏰ Monitoring timeout processing for 12 seconds...${NC}"
echo "Only the leader should send notifications"

for i in {1..12}; do
    echo -e "${BLUE}⏱️  Time: ${i}s${NC}"
    
    if [ $((i % 4)) -eq 0 ]; then
        show_leader_status
        
        # Check notification state
        level=$(podman exec redis-timeout-poc redis-cli HGET notification_states $CONV_ID 2>/dev/null || echo "0")
        echo -e "${YELLOW}    Notification level for $CONV_ID: $level${NC}"
    fi
    
    sleep 1
done

    show_leader_status
    
    wait_for_input
    
    # Test 5: Waiting Conversations Across Instances
    echo -e "\n${BLUE}🎬 TEST 5: Waiting Conversations Management${NC}"
    echo "=========================================="
    echo ""
    echo "Testing that waiting conversations are managed consistently across all instances"
    
    # Track multiple conversations through different instances
    echo -e "${YELLOW}📝 Tracking conversations through different instances...${NC}"
    
    CONV_IDS=()
    for i in $(seq 1 $INSTANCES); do
        port=$((BASE_PORT + i - 1))
        conv_id="multi_conv_${i}_$(date +%s)"
        CONV_IDS+=($conv_id)
        
        echo -e "  Tracking $conv_id via Instance $i (Port $port)"
        curl -s -X POST "http://localhost:$port/conversations/$conv_id/agent-message" \
          -H "Content-Type: application/json" \
          -d "{\"agent_id\":\"agent_$i\",\"message_id\":\"msg_$(date +%s)\"}" >/dev/null
        
        sleep 1
    done
    
    echo -e "\n${YELLOW}📊 Checking waiting conversations in Redis:${NC}"
    waiting_count=$(podman exec redis-timeout-poc redis-cli ZCARD waiting_conversations 2>/dev/null || echo "0")
    echo "  Total waiting conversations: $waiting_count"
    
    if [ "$waiting_count" != "0" ]; then
        echo -e "${YELLOW}  Details:${NC}"
        podman exec redis-timeout-poc redis-cli ZRANGE waiting_conversations 0 -1 WITHSCORES | \
        while read -r conv_id && read -r timestamp; do
            current_time=$(date +%s)000
            wait_time=$(( (current_time - timestamp) / 1000 ))
            echo "    $conv_id: waiting ${wait_time}s"
        done
    fi
    
    show_leader_status
    
    echo -e "\n${YELLOW}⏰ Waiting 8 seconds for timeout notifications...${NC}"
    echo "Only the leader should process these timeouts"
    
    for i in {1..8}; do
        echo -e "${BLUE}⏱️  Time: ${i}s${NC}"
        sleep 1
    done
    
    # Check notification states
    echo -e "\n${YELLOW}📊 Checking notification states:${NC}"
    states=$(podman exec redis-timeout-poc redis-cli HGETALL notification_states 2>/dev/null || echo "(empty)")
    if [ "$states" = "(empty)" ]; then
        echo "  (none yet)"
    else
        echo "$states" | while read -r conv_id && read -r level; do
            echo "  $conv_id: level $level"
        done
    fi
    
    wait_for_input
    
    # Test 6: Customer Messages Clear Timeouts
    echo -e "\n${BLUE}🎬 TEST 6: Customer Messages Clear Timeouts${NC}"
    echo "==========================================="
    echo ""
    echo "Testing customer responses clear timeouts when sent to any instance"
    
    # Send customer responses through different instances
    echo -e "${YELLOW}💬 Sending customer responses through different instances...${NC}"
    
    for i in $(seq 1 $INSTANCES); do
        if [ $i -le ${#CONV_IDS[@]} ]; then
            port=$((BASE_PORT + i - 1))
            conv_id=${CONV_IDS[$((i-1))]}
            
            echo -e "  Clearing $conv_id via Instance $i (Port $port)"
            curl -s -X POST "http://localhost:$port/conversations/$conv_id/customer-response" \
              -H "Content-Type: application/json" \
              -d "{\"customer_id\":\"customer_$i\",\"message_id\":\"response_$(date +%s)\"}" >/dev/null
            
            sleep 1
        fi
    done
    
    echo -e "\n${YELLOW}📊 Checking conversations after customer responses:${NC}"
    waiting_count=$(podman exec redis-timeout-poc redis-cli ZCARD waiting_conversations 2>/dev/null || echo "0")
    echo "  Remaining waiting conversations: $waiting_count"
    
    states_count=$(podman exec redis-timeout-poc redis-cli HLEN notification_states 2>/dev/null || echo "0")
    echo "  Remaining notification states: $states_count"
    
    if [ "$waiting_count" != "0" ]; then
        echo -e "${YELLOW}  Remaining conversations:${NC}"
        podman exec redis-timeout-poc redis-cli ZRANGE waiting_conversations 0 -1 WITHSCORES
    fi
    
    wait_for_input
    
    # Test 7: Timeout Restart Cycle
    echo -e "\n${BLUE}🎬 TEST 7: Timeout Restart After Customer Response${NC}"
    echo "==============================================="
    echo ""
    echo "Testing that timeouts restart when agent sends new message after customer response"
    
    # Pick a conversation that was cleared and restart it
    restart_conv="restart_test_$(date +%s)"
    echo -e "${YELLOW}📝 Starting fresh timeout cycle for: $restart_conv${NC}"
    
    # Agent sends message via one instance
    curl -s -X POST "http://localhost:$BASE_PORT/conversations/$restart_conv/agent-message" \
      -H "Content-Type: application/json" \
      -d "{\"agent_id\":\"agent_restart\",\"message_id\":\"msg_$(date +%s)\"}" >/dev/null
    
    echo "✅ Agent message sent"
    
    # Wait for first notification
    echo -e "\n${YELLOW}⏰ Waiting 6 seconds for first notification...${NC}"
    sleep 6
    
    level=$(podman exec redis-timeout-poc redis-cli HGET notification_states $restart_conv 2>/dev/null || echo "0")
    echo "  Notification level: $level"
    
    # Customer responds via different instance
    port=$((BASE_PORT + 1))
    echo -e "\n${YELLOW}💬 Customer responds via Instance 2 (Port $port)...${NC}"
    curl -s -X POST "http://localhost:$port/conversations/$restart_conv/customer-response" \
      -H "Content-Type: application/json" \
      -d "{\"customer_id\":\"customer_restart\",\"message_id\":\"response_$(date +%s)\"}" >/dev/null
    
    echo "✅ Customer response sent - timeout cleared"
    
    # Agent sends follow-up message via third instance
    port=$((BASE_PORT + 2))
    echo -e "\n${YELLOW}📝 Agent sends follow-up message via Instance 3 (Port $port)...${NC}"
    curl -s -X POST "http://localhost:$port/conversations/$restart_conv/agent-message" \
      -H "Content-Type: application/json" \
      -d "{\"agent_id\":\"agent_followup\",\"message_id\":\"msg_followup_$(date +%s)\"}" >/dev/null
    
    echo "✅ Follow-up message sent - fresh timeout cycle started"
    
    # Show final state
    echo -e "\n${YELLOW}📊 Final Redis state:${NC}"
    waiting_count=$(podman exec redis-timeout-poc redis-cli ZCARD waiting_conversations 2>/dev/null || echo "0")
    echo "  Waiting conversations: $waiting_count"
    
    if [ "$waiting_count" != "0" ]; then
        podman exec redis-timeout-poc redis-cli ZRANGE waiting_conversations 0 -1 WITHSCORES | \
        while read -r conv_id && read -r timestamp; do
            current_time=$(date +%s)000
            wait_time=$(( (current_time - timestamp) / 1000 ))
            echo "    $conv_id: waiting ${wait_time}s"
        done
    fi
    
    show_leader_status
    
    echo -e "\n${GREEN}🎉 Multi-instance leader election test completed!${NC}"
    echo ""
    echo -e "${BLUE}📋 What you should have observed:${NC}"
    echo "  1. ✅ Only ONE instance became leader at any time"
    echo "  2. ✅ Leader election happened quickly (~3-5 seconds)"
    echo "  3. ✅ When leader failed, re-election occurred automatically"
    echo "  4. ✅ Only the leader processed timeouts and sent notifications"
    echo "  5. ✅ All instances could receive API calls"
    echo "  6. ✅ Waiting conversations managed consistently across instances"
    echo "  7. ✅ Customer responses clear timeouts regardless of which instance receives them"
    echo "  8. ✅ Timeout cycles restart properly when agents send follow-up messages"
    echo ""
    echo -e "${YELLOW}📊 Check individual instance logs:${NC}"
    for i in $(seq 1 $INSTANCES); do
        echo "  tail -f phase1-instance-$i.log"
    done
    echo ""
    echo -e "${BLUE}🔍 Redis monitoring commands:${NC}"
    echo "  podman exec -it redis-timeout-poc redis-cli monitor"
    echo "  podman exec redis-timeout-poc redis-cli ZRANGE waiting_conversations 0 -1 WITHSCORES"
    echo "  podman exec redis-timeout-poc redis-cli HGETALL notification_states"
    echo "  podman exec redis-timeout-poc redis-cli GET timeout:leader" 