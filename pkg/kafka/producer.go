package kafka

import (
	confluent_kafka "github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"strings"
)

func NewKafkaProducer(kafkaServers []string, configs map[string]interface{}) (*confluent_kafka.Producer, error) {
	cm := confluent_kafka.ConfigMap{
		"bootstrap.servers": strings.Join(kafkaServers, ","),
	}
	if configs != nil {
		for k, v := range configs {
			cm[k] = v
		}
	}

	return confluent_kafka.NewProducer(&cm)
}

func NewDefaultKafkaProducer(kafkaServers []string) (*confluent_kafka.Producer, error) {
	return NewKafkaProducer(kafkaServers, map[string]interface{}{
		"acks":              "all",
		"retries":           3,
		"linger.ms":         1,
		"batch.size":        1000000,
		"compression.type":  "lz4",
		"message.max.bytes": 104857600,
	})
}
