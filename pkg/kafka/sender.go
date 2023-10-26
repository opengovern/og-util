package kafka

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"time"

	confluent_kafka "github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"go.uber.org/zap"
)

const (
	EsIndexHeader = "elasticsearch_index"
	KafkaPageSize = 5000
)

type Doc interface {
	KeysAndIndex() ([]string, string)
}

func trimEmptyMaps(input map[string]interface{}) {
	for key, value := range input {
		switch value.(type) {
		case map[string]interface{}:
			if len(value.(map[string]interface{})) != 0 {
				trimEmptyMaps(value.(map[string]interface{}))
			}
			if len(value.(map[string]interface{})) == 0 {
				delete(input, key)
			}
		}
	}
}

func trimJsonFromEmptyObjects(input []byte) ([]byte, error) {
	unknownData := map[string]interface{}{}
	err := json.Unmarshal(input, &unknownData)
	if err != nil {
		return nil, err
	}
	trimEmptyMaps(unknownData)
	return json.Marshal(unknownData)
}

func asProducerMessage(r Doc, topic string, partition int32) (*confluent_kafka.Message, error) {
	keys, index := r.KeysAndIndex()
	value, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}

	value, err = trimJsonFromEmptyObjects(value)
	if err != nil {
		return nil, err
	}

	return Msg(HashOf(keys...), value, index, topic, partition), nil
}

func messageID(r Doc) string {
	k, _ := r.KeysAndIndex()
	return fmt.Sprintf("%v", k)
}

func HashOf(strings ...string) string {
	h := sha256.New()
	for _, s := range strings {
		h.Write([]byte(s))
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func Msg(key string, value []byte, index string, topic string, partition int32) *confluent_kafka.Message {
	return &confluent_kafka.Message{
		TopicPartition: confluent_kafka.TopicPartition{
			Topic:     &topic,
			Partition: partition,
		},
		Value:   value,
		Key:     []byte(key),
		Headers: []confluent_kafka.Header{{Key: EsIndexHeader, Value: []byte(index)}},
	}
}

func DoSend(producer *confluent_kafka.Producer, topic string, partition int32, docs []Doc, logger *zap.Logger, LargeDescribeResourceMessage *prometheus.CounterVec) error {
	var msgs []*confluent_kafka.Message
	if partition == -1 {
		partition = confluent_kafka.PartitionAny
	}
	for _, v := range docs {
		msg, err := asProducerMessage(v, topic, partition)
		if err != nil {
			logger.Error(fmt.Sprintf("Failed to convert msg[%s] to Kafka ProducerMessage, ignoring...", messageID(v)), zap.Error(err))
			continue
		}
		msgs = append(msgs, msg)
	}

	for startPageIdx := 0; startPageIdx < len(msgs); startPageIdx += KafkaPageSize {
		msgsToSend := msgs[startPageIdx:min(startPageIdx+KafkaPageSize, len(msgs))]
		var err error
		var failedMessages []*confluent_kafka.Message
		var failedMessagesValues [][]byte
		for retry := 0; retry < 10; retry++ {
			failedMessages, err = SyncSend(logger, producer, msgsToSend, LargeDescribeResourceMessage)
			for _, fm := range failedMessages {
				failedMessagesValues = append(failedMessagesValues, fm.Value)
			}
			if err != nil {
				logger.Error("Failed calling SyncSend", zap.Error(fmt.Errorf("failed Messages: %v, error message: %v", failedMessagesValues, err.Error())))
				if len(failedMessages) == 0 {
					return fmt.Errorf("error message: %v", err.Error())
				}
				if retry == 9 {
					return fmt.Errorf("failed Messages: %v, error message: %v", failedMessagesValues, err.Error())
				}
				msgs = failedMessages
				time.Sleep(15 * time.Second)
				continue
			}
			break
		}
	}
	return nil
}

func SyncSend(logger *zap.Logger, producer *confluent_kafka.Producer, msgs []*confluent_kafka.Message, LargeDescribeResourceMessage *prometheus.CounterVec) ([]*confluent_kafka.Message, error) {
	deliverChan := make(chan confluent_kafka.Event, len(msgs))
	for _, msg := range msgs {
		err := producer.Produce(msg, deliverChan)
		if err != nil {
			logger.Error("Failed calling Produce",
				zap.Error(err),
				zap.String("Kafka Message Size", fmt.Sprintf("%v", len(msg.Value))),
				zap.String("Kafka Message Index", string(msg.Headers[0].Value)))
			if err.Error() == "Broker: Message size too large" {
				if LargeDescribeResourceMessage != nil {
					LargeDescribeResourceMessage.WithLabelValues(string(msg.Headers[0].Value)).Inc()
				}
			}
			return msgs, err
		}
	}

	errList := make([]error, 0)

	failedMessages := make([]*confluent_kafka.Message, 0)
	for ackedMessageCount := 0; ackedMessageCount < len(msgs); {
		select {
		case e, isOpen := <-deliverChan:
			if !isOpen || e == nil {
				break
			}
			switch e.(type) {
			case *confluent_kafka.Message:
				m := e.(*confluent_kafka.Message)
				if m.TopicPartition.Error != nil {
					logger.Error("Delivery failed", zap.Error(m.TopicPartition.Error))
					errList = append(errList, m.TopicPartition.Error)
					failedMessages = append(failedMessages, m)
				} else {
					logger.Debug("Delivered message to topic", zap.String("topic", *m.TopicPartition.Topic))
				}
				ackedMessageCount++
			case confluent_kafka.Error:
				err := e.(confluent_kafka.Error)
				logger.Error("Delivery failed at client level", zap.Error(err))
				errList = append(errList, err)
			default:
				logger.Error("received unknown event type", zap.Any("event", e), zap.String("event sting", e.String()))
			}
		case <-time.After(time.Minute):
			logger.Error("Delivery failed due to timeout")
			return nil, fmt.Errorf("delivery failed due to timeout")
		}
	}
	close(deliverChan)
	if len(errList) > 0 {
		return failedMessages, fmt.Errorf("failed to persist %d resources in kafka with sync send: %v", len(errList), errList)
	}
	return nil, nil
}
