package es

import (
	"crypto/sha256"
	"fmt"
)

const (
	EsIndexHeader = "elasticsearch_index"
)

type Doc interface {
	KeysAndIndex() ([]string, string)
}

type DocBase map[string]any

func (d DocBase) GetIdAndIndex() (string, string) {
	return d["es_id"].(string), d["es_index"].(string)
}

func HashOf(strings ...string) string {
	h := sha256.New()
	for _, s := range strings {
		h.Write([]byte(s))
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
