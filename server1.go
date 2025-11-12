package main

import (
	pb "Mandatory-Activity-4/grpc"
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"sync"
	"time"

	"google.golang.org/grpc"
)

type mutexServer struct {
	pb.UnimplementedMutexServiceServer

	mu         sync.Mutex
	id         int32
	timestamp  int32
	state      string
	queue      chan [2]int32
	peerAddrs  []string
	peers      map[int]pb.MutexServiceClient
	replyCount int
}

func (s *mutexServer) connectToPeers(peerIDs []int) {
	for i, addr := range s.peerAddrs {
		go func(peerID int, a string) {
			for {
				conn, err := grpc.Dial(a, grpc.WithInsecure())
				if err == nil {
					client := pb.NewMutexServiceClient(conn)
					s.mu.Lock()
					s.peers[peerID] = client
					s.mu.Unlock()

					fmt.Printf("Server %d connected to peer %s (ID=%d)\n", s.id, a, peerID)
					log.Printf("[Server %d | STATE=%s | T=%d] Connected to peer %s", s.id, s.state, s.timestamp, a)
					return
				}
				time.Sleep(time.Second)
			}
		}(peerIDs[i], addr)
	}
}

func (s *mutexServer) simulate() {
	fmt.Printf("Server %d started simulation\n", s.id)
	for {
		if s.state == "RELEASED" {
			if rand.Intn(2) == 0 {
				s.enter()
			}
		}
		time.Sleep(2 * time.Second)
	}
}

func (s *mutexServer) waitForPeers(total int) {
	for {
		s.mu.Lock()
		if len(s.peers) == total {
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()
		time.Sleep(100 * time.Millisecond)
	}
}

func (s *mutexServer) enter() {
	s.mu.Lock()
	s.timestamp++
	s.replyCount = 0
	s.state = "WANTED"

	log.Printf("[Server %d | STATE=%s | T=%d] Wants to enter critical section", s.id, s.state, s.timestamp)
	fmt.Printf("server %d wants to enter the critical section\n", s.id)

	msg := &pb.RequestMessage{
		Id:        s.id,
		Timestamp: s.timestamp,
	}
	s.mu.Unlock()

	ctx := context.Background()

	s.mu.Lock()
	for peerID, client := range s.peers {
		if int32(peerID) != s.id {
			log.Printf("[Server %d | STATE=%s | T=%d] Sent request to %d", s.id, s.state, s.timestamp, peerID)
			go func(c pb.MutexServiceClient, pid int32) {
				_, err := c.Request(ctx, msg)
				if err != nil {
					log.Printf("[Server %d] Error sending request to peer %d: %v", s.id, pid, err)
				}
			}(client, int32(peerID))
		}
	}
	s.mu.Unlock()
}

func (s *mutexServer) Request(ctx context.Context, req *pb.RequestMessage) (*pb.Empty, error) {
	fmt.Printf("Server %d Received a request from %d\n", s.id, req.Id)
	log.Printf("[Server %d | STATE=%s | T=%d] Received REQUEST from %d (T=%d)", s.id, s.state, s.timestamp, req.Id, req.Timestamp)

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == "HELD" || (s.state == "WANTED" &&
		(s.timestamp < req.Timestamp || (s.timestamp == req.Timestamp && s.id < req.Id))) {
		s.queue <- [2]int32{req.Timestamp, req.Id}
		log.Printf("[Server %d | STATE=%s | T=%d] Queued request from %d", s.id, s.state, s.timestamp, req.Id)
	} else {
		msg := &pb.ReplyMessage{
			Id:        s.id,
			Timestamp: s.timestamp,
		}

		s.peers[int(req.Id)].Reply(ctx, msg)
		log.Printf("[Server %d | STATE=%s | T=%d] Sent REPLY to %d", s.id, s.state, s.timestamp, req.Id)
	}

	s.timestamp = max(s.timestamp, req.Timestamp) + 1

	return &pb.Empty{}, nil
}

func (s *mutexServer) Reply(ctx context.Context, rep *pb.ReplyMessage) (*pb.Empty, error) {
	fmt.Printf("Server %d received a reply from %d\n", s.id, rep.Id)
	log.Printf("[Server %d | STATE=%s | T=%d] Received REPLY from %d", s.id, s.state, s.timestamp, rep.Id)

	s.mu.Lock()

	s.replyCount++

	okToEnter := false
	if s.replyCount == 2 && s.state == "WANTED" {
		okToEnter = true
		s.state = "HELD"
	}

	s.mu.Unlock()

	if okToEnter {
		log.Printf("[Server %d] ENTERING critical section", s.id)
		fmt.Println("Server", s.id, "has entered the critical section")

		time.Sleep(3 * time.Second)

		log.Printf("[Server %d] EXITING critical section", s.id)
		fmt.Println("Server", s.id, "left the critical section")

		s.mu.Lock()
		s.state = "RELEASED"

		s.timestamp++

		for len(s.queue) > 0 {
			req := <-s.queue

			msg := &pb.ReplyMessage{
				Id:        s.id,
				Timestamp: s.timestamp,
			}

			go s.peers[int(req[1])].Reply(context.Background(), msg)
			log.Printf("[Server %d] Sent queued REPLY to %d", s.id, req[1])
		}

		s.mu.Unlock()
	}

	return &pb.Empty{}, nil
}

func max(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}

func main() {
	logFile, err := os.OpenFile("server.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	log.SetOutput(logFile)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	go spawnServer(0, ":5050", []string{":5051", ":5052"}, []int{1, 2})
	go spawnServer(1, ":5051", []string{":5050", ":5052"}, []int{0, 2})
	go spawnServer(2, ":5052", []string{":5050", ":5051"}, []int{0, 1})

	select {}
}

func spawnServer(server_id int, port string, peerPorts []string, peerIDs []int) {
	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	s := &mutexServer{
		id:        int32(server_id),
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

	go s.connectToPeers(peerIDs)
	s.waitForPeers(len(peerIDs))
	log.Printf("[Server %d] All peers connected, starting simulation", server_id)

	go s.simulate()
}
