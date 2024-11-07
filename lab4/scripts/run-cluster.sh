#!/bin/bash

# Small utility to run N instances of the KV server based on a shardmap
# Requires the JSON utility `jq` to be installed: https://stedolan.github.io/jq/

# Usage: run-cluster.sh shardmap.json
#
# Runs a process using server.go for each node in the shardmap in the background.

set -e

shardmap_file=$1
nodes=$(jq -r '.nodes | keys[]' < $shardmap_file)
for node in $nodes ; do
	go run cmd/server/server.go --shardmap "$shardmap_file" --node "$node" &
	go run cmd/stress/tester.go --shardmap "single-node.json" --node $node --get-qps 200 --set-qps 50 --ttl 5 --num-keys 100 & # single-node load, high Get QPS, frequent TTL expirations
	go run cmd/stress/tester.go --shardmap "test-2-node-full.json" --node $node --get-qps 100 --set-qps 30 --ttl 3600 --num-keys 1000 & # redundancy, consistency, load distribution
	go run cmd/stress/tester.go --shardmap "test-3-node-100-shard.json" --node $node --get-qps 150 --set-qps 100 --ttl 2 --num-keys 500 & # concurrency, TTL churn, load balancing + dynamic env.
	go run cmd/stress/tester.go --shardmap "test-5-node.json" --node $node --get-qps 200 --set-qps 50 --ttl 1 --num-keys 2000 & # memory efficiency, high turnover, read/write consistency in large setup
done

wait