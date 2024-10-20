package main

import (
	user "cs426.yale.edu/lab1/user_service/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"log"
)

func main() {
	creds := insecure.NewCredentials()
	var conn *grpc.ClientConn
	conn, err := grpc.NewClient("localhost:8081", grpc.WithTransportCredentials(creds))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer func(conn *grpc.ClientConn) {
		err := conn.Close()
		if err != nil {
			log.Fatalf("Close unsuccesful %v", err)
		}
	}(conn)

	c := user.NewUserServiceClient(conn)
	defer conn.Close()

	// the client has ended here.

	userRequestInfo := &user.GetUserRequest{UserIds: []uint64{1, 2, 3}}

	userResponseInfo, err := c.GetUser(context.Background(), userRequestInfo)

	getSubscribersForRequestInfo := &user.GetUserRequest{UserIds: []uint64{1}}
	result, err := c.GetUser(context.Background(), getSubscribersForRequestInfo)
	if err != nil {
		log.Fatalf("Error when calling GetUser: %s", err)
	}

	log.Printf("Response from server: %s", result)
	var SubscribedTo []uint64 = result.Users[0].SubscribedTo
	userRequestInfo = &user.GetUserRequest{UserIds: SubscribedTo}
	userResponseInfo, err = c.GetUser(context.Background(), userRequestInfo)
	if err != nil {
		log.Fatalf("Error when calling GetUser: %s", err)
	}
	var LikedVideos []uint64 = make([]uint64, 0)
	for _, thisUser := range userResponseInfo.Users {
		LikedVideos = append(LikedVideos, thisUser.LikedVideos...)
	}

	if err != nil {
		log.Fatalf("Error when calling GetUser: %s", err)
	}

	log.Printf("Response from server: %s", userResponseInfo)

}
