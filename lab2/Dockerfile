FROM golang:1.23.2-alpine AS builder

WORKDIR /app
COPY go.mod ./
COPY . .

# build the app
RUN go mod tidy
RUN go build -o /video_rec_service video_rec_service/server/server.go

FROM alpine:latest

WORKDIR /app

COPY --from=builder /video_rec_service /video_rec_service
CMD ["/video_rec_service"]
