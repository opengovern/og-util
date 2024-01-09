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

func HashOf(strings ...string) string {
	h := sha256.New()
	for _, s := range strings {
		h.Write([]byte(s))
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
