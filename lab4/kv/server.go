package kv

import (
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"sync"
	"sync/atomic"
	"time"

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
	item, ok := store.store[key]
	if ok {
		if item.TTLms > time.Now().UnixMilli() {
			defer store.rmu.RUnlock()
			return item, true
		} else {
			store.rmu.RUnlock()
			store.delete(key)
			return KVItem{}, false
		}
	}
	defer store.rmu.RUnlock()
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
		if item.TTLms < time.Now().UnixMilli() {
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
	shards := make([]KVShard, numShards)
	for i := 0; i < numShards; i++ {
		shards[i] = *MakeKVShard(numShards)
	}

	return &KVStore{
		shards: shards,
	}
}

func (shard *KVShard) getPartition(key string) *KVPartition {
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
	partition := hashfunc(key) % len(shard.partitions)
	return &shard.partitions[partition]
}

func (shard *KVShard) clearExpired() {
	//println("Clearing Shard", shard.isActive.Load())
	for shard.isActive.Load() {
		//println("Clearing Shard")
		for i := 0; i < len(shard.partitions); i++ {
			if !shard.isActive.Load() {
				break
			}

			if shard.partitions[i].rmu_clean.TryLock() {
				shard.partitions[i].rmu_clean.Unlock()
				go shard.partitions[i].clearExpired()
			}
			var delayTime time.Duration = time.Duration(2 / (len(shard.partitions) + 1))
			time.Sleep(time.Second * delayTime)
		}
	}
}

func (store *KVStore) clearExpiredAll() {
	//println("Clearing Store")
	for i := 0; i < len(store.shards); i++ {
		store.shards[i].isActive.Store(true)
		//println("Starting cleanup for shard ", i)
		go store.shards[i].clearExpired()
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
	storedShardMap *ShardMapState
}

func (server *KvServerImpl) handleShardMapUpdate() {
	// TODO: Part C
	server.storedShardMap = server.shardMap.GetState()
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
	hash := GetShardForKey(key, server.storedShardMap.NumShards)
	// check if shard should be handled by this server
	if !StringArrayContains(server.storedShardMap.ShardsToNodes[hash], server.nodeName) {
		return nil, status.Errorf(codes.NotFound, "Shard not handled by this server")
	}
	return &server.localStore.shards[hash-1], nil
}
func MakeKvServer(nodeName string, shardMap *ShardMap, clientPool ClientPool) *KvServerImpl {
	localStore := MakeKVStore(shardMap.GetState().NumShards)
	listener := shardMap.MakeListener()
	server := KvServerImpl{
		nodeName:   nodeName,
		shardMap:   shardMap,
		listener:   &listener,
		clientPool: clientPool,
		shutdown:   make(chan struct{}),
		localStore: localStore,
	}
	server.localStore.clearExpiredAll()
	go server.shardMapListenLoop()
	server.handleShardMapUpdate()
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

	shard, err := server.GetShardPartitionForKey(key)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "Shard not handled by this server")
	}
	partition := shard.getPartition(key)
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

	shard, err := server.GetShardPartitionForKey(key)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "Shard not handled by this server")
	}
	partition := shard.getPartition(key)
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
	shard, err := server.GetShardPartitionForKey(key)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "Shard not handled by this server")
	}
	partition := shard.getPartition(key)

	partition.delete(key)
	return &proto.DeleteResponse{}, nil
}

func (server *KvServerImpl) GetShardContents(
	ctx context.Context,
	request *proto.GetShardContentsRequest,
) (*proto.GetShardContentsResponse, error) {

	shardIndex := request.Shard
	if shardIndex < 0 || int(shardIndex) >= len(server.localStore.shards) {
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
