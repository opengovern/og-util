package kafka

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"

	confluence_kafka "github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"go.uber.org/zap"
)

const (
	EsIndexHeader = "elasticsearch_index"
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

func asProducerMessage(r Doc) (*confluence_kafka.Message, error) {
	keys, index := r.KeysAndIndex()
	value, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}

	value, err = trimJsonFromEmptyObjects(value)
	if err != nil {
		return nil, err
	}

	return Msg(HashOf(keys...), value, index), nil
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

func Msg(key string, value []byte, index string) *confluence_kafka.Message {
	return &confluence_kafka.Message{
		Value:   value,
		Key:     []byte(key),
		Headers: []confluence_kafka.Header{{Key: EsIndexHeader, Value: []byte(index)}},
	}
}

func DoSend(producer *confluence_kafka.Producer, topic string, partition int32, docs []Doc, logger *zap.Logger) error {
	deliverChan := make(chan confluence_kafka.Event, len(docs))
	errChan := make(chan error, len(docs))
	wg := &sync.WaitGroup{}
	go func() {
		for e := range deliverChan {
			switch e.(type) {
			case *confluence_kafka.Error:
				m := e.(*confluence_kafka.Message)
				if m.TopicPartition.Error != nil {
					logger.Error("Delivery failed", zap.Error(m.TopicPartition.Error))
					errChan <- m.TopicPartition.Error
				} else {
					logger.Debug("Delivered message to topic", zap.String("topic", *m.TopicPartition.Topic))
				}
			}
			wg.Done()
		}
	}()

	for _, v := range docs {
		msg, err := asProducerMessage(v)
		if err != nil {
			logger.Error("Failed calling AsProducerMessage", zap.Error(fmt.Errorf("Failed to convert msg[%s] to Kafka ProducerMessage, ignoring...", messageID(v))))
			continue
		}

		// Override the topic and partition if provided
		if partition == -1 {
			partition = confluence_kafka.PartitionAny
		}
		msg.TopicPartition = confluence_kafka.TopicPartition{
			Topic:     &topic,
			Partition: partition,
		}

		err = producer.Produce(msg, deliverChan)
		if err != nil {
			logger.Error("Failed calling Produce", zap.Error(fmt.Errorf("Failed to persist resource[%s] in kafka topic[%s]: %s\nMessage: %v\n", messageID(v), topic, err, msg)))
			continue
		}
		wg.Add(1)
	}

	wg.Wait()
	close(deliverChan)
	errList := make([]error, 0)
	for err := range errChan {
		errList = append(errList, err)
	}
	if len(errList) > 0 {
		return fmt.Errorf("failed to persist %d resources in kafka topic[%s]: %v", len(errList), topic, errList)
	}
	close(errChan)

	return nil
}
