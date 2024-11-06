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

const NUM_KV_PARTITIONS = 16

type KVItem struct {
	Value string
	TTLms int64
}

type KVShardStore struct {
	rmu       sync.RWMutex
	rmu_clean sync.Mutex
	store     map[string]KVItem
}

type KVStore struct {
	partitions []KVShardStore // one shard store per shard. For now,
	isActive   atomic.Bool
}

func (store *KVShardStore) get(key string) (KVItem, bool) {
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

func (store *KVShardStore) set(key string, value string, ttlms int64) {
	store.rmu.Lock()
	defer store.rmu.Unlock()
	store.store[key] = KVItem{Value: value, TTLms: ttlms + time.Now().UnixMilli()}
}

func (store *KVShardStore) delete(key string) {
	store.rmu.Lock()
	defer store.rmu.Unlock()
	delete(store.store, key)
}

func (store *KVShardStore) clearExpired() {
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

func MakeKVStore(numShards int) *KVStore {
	partitions := make([]KVShardStore, numShards)
	for i := 0; i < numShards; i++ {
		partitions[i] = KVShardStore{store: make(map[string]KVItem)}
	}
	return &KVStore{
		partitions: partitions,
	}
}

func (store *KVStore) getPartition(key string) *KVShardStore {
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
	shard := hashfunc(key) % len(store.partitions)
	return &store.partitions[shard]
}

func (store *KVStore) clearExpiredAll() {
	for store.isActive.Load() {
		for i := 0; i < len(store.partitions); i++ {
			if !store.isActive.Load() {
				break
			}

			if store.partitions[i].rmu_clean.TryLock() {
				store.partitions[i].rmu_clean.Unlock()
				go store.partitions[i].clearExpired()
			}
			var delayTime time.Duration = time.Duration(2 / (len(store.partitions) + 1))
			time.Sleep(time.Second * delayTime)
		}
	}
}

func (store *KVStore) Shutdown() {
	store.isActive.Store(false)
}

type KvServerImpl struct {
	proto.UnimplementedKvServer
	nodeName string

	shardMap   *ShardMap
	listener   *ShardMapListener
	clientPool ClientPool
	shutdown   chan struct{}

	// Part A
	localStore *KVStore
}

func (server *KvServerImpl) handleShardMapUpdate() {
	// TODO: Part C
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

func MakeKvServer(nodeName string, shardMap *ShardMap, clientPool ClientPool) *KvServerImpl {
	localStore := MakeKVStore(NUM_KV_PARTITIONS)
	listener := shardMap.MakeListener()
	server := KvServerImpl{
		nodeName:   nodeName,
		shardMap:   shardMap,
		listener:   &listener,
		clientPool: clientPool,
		shutdown:   make(chan struct{}),
		localStore: localStore,
	}
	go server.localStore.clearExpiredAll()
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

	partition := server.localStore.getPartition(key)
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

	partition := server.localStore.getPartition(key)
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

	partition := server.localStore.getPartition(key)
	partition.delete(key)
	return &proto.DeleteResponse{}, nil
}

func (server *KvServerImpl) GetShardContents(
	ctx context.Context,
	request *proto.GetShardContentsRequest,
) (*proto.GetShardContentsResponse, error) {
	panic("TODO: Part C")
}
