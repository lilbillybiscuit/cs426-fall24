package raft

//
// Raft tests.
//
// we will use the original test_test.go to test your code for grading.
// so, while you can modify this code to help you debug, please
// test with the original before submitting.
//

import "testing"
import "time"

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
	println("Leader crashed: ", leader1)

	// new leader should be elected
	cfg.checkOneLeader()

	cfg.connect(leader1)
	println("Leader recovered: ", leader1)

	cfg.checkOneLeader()

	cfg.end()
}

//
//func TestNetworkPartition3A(t *testing.T) {
//	servers := 5
//	cfg := make_config(t, servers, false, false)
//	defer cfg.cleanup()
//
//	cfg.begin("Test (3A): network partition")
//
//	leader1 := cfg.checkOneLeader()
//
//	// Partition the network into two groups
//	cfg.disconnect((leader1 + 1) % servers)
//	cfg.disconnect((leader1 + 2) % servers)
//	println("Network partitioned")
//
//	// Check that the partitioned group cannot elect a leader
//	cfg.checkNoLeader()
//
//	// Restore the network
//	cfg.connect((leader1 + 1) % servers)
//	cfg.connect((leader1 + 2) % servers)
//	println("Network restored")
//
//	// Ensure a leader is elected after the partition is resolved
//	cfg.checkOneLeader()
//
//	cfg.end()
//}

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
