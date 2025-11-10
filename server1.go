package main

import (
	pb "Mandatory-Activity-4/grpc"
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
)

type mutexServer struct {
	pb.UnimplementedMutexServiceServer

	mu sync.Mutex
	// Last received Lamport timestamp
	id         int
	timestamp  int32
	state      string
	queue      chan [2]int32 // [0] == T_i and [1] == p_i
	peerAddrs  []string
	peers      map[int]pb.MutexServiceClient
	replyCount int
}

func (s *mutexServer) connectToPeers() {
	for _, addr := range s.peerAddrs {
		go func(a string) {
			for {
				conn, err := grpc.Dial(a, grpc.WithInsecure())
				if err == nil {
					client := pb.NewMutexServiceClient(conn)
					s.mu.Lock()
					s.peers[len(s.peers)+1] = client
					s.mu.Unlock()
					fmt.Printf("Server %d connected to peer %s\n", s.id, a)
					return
				}
				time.Sleep(time.Second)
			}
		}(addr)
	}
}

func (s *mutexServer) simulate() {
	for {
		if rand.Intn(2) == 0 {
			s.enter()
		}
		time.Sleep(2 * time.Second)
	}
}

func (s *mutexServer) enter() {
	s.state = "WANTED"
	s.mu.Lock()
	defer s.mu.Unlock()
	s.replyCount = 0
	// Lamport timestamp update
	s.timestamp++

	msg := &pb.RequestMessage{
		Id:        int32(s.id), // your own server ID
		Timestamp: s.timestamp, // your current Lamport timestamp
	}

	for i := 0; i < 3; i++ {
		if i != s.id {
			ctx := context.Background()
			s.peers[i].Request(ctx, msg)
		}
	}
}

func (s *mutexServer) Request(ctx context.Context, req *pb.RequestMessage) (*pb.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Lamport timestamp update
	s.timestamp = max(s.timestamp, req.Timestamp) + 1

	if s.state == "HELD" || (s.state == "WANTED" && s.timestamp < req.Timestamp) {
		s.queue <- [2]int32{req.Timestamp, req.Id}
	} else {
		msg := &pb.ReplyMessage{
			Id:        int32(s.id), // your own server ID
			Timestamp: s.timestamp, // your current Lamport timestamp
		}

		s.peers[int(req.Id)].Reply(ctx, msg)
	}

	log.Printf("[Request] From ID=%d | T=%d | Updated T=%d", req.Id, req.Timestamp, s.timestamp)
	return &pb.Empty{}, nil
}

func (s *mutexServer) Reply(ctx context.Context, rep *pb.ReplyMessage) (*pb.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.replyCount++

	if s.replyCount == 2 {
		s.state = "HELD"
		fmt.Println(s.id, " has entered the critical section: ")
		time.Sleep(10 * time.Second)
		s.state = "RELEASED"
	}

	// Lamport timestamp update
	//s.timestamp = max(s.timestamp, rep.Timestamp) + 1

	log.Printf("[Reply] From ID=%d | T=%d | Updated T=%d", rep.Id, rep.Timestamp, s.timestamp)
	return &pb.Empty{}, nil
}

func max(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}

func main() {
	go spawnServer(0, ":5050", []string{":5051", ":5052"})
	go spawnServer(1, ":5051", []string{":5050", ":5052"})
	go spawnServer(2, ":5052", []string{":5050", ":5051"})

	select {} // keep main alive
}
func spawnServer(server_id int, port string, peerPorts []string) {
	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	s := &mutexServer{
		id:        server_id,
		timestamp: 1,
		state:     "RELEASED",
		queue:     make(chan [2]int32, 10),
		peerAddrs: peerPorts,
		peers:     make(map[int]pb.MutexServiceClient),
	}

	grpcServer := grpc.NewServer()
	pb.RegisterMutexServiceServer(grpcServer, s)

	go func() {
		fmt.Println("Server", server_id, "listening on", port)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve: %v", err)
		}
	}()

	// Connect to peers after startup
	go s.connectToPeers()

	go s.simulate()
}
