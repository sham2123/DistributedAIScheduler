package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"time"

	// Generate random IDs for the worker
	"github.com/google/uuid"

	// Import our generated Proto definitions
	pb "github.com/yi-json/synapse/api/proto/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	MasterAddr = "localhost:9000"
	WorkerPort = 8080
)

// handles commands from the master
type WorkerServer struct {
	pb.UnimplementedWorkerServer
	WorkerID string
}

// called by the Master when we are assigned a task
func (s *WorkerServer) StartJob(ctx context.Context, req *pb.StartJobRequest) (*pb.StartJobResponse, error) {
	log.Printf("STARTING CONTAINER: JobID=%s, Image=%s", req.JobId, req.Image)

	// execute the carapace runtime
	// assumes the 'carapace' binary is in the same folder
	cmd := exec.Command("./carapace", "run", req.JobId, req.Image, "/bin/sh")

	// wire up the output so we see it in this terminal
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// run in background so we don't block the gRPC return
	go func() {
		if err := cmd.Run(); err != nil {
			log.Printf("Job %s failed: %v", req.JobId, err)
		} else {
			log.Printf("Job %s finished successfully", req.JobId)
		}
	}()

	return &pb.StartJobResponse{
		JobId:   req.JobId,
		Success: true,
	}, nil
}

// TODO: Stub for now, impl later
func (s *WorkerServer) StopJob(ctx context.Context, req *pb.StopJobRequest) (*pb.StopJobResponse, error) {
	log.Printf("STOPPING JOB: %s", req.JobId)
	return &pb.StopJobResponse{Success: true}, nil
}

func main() {
	workerID := uuid.New().String()
	log.Printf("Starting Worker Node - ID: %s", workerID)

	conn, client, err := dialScheduler()
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	response, err := registerWithMaster(ctx, client, workerID)
	if err != nil {
		log.Fatalf("could not register: %v", err)
	}

	log.Printf("Success! Master says: %s", response.Message)

	startHeartbeatLoop(client, workerID)

	if err := serveWorkerCommands(workerID); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

func dialScheduler() (*grpc.ClientConn, pb.SchedulerClient, error) {
	conn, err := grpc.NewClient(MasterAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, err
	}
	return conn, pb.NewSchedulerClient(conn), nil
}

func registerWithMaster(ctx context.Context, client pb.SchedulerClient, workerID string) (*pb.RegisterResponse, error) {
	return client.RegisterWorker(ctx, &pb.RegisterRequest{
		WorkerId:    workerID,
		CpuCores:    4,
		MemoryBytes: 1024 * 1024 * 1024, // 1 GB
		GpuCount:    1,
		Port:        WorkerPort,
	})
}

func startHeartbeatLoop(client pb.SchedulerClient, workerID string) {
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			_, err := client.SendHeartbeat(context.Background(), &pb.HeartbeatRequest{
				WorkerId: workerID,
			})
			if err != nil {
				log.Printf("Heartbeat failed: %v", err)
			} else {
				log.Printf("Pulse sent")
			}
		}
	}()
}

func serveWorkerCommands(workerID string) error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", WorkerPort))
	if err != nil {
		return fmt.Errorf("failed to listen on %d: %w", WorkerPort, err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterWorkerServer(grpcServer, &WorkerServer{WorkerID: workerID})

	log.Printf("Worker listening on port %d...", WorkerPort)
	return grpcServer.Serve(lis)
}
