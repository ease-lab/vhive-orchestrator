package main

import (
	"context"
	"sync"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	log "github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"

	"eventing/proto"
)

var (
	conn   *grpc.ClientConn
	client proto.TimeseriesClient
	lock   sync.Mutex
)

func Start(tdbAddr string, matchers map[string]string) {
	lock.Lock()
	defer lock.Unlock()

	var dialOption grpc.DialOption
	if *withTracing {
		dialOption = grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor())
	} else {
		dialOption = grpc.WithBlock()
	}
	var err error
	conn, err = grpc.Dial(tdbAddr, grpc.WithInsecure(), dialOption)
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}

	client = proto.NewTimeseriesClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := client.StartExperiment(ctx, &proto.ExperimentDefinition{
		CompletionEventDescriptors: []*proto.CompletionEventDescriptor{
			{
				AttrMatchers: matchers,
			},
		},
	}); err != nil {
		log.Fatalln("failed to start experiment", err)
	}
}

func End() (durations []time.Duration) {
	lock.Lock()
	defer lock.Unlock()

	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := client.EndExperiment(ctx, &empty.Empty{})
	if err != nil {
		log.Fatalln("failed to end experiment", err)
	}
	for _, inv := range res.Invocations {
		// Skip incomplete invocations
		if inv.Status != proto.InvocationStatus_COMPLETED {
			continue
		}
		durations = append(durations, inv.Duration.AsDuration())
	}
	return
}
