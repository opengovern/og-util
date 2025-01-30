package jq

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/zap"
)

type consumerInfo struct {
	stream         string
	consumerConfig jetstream.ConsumerConfig
}

type consumeInfo struct {
	consumerInfo consumerInfo
	handler      func(jetstream.Msg)
	opts         []jetstream.PullConsumeOpt
}

type JobQueue struct {
	logger *zap.Logger
	conn   *nats.Conn
	js     jetstream.JetStream

	consumers []consumerInfo
	consumes  []consumeInfo
	streams   []jetstream.StreamConfig
}

func New(url string, logger *zap.Logger) (*JobQueue, error) {
	jq := &JobQueue{
		conn:   nil,
		js:     nil,
		logger: logger.Named("jq"),
	}

	conn, err := nats.Connect(
		url,
		nats.ReconnectHandler(jq.reconnectHandler),
		nats.DisconnectErrHandler(jq.disconnectHandler),
		nats.MaxReconnects(-1),
		nats.PingInterval(10*time.Second),
	)
	if err != nil {
		return nil, err
	}

	jq.conn = conn

	js, err := jetstream.New(conn)
	if err != nil {
		return nil, err
	}

	jq.js = js

	return jq, nil
}

func (jq *JobQueue) reconnectHandler(nc *nats.Conn) {
	jq.logger.Info("got reconnected", zap.String("url", nc.ConnectedUrl()))
	jq.js, _ = jetstream.New(nc)

	for _, stream := range jq.streams {
		_, err := jq.js.CreateOrUpdateStream(context.Background(), stream)
		if err != nil {
			jq.logger.Error("stream re-creation after reconnect failed", zap.Error(err))
		}
	}

	for _, consumer := range jq.consumers {
		_, err := jq.js.CreateOrUpdateConsumer(context.Background(), consumer.stream, consumer.consumerConfig)
		if err != nil {
			jq.logger.Error("consumer re-creation after reconnect failed", zap.Error(err))
		}
	}

	for _, consume := range jq.consumes {
		consumer, err := jq.js.CreateOrUpdateConsumer(context.Background(), consume.consumerInfo.stream, consume.consumerInfo.consumerConfig)
		if err != nil {
			jq.logger.Error("consumer re-creation after reconnect failed", zap.Error(err))
		}
		_, err = consumer.Consume(consume.handler, consume.opts...)
		if err != nil {
			jq.logger.Error("consumer re-consumption after reconnect failed", zap.Error(err))
		}
	}
}

func (jq *JobQueue) disconnectHandler(_ *nats.Conn, err error) {
	jq.logger.Error("got disconnected", zap.Error(err))
}

func (jq *JobQueue) closeHandler(nc *nats.Conn) {
	jq.logger.Fatal("connection lost", zap.Error(nc.LastError()))
}

func (jq *JobQueue) Stream(ctx context.Context, name, description string, topics []string, maxMsgs int64) error {
	// https://docs.nats.io/nats-concepts/jetstream/streams
	config := jetstream.StreamConfig{
		Name:         name,
		Description:  description,
		Subjects:     topics,
		Retention:    jetstream.WorkQueuePolicy,
		MaxConsumers: -1,
		MaxMsgs:      maxMsgs,
		MaxBytes:     1000 * maxMsgs, // we are considering around 50MB for each stream
		Discard:      jetstream.DiscardNew,
		Duplicates:   15 * time.Minute,
		Replicas:     1,
		Storage:      jetstream.MemoryStorage,
	}

	return jq.StreamWithConfig(ctx, name, description, topics, config)
}

func (jq *JobQueue) StreamWithConfig(ctx context.Context, name, description string, topics []string, config jetstream.StreamConfig) error {
	// https://docs.nats.io/nats-concepts/jetstream/streams
	config.Name = name
	config.Description = description
	config.Subjects = topics
	if _, err := jq.js.CreateOrUpdateStream(ctx, config); err != nil {
		return err
	}

	jq.streams = append(jq.streams, config)

	return nil
}

// Consume consumes messages from the given topic using the specified queue group.
// it creates pull consumer which is the only mode that is available in the new version
// of nats.go library.
func (jq *JobQueue) Consume(
	ctx context.Context,
	service string,
	stream string,
	topics []string,
	queue string,
	handler func(jetstream.Msg),
) (jetstream.ConsumeContext, error) {
	return jq.ConsumeWithConfig(ctx, service, stream, topics, jetstream.ConsumerConfig{
		Name:              fmt.Sprintf("%s-service", service),
		Description:       fmt.Sprintf("%s Service", strings.ToTitle(service)),
		FilterSubjects:    topics,
		Durable:           "",
		Replicas:          1,
		AckPolicy:         jetstream.AckExplicitPolicy,
		DeliverPolicy:     jetstream.DeliverAllPolicy,
		MaxAckPending:     -1,
		InactiveThreshold: time.Hour,
	}, nil, handler)
}

// ConsumeWithConfig consumes messages from the given topic using the specified consumer config
// it creates pull consumer which is the only mode that is available in the new version
// of nats.go library.
func (jq *JobQueue) ConsumeWithConfig(
	ctx context.Context,
	service string,
	stream string,
	topics []string,
	config jetstream.ConsumerConfig,
	pullConsumerOpts []jetstream.PullConsumeOpt,
	handler func(jetstream.Msg),
) (jetstream.ConsumeContext, error) {
	config.Name = fmt.Sprintf("%s-service", service)
	config.Description = fmt.Sprintf("%s Service", strings.ToTitle(service))
	config.FilterSubjects = topics

	consumer, err := jq.js.CreateOrUpdateConsumer(ctx, stream, config)
	if err != nil {
		return nil, err
	}

	consumeCtx, err := consumer.Consume(handler, pullConsumerOpts...)
	if err != nil {
		return nil, err
	}

	jq.consumes = append(jq.consumes, consumeInfo{
		consumerInfo: consumerInfo{
			stream:         stream,
			consumerConfig: config,
		},
		handler: handler,
		opts:    pullConsumerOpts,
	})

	return consumeCtx, nil
}

func (jq *JobQueue) CreateOrUpdateConsumer(
	ctx context.Context,
	service string,
	stream string,
	topics []string,
	config jetstream.ConsumerConfig,
) error {
	config.Name = fmt.Sprintf("%s-service", service)
	config.Description = fmt.Sprintf("%s Service", strings.ToTitle(service))
	config.FilterSubjects = topics

	_, err := jq.js.CreateOrUpdateConsumer(ctx, stream, config)
	if err != nil {
		return err
	}

	jq.consumers = append(jq.consumers, consumerInfo{
		stream:         stream,
		consumerConfig: config,
	})

	return nil
}

func (jq *JobQueue) Produce(ctx context.Context, topic string, data []byte, id string) (*uint64, error) {
	pubAck, err := jq.js.Publish(ctx, topic, data, jetstream.WithMsgID(id))
	if err != nil {
		return nil, err
	}

	return &pubAck.Sequence, nil
}

func (jq *JobQueue) DeleteMessage(ctx context.Context, streamName string, sequenceNumber uint64) error {
	stream, err := jq.js.Stream(ctx, streamName)
	if err != nil {
		return err
	}
	err = stream.DeleteMsg(ctx, sequenceNumber)
	if err != nil {
		return err
	}
	return nil
}
