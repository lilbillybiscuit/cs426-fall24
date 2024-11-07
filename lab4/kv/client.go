package kv

import (
	"context"
	"fmt"
	"sync"
	"time"

	"cs426.yale.edu/lab4/kv/proto"
	"github.com/sirupsen/logrus"
)

type Kv struct {
	shardMap   *ShardMap
	clientPool ClientPool

	// Add any client-side state you want here
	rrCounters map[int]int
	mu         sync.Mutex
}

func MakeKv(shardMap *ShardMap, clientPool ClientPool) *Kv {
	kv := &Kv{
		shardMap:   shardMap,
		clientPool: clientPool,
		rrCounters: make(map[int]int),
	}
	// Add any initialization logic
	return kv
}

// Part B2
func (kv *Kv) getBalancedNodes(nodes []string, shard int) []string {
	if len(nodes) == 1 {
		return nodes
	}
	kv.mu.Lock()
	defer kv.mu.Unlock()
	idx := kv.rrCounters[shard]
	kv.rrCounters[shard] = (idx + 1) % len(nodes)
	// Part B3
	return append(nodes[idx:], nodes[:idx]...)
}

func (kv *Kv) Get(ctx context.Context, key string) (string, bool, error) {
	// Trace-level logging -- you can remove or use to help debug in your tests
	// with `-log-level=trace`. See the logging section of the spec.
	logrus.WithFields(
		logrus.Fields{"key": key},
	).Trace("client sending Get() request")

	// Part B1
	shard := GetShardForKey(key, kv.shardMap.NumShards())

	nodes := kv.shardMap.NodesForShard(shard)
	if len(nodes) == 0 {
		return "", false, fmt.Errorf("no nodes for shard %d", shard)
	}

	// Part B3
	balancedNodes := kv.getBalancedNodes(nodes, shard)
	var lastErr error
	for _, nodeName := range balancedNodes {
		client, err := kv.clientPool.GetClient(nodeName)
		if err != nil {
			lastErr = err
			continue
		}
		req := &proto.GetRequest{Key: key}
		resp, err := client.Get(ctx, req)
		if err != nil {
			lastErr = err
			continue
		}
		return resp.Value, resp.WasFound, nil
	}
	return "", false, lastErr
}

func (kv *Kv) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	logrus.WithFields(
		logrus.Fields{"key": key},
	).Trace("client sending Set() request")

	// Part B4
	share := GetShardForKey(key, kv.shardMap.NumShards())
	nodes := kv.shardMap.NodesForShard(share)
	if len(nodes) == 0 {
		return fmt.Errorf("no nodes for shard %d", share)
	}

	req := &proto.SetRequest{Key: key, Value: value, TtlMs: int64(ttl / time.Millisecond)}

	var wg sync.WaitGroup
	var lastErr error

	// send cuncurrent requests to all nodes
	for _, nodeName := range nodes {
		wg.Add(1)
		go func(nodeName string) {
			defer wg.Done()
			client, err := kv.clientPool.GetClient(nodeName)
			if err != nil {
				lastErr = err
				return
			}
			_, err = client.Set(ctx, req)
			if err != nil {
				lastErr = err
			}
		}(nodeName)
	}
	wg.Wait()
	return lastErr
}

func (kv *Kv) Delete(ctx context.Context, key string) error {
	logrus.WithFields(
		logrus.Fields{"key": key},
	).Trace("client sending Delete() request")

	// Part B5
	shard := GetShardForKey(key, kv.shardMap.NumShards())
	nodes := kv.shardMap.NodesForShard(shard)
	if len(nodes) == 0 {
		return fmt.Errorf("no nodes for shard %d", shard)
	}

	req := &proto.DeleteRequest{Key: key}

	var wg sync.WaitGroup
	var lastErr error

	// send cuncurrent requests to all nodes
	for _, nodeName := range nodes {
		wg.Add(1)
		go func(nodeName string) {
			defer wg.Done()
			client, err := kv.clientPool.GetClient(nodeName)
			if err != nil {
				lastErr = err
				return
			}
			_, err = client.Delete(ctx, req)
			if err != nil {
				lastErr = err
			}
		}(nodeName)
	}
	wg.Wait()
	return lastErr
}
