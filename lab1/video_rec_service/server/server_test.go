package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	fipb "cs426.yale.edu/lab1/failure_injection/proto"
	umc "cs426.yale.edu/lab1/user_service/mock_client"
	usl "cs426.yale.edu/lab1/user_service/server_lib"
	vmc "cs426.yale.edu/lab1/video_service/mock_client"
	vsl "cs426.yale.edu/lab1/video_service/server_lib"

	pb "cs426.yale.edu/lab1/video_rec_service/proto"
	sl "cs426.yale.edu/lab1/video_rec_service/server_lib"
)

func TestServerBasic(t *testing.T) {
	vrOptions := sl.VideoRecServiceOptions{
		MaxBatchSize:    50,
		DisableFallback: true,
		DisableRetry:    true,
	}
	// You can specify failure injection options here or later send
	// SetInjectionConfigRequests using these mock clients
	uClient :=
		umc.MakeMockUserServiceClient(*usl.DefaultUserServiceOptions())
	vClient :=
		vmc.MakeMockVideoServiceClient(*vsl.DefaultVideoServiceOptions())
	vrService := sl.MakeVideoRecServiceServerWithMocks(
		vrOptions,
		uClient,
		vClient,
	)

	var userId uint64 = 204054
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	out, err := vrService.GetTopVideos(
		ctx,
		&pb.GetTopVideosRequest{UserId: userId, Limit: 5},
	)
	assert.True(t, err == nil)

	videos := out.Videos
	assert.Equal(t, 5, len(videos))
	assert.EqualValues(t, 1012, videos[0].VideoId)
	assert.Equal(t, "Harry Boehm", videos[1].Author)
	assert.EqualValues(t, 1209, videos[2].VideoId)
	assert.Equal(t, "https://video-data.localhost/blob/1309", videos[3].Url)
	assert.Equal(t, "precious here", videos[4].Title)

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	vClient.SetInjectionConfig(ctx, &fipb.SetInjectionConfigRequest{
		Config: &fipb.InjectionConfig{
			// fail one in 1 request, i.e., always fail
			FailureRate: 1,
		},
	})

	// Since we disabled retry and fallback, we expect the VideoRecService to
	// throw an error since the VideoService is "down".
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = vrService.GetTopVideos(
		ctx,
		&pb.GetTopVideosRequest{UserId: userId, Limit: 5},
	)
	assert.False(t, err == nil)
}

func TestServerBatchingLargeBatch(t *testing.T) {
	vrOptions := sl.VideoRecServiceOptions{
		MaxBatchSize:    51,
		DisableFallback: true,
		DisableRetry:    true,
	}
	uClient := umc.MakeMockUserServiceClient(*usl.DefaultUserServiceOptions())
	vClient := vmc.MakeMockVideoServiceClient(*vsl.DefaultVideoServiceOptions())
	vrService := sl.MakeVideoRecServiceServerWithMocks(
		vrOptions,
		uClient,
		vClient,
	)

	var userId uint64 = 204054
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	out, err := vrService.GetTopVideos(
		ctx,
		&pb.GetTopVideosRequest{UserId: userId, Limit: 5},
	)

	// expect an error
	assert.True(t, err != nil)
	assert.True(t, out == nil)
	assert.Contains(t, err.Error(), "User")
	assert.Contains(t, err.Error(), "exceeded the max batch size")
}

func TestServerBatchingTimeoutBeforeFinish(t *testing.T) {
	vrOptions := sl.VideoRecServiceOptions{
		MaxBatchSize:    50,
		DisableFallback: true,
		DisableRetry:    true,
	}
	uClient := umc.MakeMockUserServiceClient(*usl.DefaultUserServiceOptions())
	vClient := vmc.MakeMockVideoServiceClient(*vsl.DefaultVideoServiceOptions())
	vrService := sl.MakeVideoRecServiceServerWithMocks(
		vrOptions,
		uClient,
		vClient,
	)

	var userId uint64 = 204054
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	vClient.SetInjectionConfig(ctx, &fipb.SetInjectionConfigRequest{
		Config: &fipb.InjectionConfig{
			SleepNs:              10000000, // 10ms
			FailureRate:          9,        // should use less than 10 requests, only 339 items
			ResponseOmissionRate: 0,
		},
	})

	out, err := vrService.GetTopVideos(
		ctx,
		&pb.GetTopVideosRequest{UserId: userId, Limit: 1000000},
	)

	// expect an error
	assert.True(t, err == nil)
	assert.True(t, out != nil)
	assert.True(t, len(out.Videos) == 339)
}

func TestServerStats(t *testing.T) {
	vrOptions := sl.VideoRecServiceOptions{
		MaxBatchSize:    50,
		DisableFallback: true,
		DisableRetry:    true,
	}
	uClient := umc.MakeMockUserServiceClient(*usl.DefaultUserServiceOptions())
	vClient := vmc.MakeMockVideoServiceClient(*vsl.DefaultVideoServiceOptions())
	vrService := sl.MakeVideoRecServiceServerWithMocks(
		vrOptions,
		uClient,
		vClient,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// send in parallel, should all succeed
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := vrService.GetTopVideos(
				ctx,
				&pb.GetTopVideosRequest{UserId: uint64(204000 + i), Limit: 5},
			)
			assert.True(t, err == nil)
		}(i)
	}

	wg.Wait()
	stats, err := vrService.GetStats(ctx, &pb.GetStatsRequest{})
	assert.True(t, err == nil)
	assert.True(t, stats != nil)
	assert.Equal(t, uint64(100), stats.GetTotalRequests())
	assert.Equal(t, uint64(0), stats.GetTotalErrors())

	// test errors
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	vClient.SetInjectionConfig(ctx, &fipb.SetInjectionConfigRequest{
		Config: &fipb.InjectionConfig{
			SleepNs:              100000000, // 10ms
			FailureRate:          1,         // should use less than 10 requests, only 339 items
			ResponseOmissionRate: 0,
		},
	})

	// send in parallel, should all succeed
	var wg2 sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg2.Add(1)
		go func(i int) {
			defer wg2.Done()
			_, err := vrService.GetTopVideos(
				ctx,
				&pb.GetTopVideosRequest{UserId: uint64(204000 + i), Limit: 5},
			)
			assert.True(t, err != nil)
		}(i)
	}

	wg2.Wait()
	stats, err = vrService.GetStats(ctx, &pb.GetStatsRequest{})
	assert.True(t, err == nil)
	assert.True(t, stats != nil)
	assert.Equal(t, uint64(200), stats.GetTotalRequests())
	assert.Equal(t, uint64(100), stats.GetTotalErrors())
	assert.Equal(t, uint64(100), stats.GetVideoServiceErrors())
	assert.Equal(t, uint64(100), stats.GetTotalErrors())
}

func TestErrorHandling(t *testing.T) {
	vrOptions := sl.VideoRecServiceOptions{
		MaxBatchSize:    50,
		DisableFallback: true,
		DisableRetry:    true,
	}
	uClient := umc.MakeMockUserServiceClient(*usl.DefaultUserServiceOptions())
	vClient := vmc.MakeMockVideoServiceClient(*vsl.DefaultVideoServiceOptions())
	vrService := sl.MakeVideoRecServiceServerWithMocks(
		vrOptions,
		uClient,
		vClient,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Inject failure in the UserService
	uClient.SetInjectionConfig(ctx, &fipb.SetInjectionConfigRequest{
		Config: &fipb.InjectionConfig{
			FailureRate: 1, // Always fail
		},
	})

	_, err := vrService.GetTopVideos(
		ctx,
		&pb.GetTopVideosRequest{UserId: 204054, Limit: 5},
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "UserService")

	// Reset failure injection for UserService
	uClient.SetInjectionConfig(ctx, &fipb.SetInjectionConfigRequest{
		Config: &fipb.InjectionConfig{
			FailureRate: 0, // No failure
		},
	})

	// Inject failure in the VideoService
	vClient.SetInjectionConfig(ctx, &fipb.SetInjectionConfigRequest{
		Config: &fipb.InjectionConfig{
			FailureRate: 1, // Always fail
		},
	})

	_, err = vrService.GetTopVideos(
		ctx,
		&pb.GetTopVideosRequest{UserId: 204054, Limit: 5},
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "VideoService")
}

func TestRetry(t *testing.T) {
	vrOptions := sl.VideoRecServiceOptions{
		MaxBatchSize:    10,
		DisableFallback: false,
		DisableRetry:    true,
	}
	uClient := umc.MakeMockUserServiceClient(*usl.DefaultUserServiceOptions())
	vClient := vmc.MakeMockVideoServiceClient(*vsl.DefaultVideoServiceOptions())
	vrService := sl.MakeVideoRecServiceServerWithMocks(
		vrOptions,
		uClient,
		vClient,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	vClient.SetInjectionConfig(ctx, &fipb.SetInjectionConfigRequest{
		Config: &fipb.InjectionConfig{
			SleepNs:              0, // 10ms
			FailureRate:          50,
			ResponseOmissionRate: 0,
		},
	})

	// make a request, should fail
	_, err := vrService.GetTopVideos(
		ctx,
		&pb.GetTopVideosRequest{UserId: 204054, Limit: 5},
	)
	assert.True(t, err != nil)

	// should be a difference in retrying vs no retrying
	vrOptions2 := sl.VideoRecServiceOptions{
		MaxBatchSize:    10,
		DisableFallback: false,
		DisableRetry:    false,
	}
	vrService2 := sl.MakeVideoRecServiceServerWithMocks(
		vrOptions2,
		uClient,
		vClient,
	)

	// make a request, should succeed
	output, err := vrService2.GetTopVideos(
		ctx,
		&pb.GetTopVideosRequest{UserId: 204054, Limit: 5},
	)
	assert.True(t, err == nil)
	assert.True(t, output != nil)

}

func TestRefreshCache(t *testing.T) {
	vrOptions := sl.VideoRecServiceOptions{
		MaxBatchSize:    51,
		DisableFallback: false,
		DisableRetry:    true,
	}
	uClient := umc.MakeMockUserServiceClient(*usl.DefaultUserServiceOptions())
	vClient := vmc.MakeMockVideoServiceClient(*vsl.DefaultVideoServiceOptions())
	vrService := sl.MakeVideoRecServiceServerWithMocks(
		vrOptions,
		uClient,
		vClient,
	)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	vClient.SetInjectionConfig(ctx, &fipb.SetInjectionConfigRequest{
		Config: &fipb.InjectionConfig{
			SleepNs:              0, // 10ms
			FailureRate:          1, // should use less than 10 requests, only 339 items
			ResponseOmissionRate: 0,
		},
	})

	// make a request, should fail
	_, err := vrService.GetTopVideos(
		ctx,
		&pb.GetTopVideosRequest{UserId: 204054, Limit: 5},
	)
	assert.True(t, err != nil)
	//assert.Contains(t, err.Error(), "Video")

	vClient.SetInjectionConfig(ctx, &fipb.SetInjectionConfigRequest{
		Config: &fipb.InjectionConfig{
			SleepNs:              0, // 10ms
			FailureRate:          0, // should use less than 10 requests, only 339 items
			ResponseOmissionRate: 0,
		},
	})
	vrService.ContinuallyRefreshCache()

	time.Sleep(1 * time.Second)
	// vClient always fails now
	vClient.SetInjectionConfig(ctx, &fipb.SetInjectionConfigRequest{
		Config: &fipb.InjectionConfig{
			SleepNs:              0, // 10ms
			FailureRate:          1, // should use less than 10 requests, only 339 items
			ResponseOmissionRate: 0,
		},
	})

	// make a request, should succeed but have stale data in cache
	res, err := vrService.GetTopVideos(
		ctx,
		&pb.GetTopVideosRequest{UserId: 204054, Limit: 5},
	)
	assert.True(t, err == nil)
	assert.True(t, res != nil)
	//assert.True(t, res.StaleResponse == true)

}
