package raft

import "testing"

//import "fmt"
import "time"

import "math/rand"

//import "sync"

//
// Raft tests.
//
// we will use the original test_test.go to test your code for grading.
// so, while you can modify this code to help you debug, please
// test with the original before submitting.
//

// The tester generously allows solutions to complete elections in one second
// (much more than the paper's range of timeouts).
// const RaftElectionTimeout = 1000 * time.Millisecond
func TestLeaderCrashAndRecover3A(t *testing.T) {
	servers := 5
	cfg := make_config(t, servers, false, false)
	defer cfg.cleanup()

	cfg.begin("Test (3A): leader crash and recover")

	leader1 := cfg.checkOneLeader()

	// simulate leader crash
	cfg.disconnect(leader1)
	println("Leader crashed: ", leader1, time.Now().UnixNano()/1e6)

	cfg.checkOneLeader()
	println("Recovering Leader", leader1)
	cfg.connect(leader1)
	println("Leader recovered: ", leader1)
	time.Sleep(300 * time.Millisecond)
	cfg.checkOneLeader()

	cfg.end()
}

func TestRapidLeaderChanges3A(t *testing.T) {
	servers := 5
	cfg := make_config(t, servers, false, false)
	defer cfg.cleanup()

	cfg.begin("Test (3A): rapid leader changes")

	for i := 0; i < 5; i++ {
		leader := cfg.checkOneLeader()
		cfg.disconnect(leader)
		println("Leader disconnected: ", leader)
		time.Sleep(500 * time.Millisecond)
		cfg.connect(leader)
		println("Leader reconnected: ", leader)
	}

	cfg.checkOneLeader()

	cfg.end()
}

func TestAllNodesCrashAndRecover3A(t *testing.T) {
	servers := 5
	cfg := make_config(t, servers, false, false)
	defer cfg.cleanup()

	cfg.begin("Test (3A): all nodes crash and recover")

	// disconnect all
	for i := 0; i < servers; i++ {
		cfg.disconnect(i)
		println("Node disconnected: ", i)
	}

	// reconnect all nodes
	for i := 0; i < servers; i++ {
		cfg.connect(i)
		println("Node reconnected: ", i)
	}

	// one leader
	cfg.checkOneLeader()

	cfg.end()
}

// Test rapid leader changes and log consistency
func TestRapidLeaderChange3B(t *testing.T) {
	servers := 5
	cfg := make_config(t, servers, false, false)
	defer cfg.cleanup()

	cfg.begin("Test: rapid leader changes")

	cfg.one(1, servers, true)

	// force rapid leader changes by repeatedly disconnecting the leader
	for i := 0; i < 5; i++ {
		leader := cfg.checkOneLeader()
		cfg.disconnect(leader)

		time.Sleep(2 * time.Second)
		cfg.checkOneLeader()

		cfg.one(10+i, servers-1, true)

		// reconnect old leader
		cfg.connect(leader)
		cfg.one(20+i, servers, true)
	}

	cfg.end()
}

// Test concurrent commands with network partitions
func TestConcurrentWithPartitions3B(t *testing.T) {
	servers := 5
	cfg := make_config(t, servers, false, false)
	defer cfg.cleanup()

	cfg.begin("Test: concurrent commands with partitions")

	leader1 := cfg.checkOneLeader()

	partition1 := []int{leader1, (leader1 + 1) % servers}
	partition2 := []int{(leader1 + 2) % servers, (leader1 + 3) % servers, (leader1 + 4) % servers}

	for _, server := range partition2 {
		cfg.disconnect(server)
	}

	// submit commands to partition1
	for i := 0; i < 10; i++ {
		cfg.rafts[leader1].Start(100 + i)
	}

	for _, server := range partition2 {
		cfg.connect(server)
	}
	for _, server := range partition1 {
		cfg.disconnect(server)
	}

	// wait for new leader in partition2
	time.Sleep(2 * RaftElectionTimeout)
	leader2 := cfg.checkOneLeader()

	for i := 0; i < 10; i++ {
		cfg.rafts[leader2].Start(200 + i)
	}

	// reconnect all
	for i := 0; i < servers; i++ {
		cfg.connect(i)
	}

	cfg.one(300, servers, true)

	cfg.end()
}

// Test log replication with large entries
func TestLargeEntries3B(t *testing.T) {
	servers := 3
	cfg := make_config(t, servers, false, false)
	defer cfg.cleanup()

	cfg.begin("Test: large log entries")

	// Create large entries
	var large [10]string
	for i := 0; i < 10; i++ {
		large[i] = randstring(1000) // 1KB entries
	}

	for i, entry := range large {
		index := cfg.one(entry, servers, true)
		if index != i+1 {
			t.Fatalf("got index %v but expected %v", index, i+1)
		}
	}

	// verify all servers have identical logs
	for i := 0; i < servers; i++ {
		for j := i + 1; j < servers; j++ {
			if len(cfg.rafts[i].log) != len(cfg.rafts[j].log) {
				t.Fatalf("servers %d and %d have different log lengths", i, j)
			}
		}
	}

	cfg.end()
}

// Test recovery after temporary network isolation
func TestTemporaryIsolation3B(t *testing.T) {
	servers := 5
	cfg := make_config(t, servers, false, false)
	defer cfg.cleanup()

	cfg.begin("Test: temporary isolation")

	cfg.one(1, servers, true)

	isolated := rand.Intn(servers)
	cfg.disconnect(isolated)

	for i := 0; i < 10; i++ {
		cfg.one(10+i, servers-1, true)
	}

	cfg.connect(isolated)

	cfg.one(100, servers, true)

	time.Sleep(RaftElectionTimeout)
	last_index := cfg.one(101, servers, true)
	if nd, _ := cfg.nCommitted(last_index); nd != servers {
		t.Fatalf("not all servers committed entry %v", last_index)
	}

	cfg.end()
}

// Test leadership transfer
func TestLeadershipTransfer3B(t *testing.T) {
	servers := 3
	cfg := make_config(t, servers, false, false)
	defer cfg.cleanup()

	cfg.begin("Test: leadership transfer")

	old_leader := cfg.checkOneLeader()

	cfg.one(1, servers, true)

	for i := 0; i < 5; i++ {
		cfg.disconnect(old_leader)
		time.Sleep(RaftElectionTimeout / 2)
		cfg.connect(old_leader)
		time.Sleep(RaftElectionTimeout / 2)
	}

	cfg.checkOneLeader()

	cfg.one(2, servers, true)

	for i := 0; i < servers; i++ {
		if len(cfg.rafts[i].log) != len(cfg.rafts[0].log) {
			t.Fatalf("logs have different lengths")
		}
	}

	cfg.end()
}

func TestConsistentStateAfterRestart3C(t *testing.T) {
	servers := 3
	cfg := make_config(t, servers, false, false)
	defer cfg.cleanup()

	cfg.begin("Test (3C): consistent state after restart")

	initialLeader := cfg.checkOneLeader()
	//commit some entries
	for i := 1; i <= 5; i++ {
		cfg.one(i, servers, true)
	}

	cfg.disconnect(initialLeader)
	cfg.start1(initialLeader, cfg.applier)
	cfg.connect(initialLeader)

	// reconnect the remaining servers
	for i := 0; i < servers; i++ {
		if i != initialLeader {
			cfg.connect(i)
		}
	}

	for i := 1; i <= 5; i++ {
		cfg.wait(i, servers, i)
	}

	cfg.one(10, servers, true)
}

func TestLeadershipTransfer3C(t *testing.T) {
	servers := 5
	cfg := make_config(t, servers, false, false)
	defer cfg.cleanup()

	cfg.begin("Test (3C): leadership transfer and log consistency")

	cfg.one(10, servers, true)

	leader := cfg.checkOneLeader()
	cfg.disconnect(leader)

	cfg.one(11, servers-1, true)

	// allow disconnected leader to rejoin and check the consistency
	cfg.connect(leader)
	cfg.one(12, servers, true)

	cfg.end()
}
