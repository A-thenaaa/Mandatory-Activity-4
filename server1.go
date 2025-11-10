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
					log.Printf("[Server %d | STATE=%s | T=%d] Connected to peer %s", s.id, s.state, s.timestamp, a)
					return
				}
				time.Sleep(time.Second)
			}
		}(addr)
	}
}

func (s *mutexServer) simulate() {
	for {
		log.Printf("[Server %d | STATE=%s | T=%d] Simulation tick", s.id, s.state, s.timestamp)
		if rand.Intn(2) == 0 {
			s.enter()
		}
		time.Sleep(2 * time.Second)
	}
}

func (s *mutexServer) enter() {
	s.state = "WANTED"
	log.Printf("[Server %d | STATE=%s | T=%d] Wants to enter critical section", s.id, s.state, s.timestamp)
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
			log.Printf("[Server %d | STATE=%s | T=%d] Sent request to %d", s.id, s.state, s.timestamp, i)
		}
	}
}

func (s *mutexServer) Request(ctx context.Context, req *pb.RequestMessage) (*pb.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	log.Printf("[Server %d | STATE=%s | T=%d] Received REQUEST from %d (T=%d)", s.id, s.state, s.timestamp, req.Id, req.Timestamp)

	// Lamport timestamp update
	s.timestamp = max(s.timestamp, req.Timestamp) + 1

	if s.state == "HELD" || (s.state == "WANTED" && s.timestamp < req.Timestamp) {
		s.queue <- [2]int32{req.Timestamp, req.Id}
		log.Printf("[Server %d | STATE=%s | T=%d] Queued request from %d", s.id, s.state, s.timestamp, req.Id)
	} else {
		msg := &pb.ReplyMessage{
			Id:        int32(s.id), // your own server ID
			Timestamp: s.timestamp, // your current Lamport timestamp
		}

		s.peers[int(req.Id)].Reply(ctx, msg)
		log.Printf("[Server %d | STATE=%s | T=%d] Sent REPLY to %d", s.id, s.state, s.timestamp, req.Id)
	}

	log.Printf("[Request] From ID=%d | T=%d | Updated T=%d", req.Id, req.Timestamp, s.timestamp)
	return &pb.Empty{}, nil
}

func (s *mutexServer) Reply(ctx context.Context, rep *pb.ReplyMessage) (*pb.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.replyCount++
	log.Printf("[Server %d | STATE=%s | T=%d] Received REPLY from %d", s.id, s.state, s.timestamp, rep.Id)

	if s.replyCount == 2 {
		log.Printf("[Server %d | STATE=%s | T=%d] Entering critical section", s.id, s.state, s.timestamp)
		s.state = "HELD"
		fmt.Println(s.id, " has entered the critical section: ")
		time.Sleep(10 * time.Second)
		log.Printf("[Server %d | STATE=%s | T=%d] Exiting critical section", s.id, s.state, s.timestamp)
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
