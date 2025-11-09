package main

import (
	pb "Mandatory-Activity-4/grpc"
	"context"
	"fmt"
	"log"
	"net"
	"strconv"
	"sync"

	"google.golang.org/grpc"
)

type mutexServer struct {
	pb.UnimplementedMutexServiceServer

	mu sync.Mutex
	// Last received Lamport timestamp
	id        int
	timestamp int
	state     string
	queue     chan [2]int // [0] == T_i and [1] == p_i
}

func (s *mutexServer) Request(ctx context.Context, req *pb.RequestMessage) (*pb.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Lamport timestamp update
	s.timestamp = max(s.timestamp, int(req.Timestamp)) + 1

	log.Printf("[Request] From ID=%d | T=%d | Updated T=%d", req.Id, req.Timestamp, s.timestamp)
	return &pb.Empty{}, nil
}

func (s *mutexServer) Reply(ctx context.Context, rep *pb.ReplyMessage) (*pb.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Lamport timestamp update
	s.timestamp = max(s.timestamp, int(rep.Timestamp)) + 1

	log.Printf("[Reply] From ID=%d | T=%d | Updated T=%d", rep.Id, rep.Timestamp, s.timestamp)
	return &pb.Empty{}, nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func main() {
	go spawnServer(1, ":5050")
	go spawnServer(2, ":5051")
	go spawnServer(3, ":5052")

	select {}
}
func spawnServer(server_id int, port string) {
	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()

	ms := &mutexServer{
		id:        server_id,
		timestamp: 1,
		state:     "RELEASED",
	}

	pb.RegisterMutexServiceServer(grpcServer, ms)

	fmt.Println("Mutex gRPC server running on port" + port)
	fmt.Println("S"+strconv.Itoa(ms.id)+"-> Lamport:", ms.timestamp, "State:", ms.state)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
	
}
