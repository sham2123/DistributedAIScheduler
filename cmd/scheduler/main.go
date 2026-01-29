package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	// import generated Protobuf code - The "Language" we speak
	pb "github.com/yi-json/synapse/api/proto/v1"

	// import our internal logic - the "Brain"
	"github.com/yi-json/synapse/internal/scheduler"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// SchedulerServer wraps our internal logic so it can talk gRPC
// it doesn't store state directly; it delegates to the ClusterManager
type SchedulerServer struct {
	pb.UnimplementedSchedulerServer

	// dependency injection: we rely on the interface, not the specific implementation
	cluster scheduler.ClusterManager
}

// handles initial handshake from a new worker node
func (s *SchedulerServer) RegisterWorker(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	// log the incoming request - observability
	log.Printf("[GRPC] RegisterWorker: ID=%s, CPU=%d, RAM=%d", req.WorkerId, req.CpuCores, req.MemoryBytes)

	// convert the proto (wire format) -> internal Node struct (domain object)
	newNode := &scheduler.Node{
		ID:          req.WorkerId,
		CPUCores:    int(req.CpuCores),
		MemoryBytes: req.MemoryBytes,
		GPUCount:    int(req.GpuCount),
		Port:        int(req.Port),
	}

	// delegate to the business logic layer
	err := s.cluster.RegisterNode(newNode)
	if err != nil {
		log.Printf("[ERROR] Failed to register node: %v", err)
		return &pb.RegisterResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	// success
	return &pb.RegisterResponse{
		Success: true,
		Message: "Welcome to Synapse",
	}, nil
}

// allows a worker to ping the master to indicate liveness
func (s *SchedulerServer) SendHeartbeat(ctx context.Context, req *pb.HeartbeatRequest) (*pb.HeartbeatResponse, error) {
	log.Printf("[GRPC] Heartbeat from %s", req.WorkerId)

	// delegate to the internal cluster logic
	err := s.cluster.UpdateHeartbeat(req.WorkerId)
	if err != nil {
		log.Printf("[ERROR] Heartbeat failed for %s: %v", req.WorkerId, err)
		return nil, err
	}

	// success
	return &pb.HeartbeatResponse{Acknowledge: true}, nil
}

// handles a request from a user/CLI to run a task
func (s *SchedulerServer) SubmitJob(ctx context.Context, req *pb.SubmitJobRequest) (*pb.SubmitJobResponse, error) {
	log.Printf("[GRPC] Job Submitted: %s (CPU: %d, GPU: %d)", req.Id, req.MinCpu, req.MinGpu)

	// create the internal Job struct
	job := &scheduler.Job{
		ID:        req.Id,
		Image:     req.Image,
		MinCPU:    int(req.MinCpu),
		MinMemory: req.MinMemory,
		MinGPU:    int(req.MinGpu),
	}

	// 2. add to Queue
	s.cluster.SubmitJob(job)

	// 3. Respond
	return &pb.SubmitJobResponse{
		JobId:   req.Id,
		Success: true,
		Message: "Job queued successfully",
	}, nil
}

func newSchedulerServer(cluster scheduler.ClusterManager) *SchedulerServer {
	return &SchedulerServer{cluster: cluster}
}

func runReaper(cluster scheduler.ClusterManager) {
	ticker := time.NewTicker(5 * time.Second)
	go func() {
		for range ticker.C {
			deadIDs := cluster.MarkDeadNodes(10 * time.Second)
			for _, id := range deadIDs {
				log.Printf("REAPER: Node %s is DEAD (Missed Heartbeats)", id)
			}
		}
	}()
}

func runDispatcher(cluster scheduler.ClusterManager) {
	ticker := time.NewTicker(1 * time.Second)
	go func() {
		for range ticker.C {
			for _, job := range cluster.Schedule() {
				log.Printf("Dispatching Job %s to %d nodes...", job.ID, len(job.AssignedNodes))
				for _, nodeID := range job.AssignedNodes {
					go dispatchJobToNode(cluster, job, nodeID)
				}
			}
		}
	}()
}

func dispatchJobToNode(cluster scheduler.ClusterManager, job *scheduler.Job, nodeID string) {
	node, err := cluster.GetNode(nodeID)
	if err != nil {
		log.Printf("Error retrieving node %s: %v", nodeID, err)
		return
	}

	addr := fmt.Sprintf("localhost:%d", node.Port)
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Printf("Failed to connect to worker %s: %v", nodeID, err)
		return
	}
	defer conn.Close()

	workerClient := pb.NewWorkerClient(conn)
	_, err = workerClient.StartJob(context.Background(), &pb.StartJobRequest{
		JobId: job.ID,
		Image: job.Image,
	})
	if err != nil {
		log.Printf("Failed to start job on worker %s: %v", nodeID, err)
	} else {
		log.Printf("Worker %s started job %s", nodeID, job.ID)
	}
}

func main() {
	// setup networking: listen on TCP port 9000
	lis, err := net.Listen("tcp", ":9000")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	// initialize the logic: create the state store
	clusterManager := scheduler.NewInMemoryCluster()

	// initialize the server: inject the logic into the new gRPC server
	grpcServer := grpc.NewServer()
	schedulerServer := newSchedulerServer(clusterManager)

	// register the service so gRPC knows where to send requests
	pb.RegisterSchedulerServer(grpcServer, schedulerServer)

	runReaper(clusterManager)
	runDispatcher(clusterManager)

	// start blocking loop
	log.Printf("Synapse Master running on :9000...")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
