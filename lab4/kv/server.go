package kv

import (
	"context"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"cs426.yale.edu/lab4/kv/proto"
	"github.com/sirupsen/logrus"
)

const NUM_KV_PARTITIONS = 8

type KVItem struct {
	Value string
	TTLms int64
}

type KVPartition struct {
	rmu       sync.RWMutex
	rmu_clean sync.Mutex
	store     map[string]KVItem
}

type KVShard struct {
	partitions []KVPartition // one shard store per shard. For now,
	isActive   atomic.Bool
}

type KVStore struct {
	// Part A
	shards   []KVShard
	isActive atomic.Bool
}

func (store *KVPartition) get(key string) (KVItem, bool) {
	// ok is false in 2 cases: key is not in map, or key is in map but TTL is expired
	store.rmu.RLock()
	defer store.rmu.RUnlock()
	item, ok := store.store[key]
	if ok {
		if item.TTLms > time.Now().UnixMilli() {
			return item, true
		} else {
			delete(store.store, key)
			return KVItem{}, false
		}
	}
	return item, ok
}

func (store *KVPartition) set(key string, value string, ttlms int64) {
	store.rmu.Lock()
	defer store.rmu.Unlock()
	store.store[key] = KVItem{Value: value, TTLms: ttlms + time.Now().UnixMilli()}
}

func (store *KVPartition) delete(key string) {
	store.rmu.Lock()
	defer store.rmu.Unlock()
	delete(store.store, key)
}

func (store *KVPartition) clearExpired() {
	store.rmu_clean.Lock()
	defer store.rmu_clean.Unlock()

	store.rmu.Lock()
	defer store.rmu.Unlock()
	for key, item := range store.store {
		if item.TTLms <= time.Now().UnixMilli() {
			delete(store.store, key)
		}
	}
	//
	// Alternative: lock every time (can't do this because reading store.store is a read operation, can be race)
	//for key, item := range store.store {
	//	if item.TTLms < time.Now().UnixMilli() {
	//		store.delete(key)
	//	}
	//}
}

func MakeKVShard(numPartitions int) *KVShard {
	partitions := make([]KVPartition, numPartitions)
	for i := 0; i < numPartitions; i++ {
		partitions[i] = KVPartition{store: make(map[string]KVItem)}
	}
	return &KVShard{
		partitions: partitions,
	}
}

func MakeKVStore(numShards int) *KVStore {
	logrus.Debugf("Making KVStore with numShards", numShards, "and numPartitions", NUM_KV_PARTITIONS, "total partitions", numShards*NUM_KV_PARTITIONS)
	shards := make([]KVShard, numShards)
	for i := 0; i < numShards; i++ {
		shards[i] = *MakeKVShard(NUM_KV_PARTITIONS)
	}

	return &KVStore{
		shards: shards,
	}
}

func (shard *KVShard) getPartitionNum(key string) int {
	hashfunc := func(s string) int {
		sLen := len(s)
		if sLen < 8 {
			sLen = 8
		}

		var hashValue int
		for i := 0; i < 4 && i < len(s); i++ {
			hashValue ^= int(s[i])
		}
		for i := len(s) - 4; i < len(s) && i >= 0; i++ {
			hashValue ^= int(s[i])
		}

		return hashValue
	}
	return hashfunc(key) % len(shard.partitions)
}

func (shard *KVShard) getPartitionObj(key string) *KVPartition {
	partition := shard.getPartitionNum(key)
	return &shard.partitions[partition]
}

func (shard *KVShard) clearExpired(shouldUseSeparateGoRoutine bool) {
	//println("Clearing Shard", shard.isActive.Load())
	for shard.isActive.Load() {
		if shouldUseSeparateGoRoutine {
			// println("Clearing Shard", time.Now().UnixMilli())
			for i := 0; i < len(shard.partitions); i++ {
				if !shard.isActive.Load() {
					break
				}

				if shard.partitions[i].rmu_clean.TryLock() {
					shard.partitions[i].rmu_clean.Unlock()
					go shard.partitions[i].clearExpired()
				}
				delayTime := time.Duration(2000/len(shard.partitions)) * time.Millisecond
				time.Sleep(delayTime)
			}
		} else {
			for i := 0; i < len(shard.partitions); i++ {
				shard.partitions[i].clearExpired()
			}
			time.Sleep(2 * time.Second)
		}
	}
}

func (store *KVStore) clearExpiredAll() {
	//println("Clearing Store")
	for i := 0; i < len(store.shards); i++ {
		store.shards[i].isActive.Store(true)
		//println("Starting cleanup for shard ", i)
		// should use separate goroutine if num shards * partitions > 1000
		go store.shards[i].clearExpired(len(store.shards)*NUM_KV_PARTITIONS < 1000)
	}
}

func (shard *KVStore) Shutdown() {
	for i := 0; i < len(shard.shards); i++ {
		shard.shards[i].isActive.Store(false)
	}
}

type KvServerImpl struct {
	proto.UnimplementedKvServer
	nodeName string

	shardMap *ShardMap

	listener   *ShardMapListener
	clientPool ClientPool
	shutdown   chan struct{}

	// Part A
	localStore *KVStore

	// Part C
	availableShards map[int]struct{}
	mu              sync.RWMutex
	updatingMutex   sync.RWMutex
}

func (server *KvServerImpl) updateShardMap(shardId int, fromNode []string) {
	logrus.Debugf("Starting shard transfer for shard %d to node %s", shardId, server.nodeName)
	// TODO: consider updating the partitions one-by-one, or locking them only after the request is made
	shard := &server.localStore.shards[shardId]

	// create temporary buffer to store the data
	tempStore := make([]map[string]KVItem, len(shard.partitions))
	for i := range tempStore {
		tempStore[i] = make(map[string]KVItem)
	}

	startIndex := rand.Intn(len(fromNode))
	for i := 0; i < len(fromNode); i++ {
		fromNodeIndex := (startIndex + i) % len(fromNode)
		fromNode := fromNode[fromNodeIndex]
		if fromNode == server.nodeName {
			continue
		}
		client, err := server.clientPool.GetClient(fromNode)
		if err != nil {
			// logrus.Debugf("Failed to get client for node %s: %v", fromNode, err)
			continue
		}

		ctx := context.Background()
		resp, err := client.GetShardContents(ctx, &proto.GetShardContentsRequest{
			Shard: int32(shardId + 1),
		})
		if err != nil {
			// logrus.Debugf("Failed to get shard contents from %s: %v", fromNode, err)
			continue
		}

		// data successful, copy everything to tempStore
		for _, item := range resp.Values {
			partition := shard.getPartitionNum(item.Key)
			tempStore[partition][item.Key] = KVItem{
				Value: item.Value,
				TTLms: item.TtlMsRemaining + time.Now().UnixMilli(),
			}
		}
		break
	}

	if len(tempStore) == 0 {
		logrus.Errorf("Failed to copy shard %d from any node", shardId)
		return
	}
	for i := 0; i < len(shard.partitions); i++ {
		shard.partitions[i].rmu.Lock()
		shard.partitions[i].store = tempStore[i]
		shard.partitions[i].rmu.Unlock()
	}
	logrus.Debugf("Completed shard transfer for shard %d; tempStore has %d partitions", shardId, len(tempStore))
}

func ToShardMapSet(shards []int) map[int]struct{} {
	set := make(map[int]struct{})
	for _, shard := range shards {
		set[shard] = struct{}{}
	}
	return set
}

func (server *KvServerImpl) handleShardMapUpdate() {
	// TODO: Part C
	//server.updatingMutex.Lock()
	//defer server.updatingMutex.Unlock()
	logrus.Debugf("Entering handleShardMapUpdate for node %s", server.nodeName)
	defer logrus.Debugf("Exiting handleShardMapUpdate for node %s", server.nodeName)

	server.mu.Lock()
	prevAvailableShards := server.availableShards
	newAvailableShards := ToShardMapSet(server.shardMap.ShardsForNode(server.nodeName))
	server.mu.Unlock()

	//var prevAvailableShards map[int]struct{} = server.availableShards
	//server.availableShards = ToShardMapSet(server.shardMap.ShardsForNode(server.nodeName))

	removeShards := make([]int, 0)
	addShards := make([]int, 0)

	for shard := range prevAvailableShards {
		if _, ok := server.availableShards[shard]; !ok {
			removeShards = append(removeShards, shard)
		}
	}

	for shard := range newAvailableShards {
		if _, ok := prevAvailableShards[shard]; !ok {
			addShards = append(addShards, shard)
		}
	}

	logrus.Debugf("ShardMap update on node %s: %v -> %v", server.nodeName, removeShards, addShards)

	// at this point, we know what we need to add and remove
	var wg sync.WaitGroup
	for _, shard := range addShards {
		wg.Add(1)
		go func(s int) {
			defer wg.Done()
			server.updateShardMap(s-1, server.shardMap.NodesForShard(s))
		}(shard)
	}
	wg.Wait()
	// remove shards
	for _, shard := range removeShards {
		shardObj := &server.localStore.shards[shard-1]
		for i := 0; i < len(shardObj.partitions); i++ {
			shardObj.partitions[i].rmu.Lock()
			shardObj.partitions[i].store = make(map[string]KVItem)
			shardObj.partitions[i].rmu.Unlock()
		}
	}
	server.mu.Lock()
	server.availableShards = newAvailableShards
	server.mu.Unlock()
	logrus.Debugf("Node %s is now handling %d shards", server.nodeName, len(server.availableShards))
}
func (server *KvServerImpl) shardMapListenLoop() {
	listener := server.listener.UpdateChannel()
	for {
		select {
		case <-server.shutdown:
			return
		case <-listener:
			server.handleShardMapUpdate()
		}
	}
}

func (server *KvServerImpl) GetShardPartitionForKey(key string) (*KVShard, error) {
	server.mu.RLock()
	defer server.mu.RUnlock()
	hash := GetShardForKey(key, len(server.localStore.shards))
	if _, ok := server.availableShards[hash]; !ok {
		return nil, status.Errorf(codes.NotFound, "Shard not handled by this server")
	}
	return &server.localStore.shards[hash-1], nil
}

func MakeKvServer(nodeName string, shardMap *ShardMap, clientPool ClientPool) *KvServerImpl {
	localStore := MakeKVStore(shardMap.GetState().NumShards)
	listener := shardMap.MakeListener()
	server := KvServerImpl{
		nodeName:        nodeName,
		shardMap:        shardMap,
		listener:        &listener,
		clientPool:      clientPool,
		shutdown:        make(chan struct{}),
		localStore:      localStore,
		availableShards: make(map[int]struct{}),
	}
	server.handleShardMapUpdate()
	go server.shardMapListenLoop()

	server.localStore.clearExpiredAll()
	return &server
}

func (server *KvServerImpl) Shutdown() {
	server.shutdown <- struct{}{}
	server.localStore.Shutdown()
	server.listener.Close()
}

func (server *KvServerImpl) Get(
	ctx context.Context,
	request *proto.GetRequest,
) (*proto.GetResponse, error) {
	// Trace-level logging for node receiving this request (enable by running with -log-level=trace),
	// feel free to use Trace() or Debug() logging in your code to help debug tests later without
	// cluttering logs by default. See the logging section of the spec.
	logrus.WithFields(
		logrus.Fields{"node": server.nodeName, "key": request.Key},
	).Trace("node received Get() request")

	key := request.Key
	if len(key) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Key cannot be empty")
	}

	//server.updatingMutex.RLock()
	//defer server.updatingMutex.RUnlock()

	shard, err := server.GetShardPartitionForKey(key)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "Shard not handled by this server")
	}
	partition := shard.getPartitionObj(key)
	item, ok := partition.get(key)
	if !ok {
		return &proto.GetResponse{
			Value:    "",
			WasFound: false,
		}, nil
	}

	return &proto.GetResponse{
		Value:    item.Value,
		WasFound: true,
	}, nil
}

func (server *KvServerImpl) Set(
	ctx context.Context,
	request *proto.SetRequest,
) (*proto.SetResponse, error) {
	logrus.WithFields(
		logrus.Fields{"node": server.nodeName, "key": request.Key},
	).Trace("node received Set() request")

	key, value, tts_ms := request.Key, request.Value, request.TtlMs
	if len(key) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Key cannot be empty")
	}

	//server.updatingMutex.RLock()
	//defer server.updatingMutex.RUnlock()

	shard, err := server.GetShardPartitionForKey(key)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "Shard not handled by this server")
	}
	partition := shard.getPartitionObj(key)
	partition.set(key, value, tts_ms)
	return &proto.SetResponse{}, nil

}

func (server *KvServerImpl) Delete(
	ctx context.Context,
	request *proto.DeleteRequest,
) (*proto.DeleteResponse, error) {
	logrus.WithFields(
		logrus.Fields{"node": server.nodeName, "key": request.Key},
	).Trace("node received Delete() request")

	key := request.Key
	if len(key) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Key cannot be empty")
	}
	//server.updatingMutex.RLock()
	//defer server.updatingMutex.RUnlock()
	shard, err := server.GetShardPartitionForKey(key)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "Shard not handled by this server")
	}
	partition := shard.getPartitionObj(key)

	partition.delete(key)
	return &proto.DeleteResponse{}, nil
}

func (server *KvServerImpl) GetShardContents(
	ctx context.Context,
	request *proto.GetShardContentsRequest,
) (*proto.GetShardContentsResponse, error) {

	shardIndex := request.Shard - 1
	if shardIndex < 0 || int(shardIndex) >= len(server.localStore.shards) {
		logrus.Errorf("Invalid shard index %d", shardIndex)
		return nil, status.Errorf(codes.InvalidArgument, "Invalid shard index")
	}

	shard := &server.localStore.shards[shardIndex]

	curTime := time.Now().UnixMilli()
	shardContents := make([]*proto.GetShardValue, 0)
	for i := 0; i < len(shard.partitions); i++ {
		shard.partitions[i].rmu.RLock()
		for key, item := range shard.partitions[i].store {
			if item.TTLms < curTime {
				continue
			}
			shardContents = append(shardContents, &proto.GetShardValue{
				Key:            key,
				Value:          item.Value,
				TtlMsRemaining: item.TTLms - curTime,
			})
		}
		shard.partitions[i].rmu.RUnlock()
	}

	return &proto.GetShardContentsResponse{
		Values: shardContents,
	}, nil
}
