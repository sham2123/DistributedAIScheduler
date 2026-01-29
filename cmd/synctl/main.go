package main

import (
	"context"
	"flag"
	"log"
	"time"

	pb "github.com/yi-json/synapse/api/proto/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const MasterAddr = "localhost:9000"

type jobRequest struct {
	cpu   int
	memMB int
	gpu   int
	image string
}

func main() {
	params := parseFlags()

	conn, client, err := dialScheduler()
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	jobID := "job-" + time.Now().Format("150405")
	log.Printf("Submitting Job %s...", jobID)

	resp, err := submitJob(ctx, client, params, jobID)
	if err != nil {
		log.Fatalf("Submission Failed: %v", err)
	}

	log.Printf("Master Response: %s (Job ID: %s)", resp.Message, resp.JobId)
}

func parseFlags() jobRequest {
	cpu := flag.Int("cpu", 1, "CPU Cores required")
	mem := flag.Int("mem", 128, "Memory required in MB")
	gpu := flag.Int("gpu", 0, "GPUs required")
	image := flag.String("image", "ubuntu:latest", "Docker image to run")
	flag.Parse()

	return jobRequest{
		cpu:   *cpu,
		memMB: *mem,
		gpu:   *gpu,
		image: *image,
	}
}

func dialScheduler() (*grpc.ClientConn, pb.SchedulerClient, error) {
	conn, err := grpc.NewClient(MasterAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, err
	}
	return conn, pb.NewSchedulerClient(conn), nil
}

func submitJob(ctx context.Context, client pb.SchedulerClient, params jobRequest, jobID string) (*pb.SubmitJobResponse, error) {
	return client.SubmitJob(ctx, &pb.SubmitJobRequest{
		Id:        jobID,
		Image:     params.image,
		MinCpu:    int32(params.cpu),
		MinMemory: int64(params.memMB * 1024 * 1024),
		MinGpu:    int32(params.gpu),
	})
}
