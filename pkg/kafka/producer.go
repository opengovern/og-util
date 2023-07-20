package kafka

import (
	confluent_kafka "github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"strings"
)

func newKafkaProducer(kafkaServers []string) (*confluent_kafka.Producer, error) {
	return confluent_kafka.NewProducer(&confluent_kafka.ConfigMap{
		"bootstrap.servers": strings.Join(kafkaServers, ","),
		"acks":              "all",
		"retries":           3,
		"linger.ms":         1,
		"batch.size":        1000000,
		"compression.type":  "lz4",
	})
}
