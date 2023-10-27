package kafka

import (
	"context"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"strings"
)

type ITopicConsumer interface {
	Consume(ctx context.Context) <-chan []byte
}

func NewKafkaConsumer(ctx context.Context, brokers []string, consumerGroup string) (*kafka.Consumer, error) {
	return kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers":  strings.Join(brokers, ","),
		"group.id":           consumerGroup,
		"auto.offset.reset":  "earliest",
		"enable.auto.commit": false,
		"fetch.wait.max.ms":  1000,
		// 60 minutes
		"max.poll.interval.ms": 3600000,
	})
}

type TopicConsumer struct {
	consumer *kafka.Consumer
	topic    string
}

func NewTopicConsumer(ctx context.Context, brokers []string, topic string, consumerGroup string) (*TopicConsumer, error) {
	consumer, err := NewKafkaConsumer(ctx, brokers, consumerGroup)
	if err != nil {
		return nil, err
	}
	err = consumer.Subscribe(topic, nil)
	if err != nil {
		return nil, err
	}
	return &TopicConsumer{consumer: consumer, topic: topic}, nil
}

func (tc *TopicConsumer) Commit(msg *kafka.Message) error {
	_, err := tc.consumer.CommitMessage(msg)
	return err
}

func (tc *TopicConsumer) Consume(ctx context.Context) <-chan *kafka.Message {
	msgChan := make(chan *kafka.Message, 100)
	go func() {
		for {
			msg, err := tc.consumer.ReadMessage(-1)
			if err != nil {
				close(msgChan)
				return
			}
			msgChan <- msg
		}
	}()
	return msgChan
}

func CloseConsumer(consumer *kafka.Consumer) {
	consumer.Close()
}
