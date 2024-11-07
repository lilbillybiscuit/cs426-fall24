package kvtest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Like previous labs, you must write some tests on your own.
// Add your test cases in this file and submit them as extra_test.go.
// You must add at least 5 test cases, though you can add as many as you like.
//
// You can use any type of test already used in this lab: server
// tests, client tests, or integration tests.
//
// You can also write unit tests of any utility functions you have in utils.go
//
// Tests are run from an external package, so you are testing the public API
// only. You can make methods public (e.g. utils) by making them Capitalized.

func TestServerGetShardContents(t *testing.T) {
	// Setup with 1 node and 1 shard assigned to it
	setup := MakeTestSetup(MakeBasicOneShard())
	defer setup.Shutdown()

	// Set a key-value with TTL
	err := setup.NodeSet("testNode", "key1", "value1", 10*time.Second)
	assert.Nil(t, err)

	// Get contents of the shard
	resp, err := setup.NodeGetShardContents("testNode", 1)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(resp.Values))
	assert.Equal(t, "key1", resp.Values[0].Key)
	assert.Equal(t, "value1", resp.Values[0].Value)
	assert.True(t, resp.Values[0].TtlMsRemaining > 0)
}

func TestServerGetShardContentsExcludeExpiredKeys(t *testing.T) {
	setup := MakeTestSetup(MakeBasicOneShard())
	defer setup.Shutdown()

	// Set a key with a short TTL, let it expire
	err := setup.NodeSet("testNode", "key1", "value1", 1*time.Millisecond)
	assert.Nil(t, err)
	time.Sleep(5 * time.Millisecond)

	// Get contents of the shard and check that expired keys are not included
	resp, err := setup.NodeGetShardContents("testNode", 1)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(resp.Values)) // No keys should be returned
}

func TestServerUpdateShardMapAssignment(t *testing.T) {
	setup := MakeTestSetup(MakeBasicOneShard())
	defer setup.Shutdown()

	// Add a shard to the node and confirm assignment
	setup.UpdateShardMapping(map[int][]string{1: {"n1"}})
	_, err := setup.NodeGetShardContents("n1", 1)
	assert.Nil(t, err)
}

func TestServerGetShardContentsLoadBalancing(t *testing.T) {
	setup := MakeTestSetup(MakeManyNodesWithManyShards(100, 100))
	defer setup.Shutdown()

	// Assign a shard to multiple nodes
	setup.UpdateShardMapping(map[int][]string{1: {"n1", "n2"}})

	// Set a key on one node
	err := setup.NodeSet("n1", "key1", "value1", 10*time.Second)
	assert.Nil(t, err)

	// Check contents on another node to see if it properly balances requests
	resp, err := setup.NodeGetShardContents("n2", 1)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(resp.Values))
	assert.Equal(t, "key1", resp.Values[0].Key)
	assert.Equal(t, "value1", resp.Values[0].Value)
}

func TestClientGetLoadBalancingAcrossNodes(t *testing.T) {
	setup := MakeTestSetup(MakeManyNodesWithManyShards(100, 100))
	defer setup.Shutdown()

	// Set a key and load-balance `Get` requests
	setup.UpdateShardMapping(map[int][]string{1: {"n1", "n2"}})
	setup.NodeSet("n1", "loadBalancedKey", "loadBalancedValue", 10*time.Second)

	for i := 0; i < 10; i++ {
		val, _, err := setup.ClientGet("loadBalancedKey")
		assert.Nil(t, err)
		assert.Equal(t, "loadBalancedValue", val)
	}
}

func TestServerConcurrentSetWithTtl(t *testing.T) {
	setup := MakeTestSetup(MakeManyNodesWithManyShards(100, 100))
	defer setup.Shutdown()

	// Concurrently set keys with different TTLs
	go setup.NodeSet("n1", "key1", "value1", 5*time.Second)
	go setup.NodeSet("n1", "key2", "value2", 10*time.Second)
	go setup.NodeSet("n1", "key3", "value3", 15*time.Second)

	time.Sleep(1 * time.Second)

	// Validate that each key has the correct TTL remaining
	resp, _ := setup.NodeGet("n1", "key1")
	assert.True(t, resp.TtlMsRemaining > 4000)
	resp, _ = setup.NodeGet("n1", "key2")
	assert.True(t, resp.TtlMsRemaining > 9000)
	resp, _ = setup.NodeGet("n1", "key3")
	assert.True(t, resp.TtlMsRemaining > 14000)
}

func TestServerDeleteOnUnassignedShard(t *testing.T) {
	setup := MakeTestSetup(MakeNoShardAssigned())
	defer setup.Shutdown()

	// Try to delete from an unassigned shard
	err := setup.NodeDelete("n1", "unassignedKey")
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "Shard not handled by this server")
}

func TestServerShardMigrationWithTtlConsistency(t *testing.T) {
	setup := MakeTestSetup(MakeBasicOneShard())
	defer setup.Shutdown()

	// Set a key with TTL, migrate shard, and check TTL consistency
	err := setup.NodeSet("n1", "key1", "value1", 10*time.Second)
	assert.Nil(t, err)
	setup.UpdateShardMapping(map[int][]string{1: {"n1", "n2"}})
	setup.UpdateShardMapping(map[int][]string{1: {"n2"}})

	resp, err := setup.NodeGet("n2", "key1")
	assert.Nil(t, err)
	assert.True(t, resp.TtlMsRemaining > 0)
	assert.Equal(t, "value1", resp.Value)
}

func TestClientGetFailover(t *testing.T) {
	setup := MakeTestSetup(MakeFourNodesWithFiveShards())
	defer setup.Shutdown()

	// Set a key on one node, simulate a failure, and ensure failover works
	setup.UpdateShardMapping(map[int][]string{1: {"n1", "n2"}})
	setup.NodeSet("n1", "failoverKey", "failoverValue", 10*time.Second)
	setup.SimulateNodeFailure("n1")

	val, _, err := setup.ClientGet("failoverKey")
	assert.Nil(t, err)
	assert.Equal(t, "failoverValue", val)
}

func TestClientSetAndImmediateGetConsistency(t *testing.T) {
	setup := MakeTestSetup(MakeBasicOneShard())
	defer setup.Shutdown()

	err := setup.NodeSet("n1", "consistencyKey", "consistentValue", 10*time.Second)
	assert.Nil(t, err)

	val, wasFound, err := setup.ClientGet("consistencyKey")
	assert.Nil(t, err)
	assert.True(t, wasFound)
	assert.Equal(t, "consistentValue", val)
}
