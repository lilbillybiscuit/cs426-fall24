package server_lib

import (
	"context"
	ranker2 "cs426.yale.edu/lab1/ranker"
	umc "cs426.yale.edu/lab1/user_service/mock_client"
	user_service "cs426.yale.edu/lab1/user_service/proto"
	pb "cs426.yale.edu/lab1/video_rec_service/proto"
	vmc "cs426.yale.edu/lab1/video_service/mock_client"
	video_service "cs426.yale.edu/lab1/video_service/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"log"
	"sort"
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
}

//const BATCH_SIZE = 50

type Stats struct {
	totalRequests      atomic.Uint64
	totalErrors        atomic.Uint64
	activeRequests     atomic.Int64
	userServiceErrors  atomic.Uint64
	videoServiceErrors atomic.Uint64

	mu           sync.Mutex
	latencySum   time.Duration
	latencyCount uint64
	//latencyEstimator *quantile.Stream // for 99th percentile
}

type VideoRecServiceServer struct {
	pb.UnimplementedVideoRecServiceServer
	options VideoRecServiceOptions
	// Add any data you want here
	UserServiceClient            user_service.UserServiceClient
	UserServiceClientConnection  *grpc.ClientConn
	VideoServiceClient           video_service.VideoServiceClient
	VideoServiceClientConnection *grpc.ClientConn

	stats Stats
}

func MakeVideoRecServiceServer(options VideoRecServiceOptions) (*VideoRecServiceServer, error) {
	creds := insecure.NewCredentials()

	// start a user client
	var userConn *grpc.ClientConn
	var err error
	userConn, err = grpc.NewClient(options.UserServiceAddr, grpc.WithTransportCredentials(creds))
	if err != nil {
		log.Printf("VideoRecService: failed to connect to UserService: %v", err)
		return nil, status.Errorf(codes.Unavailable, "VideoRecService: failed to connect to UserService: %v", err)
	}
	userServiceClient := user_service.NewUserServiceClient(userConn)

	// start a video client
	var videoConn *grpc.ClientConn
	videoConn, err = grpc.NewClient(options.VideoServiceAddr, grpc.WithTransportCredentials(creds))
	if err != nil {
		log.Printf("VideoRecService: failed to connect to VideoService: %v", err)
		return nil, status.Errorf(codes.Unavailable, "VideoRecService: failed to connect to VideoService: %v", err)
	}
	videoServiceClient := video_service.NewVideoServiceClient(videoConn)

	return &VideoRecServiceServer{
		options: options,
		// Add any data to initialize here
		UserServiceClient:            userServiceClient,
		UserServiceClientConnection:  userConn,
		VideoServiceClient:           videoServiceClient,
		VideoServiceClientConnection: videoConn,
	}, nil
}

func (server *VideoRecServiceServer) ContinuallyRefreshCache() {
	// Implement your own logic here
}

func (server *VideoRecServiceServer) GetStats(
	ctx context.Context,
	req *pb.GetStatsRequest,
) (*pb.GetStatsResponse, error) {
	// Implement your own logic here
	return &pb.GetStatsResponse{}, nil
}

func (server *VideoRecServiceServer) Close() error {
	if server.UserServiceClientConnection != nil {
		if err := server.UserServiceClientConnection.Close(); err != nil {
			return status.Errorf(codes.Unavailable, "VideoRecService: failed to close UserService connection: %v", err)
		}
	}

	if server.VideoServiceClientConnection != nil {
		if err := server.VideoServiceClientConnection.Close(); err != nil {
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
	return &VideoRecServiceServer{
		options: options,
		// ...
		UserServiceClient:  mockUserServiceClient,
		VideoServiceClient: mockVideoServiceClient,
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

func (server *VideoRecServiceServer) getUserSubscribedTo(
	ctx context.Context,
	req *pb.GetTopVideosRequest,
) (*user_service.UserInfo, error) {
	c := server.UserServiceClient
	result, err := c.GetUser(ctx, &user_service.GetUserRequest{UserIds: []uint64{req.UserId}})
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
	c := server.UserServiceClient
	var BATCH_SIZE = server.options.MaxBatchSize
	if userInfo == nil {
		return nil, status.Errorf(codes.Internal, "Internal error: userInfo is nil")
	}
	var subscribedToUsers = make([]*user_service.UserInfo, 0)
	for i := 0; i < len(userInfo.SubscribedTo); i += BATCH_SIZE {
		multiUserResponseInfo, err := c.GetUser(ctx, &user_service.GetUserRequest{
			UserIds: userInfo.SubscribedTo[i:min(i+BATCH_SIZE, len(userInfo.SubscribedTo))],
		})
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
	vc := server.VideoServiceClient
	var BATCH_SIZE = server.options.MaxBatchSize

	if likedVideos == nil {
		return nil, status.Errorf(codes.Internal, "Internal error: likedVideos is nil")
	}
	videoResponseList := make([]*video_service.VideoInfo, 0, len(likedVideos))

	for i := 0; i < len(likedVideos); i += BATCH_SIZE {
		videoResponseInfo, err := vc.GetVideo(ctx, &video_service.GetVideoRequest{
			VideoIds: likedVideos[i:min(i+BATCH_SIZE, len(likedVideos))],
		})
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
) (*pb.GetTopVideosResponse, error) {

	return server.getTopVideos(ctx, req)
}
