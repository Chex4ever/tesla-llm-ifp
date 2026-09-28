package bus

import (
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

const (
	SubjectHeartbeat   = "pirate.agent.heartbeat"
	SubjectEnsureModel = "pirate.agent.ensure_model"
	SubjectInferReq    = "pirate.infer.request"
	SubjectInferResp   = "pirate.infer.response."
	StreamName         = "PIRATE"
)

// InferSubject returns the NATS subject for a node.
func InferSubject(nodeID string) string {
	return SubjectInferReq + "." + nodeID
}

func Connect(url, token string) (*nats.Conn, nats.JetStreamContext, error) {
	opts := []nats.Option{
		nats.Name("pirate-fleet"),
		nats.Timeout(10 * time.Second),
		nats.ReconnectWait(2 * time.Second),
		nats.MaxReconnects(-1),
	}
	if token != "" {
		opts = append(opts, nats.Token(token))
	}
	nc, err := nats.Connect(url, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("nats connect: %w", err)
	}
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, nil, fmt.Errorf("jetstream: %w", err)
	}
	_, _ = js.AddStream(&nats.StreamConfig{
		Name:     StreamName,
		Subjects: []string{"pirate.>"},
		Storage:  nats.FileStorage,
		MaxAge:   24 * time.Hour,
	})
	return nc, js, nil
}
