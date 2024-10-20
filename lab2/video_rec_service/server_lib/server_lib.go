package server_lib

import (
	"context"
	ranker2 "cs426.yale.edu/lab1/ranker"
	umc "cs426.yale.edu/lab1/user_service/mock_client"
	user_service "cs426.yale.edu/lab1/user_service/proto"
	pb "cs426.yale.edu/lab1/video_rec_service/proto"
	vmc "cs426.yale.edu/lab1/video_service/mock_client"
	video_service "cs426.yale.edu/lab1/video_service/proto"
	"github.com/influxdata/tdigest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"log"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type VideoRecServiceOptions struct {
	// Server address for the UserService"
	UserServiceAddr string
	// Server address for the VideoService
	VideoServiceAddr string
	// Maximum size of batches sent to UserService and VideoService
	MaxBatchSize int
	// If set, disable fallback to cache
	DisableFallback bool
	// If set, disable all retries
	DisableRetry bool
	// Size of the client pool
	ClientPoolSize int
}

//const BATCH_SIZE = 50

type Stats struct {
	totalRequests      atomic.Uint64
	totalErrors        atomic.Uint64
	activeRequests     atomic.Int64
	userServiceErrors  atomic.Uint64
	videoServiceErrors atomic.Uint64
	latencySum         atomic.Uint64 // in milliseconds
	latencyCount       atomic.Uint64
	staleRequests      atomic.Uint64

	latencyDigest *tdigest.TDigest
	digestMutex   sync.Mutex
}

func (s *Stats) updateLatencyDigest(latency float64) {
	s.digestMutex.Lock()
	defer s.digestMutex.Unlock()
	s.latencyDigest.Add(latency, 1)
}

type TrendingVideoCache struct {
	rwmu            sync.RWMutex
	expirationTimeS uint64
	written         bool
	data            *[]*video_service.VideoInfo
}

func (cache *TrendingVideoCache) updateData(expirationTime uint64, data *[]*video_service.VideoInfo) {
	cache.rwmu.Lock()
	defer cache.rwmu.Unlock()
	cache.data = data
	cache.written = true
	cache.expirationTimeS = expirationTime
	return
}

func (cache *TrendingVideoCache) readData() *[]*video_service.VideoInfo {
	cache.rwmu.RLock()
	defer cache.rwmu.RUnlock()
	return cache.data
}

func (cache *TrendingVideoCache) getExpirationTime() uint64 {
	cache.rwmu.RLock()
	defer cache.rwmu.RUnlock()
	return cache.expirationTimeS
}

func (cache *TrendingVideoCache) isWritten() bool {
	cache.rwmu.RLock()
	defer cache.rwmu.RUnlock()
	return cache.written
}

type VideoRecServiceServer struct {
	pb.UnimplementedVideoRecServiceServer
	options VideoRecServiceOptions
	// Add any data you want here
	//UserServiceClient            user_service.UserServiceClient
	//UserServiceClientConnection  *grpc.ClientConn
	//VideoServiceClient           video_service.VideoServiceClient
	//VideoServiceClientConnection *grpc.ClientConn

	UserServiceConnectionPool  []*grpc.ClientConn
	VideoServiceConnectionPool []*grpc.ClientConn

	UserServiceClientPool  []user_service.UserServiceClient
	VideoServiceClientPool []video_service.VideoServiceClient

	UserServiceClientIndex  atomic.Int32
	VideoServiceClientIndex atomic.Int32

	trendingVideo *TrendingVideoCache
	stats         Stats
}

func (server *VideoRecServiceServer) GetOneUserClient() user_service.UserServiceClient {
	maxClients := len(server.UserServiceClientPool)
	index := server.UserServiceClientIndex.Add(1)
	return server.UserServiceClientPool[int(index)%maxClients]
}

func (server *VideoRecServiceServer) GetOneVideoClient() video_service.VideoServiceClient {
	maxClients := len(server.VideoServiceClientPool)
	index := server.VideoServiceClientIndex.Add(1)
	return server.VideoServiceClientPool[int(index)%maxClients]
}

func RetryOperationWithResult[T any](operation func(args ...interface{}) (T, error), args []interface{}, maxRetries int) (T, error) {
	var result T
	var err error

	for i := 0; i < maxRetries; i++ {
		result, err := operation(args...)
		if err == nil {
			return result, nil
		}
	}

	return result, err
}

func MakeVideoRecServiceServer(options VideoRecServiceOptions) (*VideoRecServiceServer, error) {
	creds := insecure.NewCredentials()

	var userServiceClientPool []user_service.UserServiceClient
	var videoServiceClientPool []video_service.VideoServiceClient
	var userServiceConnectionPool []*grpc.ClientConn
	var videoServiceConnectionPool []*grpc.ClientConn

	// start a user client
	var userConn *grpc.ClientConn
	var err error

	for i := 0; i < options.ClientPoolSize; i++ {
		for j := 0; j < 5; j++ {
			userConn, err = grpc.NewClient(options.UserServiceAddr, grpc.WithTransportCredentials(creds))
			if err != nil {
				log.Printf("VideoRecService: failed to connect to UserService on first attempt: %v", err)
				time.Sleep(1 * time.Second)
				continue
			}
			break
		}

		if err != nil {
			log.Printf("VideoRecService: failed to connect to UserService on second attempt: %v", err)
			return nil, status.Errorf(codes.Unavailable, "VideoRecService: failed to connect to UserService: %v", err)
		}

		userServiceConnectionPool = append(userServiceConnectionPool, userConn)
		userServiceClientPool = append(userServiceClientPool, user_service.NewUserServiceClient(userConn))
	}

	var videoConn *grpc.ClientConn
	for i := 0; i < options.ClientPoolSize; i++ {
		for j := 0; j < 5; j++ {
			videoConn, err = grpc.NewClient(options.VideoServiceAddr, grpc.WithTransportCredentials(creds))
			if err != nil {
				log.Printf("VideoRecService: failed to connect to VideoService on first attempt: %v", err)
				time.Sleep(1 * time.Second)
				continue
			}
			break
		}

		if err != nil {
			log.Printf("VideoRecService: failed to connect to VideoService on second attempt: %v", err)
			return nil, status.Errorf(codes.Unavailable, "VideoRecService: failed to connect to VideoService: %v", err)
		}
		userServiceConnectionPool = append(userServiceConnectionPool, videoConn)
		videoServiceClientPool = append(videoServiceClientPool, video_service.NewVideoServiceClient(videoConn))
	}

	return &VideoRecServiceServer{
		options: options,
		// Add any data to initialize here
		//UserServiceClient:            userServiceClient,
		//UserServiceClientConnection:  userConn,
		//VideoServiceClient:           videoServiceClient,
		//VideoServiceClientConnection: videoConn,
		UserServiceConnectionPool:  userServiceConnectionPool,
		VideoServiceConnectionPool: videoServiceConnectionPool,
		UserServiceClientPool:      userServiceClientPool,
		VideoServiceClientPool:     videoServiceClientPool,
		stats: Stats{
			latencyDigest: tdigest.NewWithCompression(1000),
		},
		trendingVideo: &TrendingVideoCache{},
	}, nil
}
func (server *VideoRecServiceServer) ContinuallyRefreshCache() {
	ctx := context.Background()
	go func() {
		for {
			cacheObj := server.trendingVideo
			curTime, expirationTime := uint64(time.Now().Unix()), cacheObj.getExpirationTime()
			if curTime < expirationTime {
				time.Sleep(3 * time.Second) // Refresh every 3 seconds
				continue
			}
			//c := server.VideoServiceClient
			c := server.GetOneVideoClient()
			trendingVideoResponse, err := c.GetTrendingVideos(ctx, &video_service.GetTrendingVideosRequest{})
			if err != nil {
				log.Printf("Error when calling GetTrendingVideos: %s, waiting 10 seconds", err)
				time.Sleep(10 * time.Second)
				continue
			}

			trendingVideoList, err := server.getVideoInfoFromVideoIDBatch(ctx, trendingVideoResponse.Videos)
			if err != nil {
				log.Printf("Error when calling GetVideo to retrieve VideoInfo: %s, waiting 10 seconds", err)
				time.Sleep(10 * time.Second)
				continue
			}

			server.trendingVideo.updateData(trendingVideoResponse.ExpirationTimeS, &trendingVideoList)
		}
	}()
}

func (server *VideoRecServiceServer) GetStats(
	ctx context.Context,
	req *pb.GetStatsRequest,
) (*pb.GetStatsResponse, error) {
	totalRequests := server.stats.totalRequests.Load()
	latencySum := server.stats.latencySum.Load()
	latencyCount := server.stats.latencyCount.Load()

	var avgLatency float64
	if latencyCount > 0 {
		avgLatency = float64(latencySum) / float64(latencyCount)
	}

	server.stats.digestMutex.Lock()
	percentile99 := server.stats.latencyDigest.Quantile(0.99)
	server.stats.digestMutex.Unlock()

	if math.IsNaN(percentile99) {
		percentile99 = 0
	}

	return &pb.GetStatsResponse{
		TotalRequests:      totalRequests,
		TotalErrors:        server.stats.totalErrors.Load(),
		ActiveRequests:     uint64(server.stats.activeRequests.Load()),
		UserServiceErrors:  server.stats.userServiceErrors.Load(),
		VideoServiceErrors: server.stats.videoServiceErrors.Load(),
		AverageLatencyMs:   float32(avgLatency),
		P99LatencyMs:       float32(percentile99),
		StaleResponses:     server.stats.staleRequests.Load(),
	}, nil
}
func (server *VideoRecServiceServer) Close() error {
	for _, conn := range server.UserServiceConnectionPool {
		if err := conn.Close(); err != nil {
			return status.Errorf(codes.Unavailable, "VideoRecService: failed to close UserService connection: %v", err)
		}
	}

	for _, conn := range server.VideoServiceConnectionPool {
		if err := conn.Close(); err != nil {
			return status.Errorf(codes.Unavailable, "VideoRecService: failed to close VideoService connection: %v", err)
		}
	}
	return nil
}
func MakeVideoRecServiceServerWithMocks(
	options VideoRecServiceOptions,
	mockUserServiceClient *umc.MockUserServiceClient,
	mockVideoServiceClient *vmc.MockVideoServiceClient,
) *VideoRecServiceServer {
	// Implement your own logic here
	if mockUserServiceClient == nil || mockVideoServiceClient == nil {
		log.Printf("MakeVideoRecServiceServerWithMocks: mockUserServiceClient or mockVideoServiceClient is nil")
		return nil
	}
	var userServiceClientPool []user_service.UserServiceClient
	var videoServiceClientPool []video_service.VideoServiceClient

	for i := 0; i < options.ClientPoolSize; i++ {
		userServiceClientPool = append(userServiceClientPool, mockUserServiceClient)
		videoServiceClientPool = append(videoServiceClientPool, mockVideoServiceClient)
	}

	return &VideoRecServiceServer{
		options:                options,
		UserServiceClientPool:  userServiceClientPool,
		VideoServiceClientPool: videoServiceClientPool,
		stats: Stats{
			latencyDigest: tdigest.NewWithCompression(1000),
		},
		trendingVideo: &TrendingVideoCache{},
	}
}

func deduplicate(arr []uint64) []uint64 {
	if len(arr) == 0 {
		return arr
	}

	sort.Slice(arr, func(i, j int) bool { return arr[i] < arr[j] })

	j := 0
	for i := 1; i < len(arr); i++ {
		if arr[j] != arr[i] {
			j++
			arr[j] = arr[i]
		}
	}
	return arr[:j+1]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (server *VideoRecServiceServer) getUserWithRetry(
	ctx context.Context,
	req *user_service.GetUserRequest,
	maxRetries int,
) (*user_service.GetUserResponse, error) {
	//c := server.UserServiceClient
	c := server.GetOneUserClient()
	var result *user_service.GetUserResponse
	var err error
	if server.options.DisableRetry {
		return c.GetUser(ctx, req)
	}
	for i := 0; i < maxRetries; i++ {
		result, err = c.GetUser(ctx, req)
		if err == nil {
			return result, nil
		}
		log.Printf("Error when calling GetUser: %s, retrying. (Attempt %d of %d)", err, i+1, maxRetries)
	}
	return nil, err

}

func (server *VideoRecServiceServer) getVideoWithRetry(
	ctx context.Context,
	req *video_service.GetVideoRequest,
	maxRetries int,
) (*video_service.GetVideoResponse, error) {
	//vc := server.VideoServiceClient
	vc := server.GetOneVideoClient()
	var result *video_service.GetVideoResponse
	var err error
	if server.options.DisableRetry {
		return vc.GetVideo(ctx, req)
	}
	for i := 0; i < maxRetries; i++ {
		result, err = vc.GetVideo(ctx, req)
		if err == nil {
			return result, nil
		}
		log.Printf("Error when calling GetVideo: %s, retrying. (Attempt %d of %d)", err, i+1, maxRetries)
	}
	return nil, err

}

func (server *VideoRecServiceServer) getUserSubscribedTo(
	ctx context.Context,
	req *pb.GetTopVideosRequest,
) (*user_service.UserInfo, error) {
	//c := server.UserServiceClient
	//result, err := c.GetUser(ctx, &user_service.GetUserRequest{UserIds: []uint64{req.UserId}})
	result, err := server.getUserWithRetry(ctx, &user_service.GetUserRequest{
		UserIds: []uint64{req.UserId},
	}, 2)
	if err != nil {
		log.Printf("Error when calling GetUser: %s", err)
		return nil, status.Errorf(codes.Unavailable, "Error when calling GetUser: %s", err)
	}
	if len(result.Users) == 0 {
		return nil, status.Errorf(codes.NotFound, "User not found")
	}
	return result.Users[0], nil
}

func (server *VideoRecServiceServer) getAllLikedVideosFromUser(
	ctx context.Context,
	//c user_service.UserServiceClient,
	userInfo *user_service.UserInfo,
) ([]uint64, error) {
	//c := server.UserServiceClient
	var BATCH_SIZE = server.options.MaxBatchSize
	if userInfo == nil {
		return nil, status.Errorf(codes.Internal, "Internal error: userInfo is nil")
	}
	var subscribedToUsers = make([]*user_service.UserInfo, 0)
	for i := 0; i < len(userInfo.SubscribedTo); i += BATCH_SIZE {
		//multiUserResponseInfo, err := c.GetUser(ctx, &user_service.GetUserRequest{
		//	UserIds: userInfo.SubscribedTo[i:min(i+BATCH_SIZE, len(userInfo.SubscribedTo))],
		//})
		multiUserResponseInfo, err := server.getUserWithRetry(ctx, &user_service.GetUserRequest{
			UserIds: userInfo.SubscribedTo[i:min(i+BATCH_SIZE, len(userInfo.SubscribedTo))],
		}, 2)
		if err != nil {
			log.Printf("Error when calling GetUser: %s", err)
			return nil, status.Errorf(codes.Unavailable, "Error when calling GetUser: %s", err)
		}
		subscribedToUsers = append(subscribedToUsers, multiUserResponseInfo.Users...)
	}
	var likedVideos []uint64 = make([]uint64, 0, len(subscribedToUsers))
	for _, thisUser := range subscribedToUsers {
		if thisUser != nil && thisUser.LikedVideos != nil {
			likedVideos = append(likedVideos, thisUser.LikedVideos...)
		}
	}

	likedVideos = deduplicate(likedVideos)
	return likedVideos, nil
}

func (server *VideoRecServiceServer) getVideoInfoFromVideoIDBatch(
	ctx context.Context,
	//vc video_service.VideoServiceClient,
	likedVideos []uint64,
) ([]*video_service.VideoInfo, error) {
	//vc := server.VideoServiceClient
	var BATCH_SIZE = server.options.MaxBatchSize

	if likedVideos == nil {
		return nil, status.Errorf(codes.Internal, "Internal error: likedVideos is nil")
	}
	videoResponseList := make([]*video_service.VideoInfo, 0, len(likedVideos))

	for i := 0; i < len(likedVideos); i += BATCH_SIZE {
		//videoResponseInfo, err := vc.GetVideo(ctx, &video_service.GetVideoRequest{
		//	VideoIds: likedVideos[i:min(i+BATCH_SIZE, len(likedVideos))],
		//})
		videoResponseInfo, err := server.getVideoWithRetry(ctx, &video_service.GetVideoRequest{
			VideoIds: likedVideos[i:min(i+BATCH_SIZE, len(likedVideos))],
		}, 2)
		if err != nil {
			log.Printf("Error when calling GetVideo: %s", err)
			return nil, status.Errorf(codes.Unavailable, "Error when calling GetVideo: %s", err)
		}
		videoResponseList = append(videoResponseList, videoResponseInfo.Videos...)
	}
	return videoResponseList, nil
}

func (server *VideoRecServiceServer) getTopVideos(
	ctx context.Context,
	req *pb.GetTopVideosRequest,
) (*pb.GetTopVideosResponse, error) {
	var Limit int32 = req.Limit
	if Limit <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Limit must be positive")
	}

	origUser, err := server.getUserSubscribedTo(ctx, req)
	if err != nil {
		return nil, err
	}
	// start requesting for user info via batching
	likedVideos, err := server.getAllLikedVideosFromUser(ctx, origUser)
	if err != nil {
		return nil, err
	}

	// start requesting for videoInfo via batching
	videoResponseList, err := server.getVideoInfoFromVideoIDBatch(ctx, likedVideos)
	if err != nil {
		return nil, err
	}
	// end requesting for videoInfo via batching
	// use videoResponseList from now on

	var ranker = ranker2.BcryptRanker{}
	type UserVideoPair struct {
		rankingValue uint64
		//userCoefficient *user_service.UserCoefficients
		videoInfo *video_service.VideoInfo
	}

	var userVideoPairs []UserVideoPair = make([]UserVideoPair, 0, len(videoResponseList))
	for _, videoInfo := range videoResponseList {
		var bcryptRank uint64
		if videoInfo != nil {
			bcryptRank = ranker.Rank(origUser.UserCoefficients, videoInfo.VideoCoefficients)
			userVideoPairs = append(userVideoPairs, UserVideoPair{bcryptRank, videoInfo})
		}
	}

	// sort the video info based on the ranking value, in descending order
	sort.Slice(userVideoPairs, func(i, j int) bool {
		return userVideoPairs[i].rankingValue > userVideoPairs[j].rankingValue
	})

	var topVideos []*video_service.VideoInfo = make([]*video_service.VideoInfo, 0)
	for i, userVideoPair := range userVideoPairs {
		if int32(i) >= Limit {
			break
		}
		topVideos = append(topVideos, userVideoPair.videoInfo)
	}

	return &pb.GetTopVideosResponse{Videos: topVideos}, nil
}

func (server *VideoRecServiceServer) GetTopVideos(
	ctx context.Context,
	req *pb.GetTopVideosRequest,
) (response *pb.GetTopVideosResponse, err error) {
	server.stats.activeRequests.Add(1)
	server.stats.totalRequests.Add(1)
	start := time.Now()
	var responded bool = false
	getFallbackResponse := func() (*pb.GetTopVideosResponse, error) {
		hasData := server.trendingVideo.isWritten()
		if !hasData {
			return nil, status.Errorf(codes.Unavailable, "No data available")
		}
		defer func() {
			server.stats.staleRequests.Add(1)
			responded = true
		}()
		data := server.trendingVideo.readData()
		return &pb.GetTopVideosResponse{Videos: *data, StaleResponse: true}, nil
	}

	defer func() {
		server.stats.activeRequests.Add(-1)
		latency := time.Since(start)
		latencyMs := float64(latency.Milliseconds())
		server.stats.latencySum.Add(uint64(latencyMs))
		server.stats.latencyCount.Add(1)
		server.stats.updateLatencyDigest(latencyMs)

		if r := recover(); r != nil {
			server.stats.totalErrors.Add(1)
			err = status.Errorf(codes.Internal, "Internal server error")
			if !server.options.DisableFallback {
				response, err = getFallbackResponse()
			}

		}

		if err != nil {
			//println("ERROR IN GetTopVideos", response, server.options.DisableFallback)
			server.stats.totalErrors.Add(1)
			if strings.Contains(err.Error(), "UserService") {
				server.stats.userServiceErrors.Add(1)
			} else if strings.Contains(err.Error(), "VideoService") {
				server.stats.videoServiceErrors.Add(1)
			}
			if !responded && !server.options.DisableFallback {
				response, err = getFallbackResponse()
			}
		}
	}()

	response, err = server.getTopVideos(ctx, req)
	return response, err
}
