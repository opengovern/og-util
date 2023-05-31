package model

import (
	"sort"
	"strings"
	"time"

	"github.com/lib/pq"
	"gorm.io/gorm"
)

const (
	KaytuPrivateTagPrefix = "x-kaytu-"
	KaytuServiceCostTag   = KaytuPrivateTagPrefix + "cost-service-map"
)

type TagLike interface {
	GetKey() string
	GetValue() []string
}

func GetTagsMap(tags []TagLike) map[string][]string {
	tagsMapToMap := make(map[string]map[string]bool)
	for _, tag := range tags {
		if v, ok := tagsMapToMap[tag.GetKey()]; !ok {
			uniqueMap := make(map[string]bool)
			for _, val := range tag.GetValue() {
				uniqueMap[val] = true
			}
			tagsMapToMap[tag.GetKey()] = uniqueMap

		} else {
			for _, val := range tag.GetValue() {
				v[val] = true
			}
			tagsMapToMap[tag.GetKey()] = v
		}
	}

	result := make(map[string][]string)
	for k, v := range tagsMapToMap {
		for val := range v {
			result[k] = append(result[k], val)
		}
		sort.Slice(result[k], func(i, j int) bool {
			return result[k][i] < result[k][j]
		})
	}

	return result
}

func TrimPrivateTags(tags map[string][]string) map[string][]string {
	for k := range tags {
		if strings.HasPrefix(k, KaytuPrivateTagPrefix) {
			delete(tags, k)
		}
	}
	return tags
}

type Tag struct {
	Key   string         `gorm:"primaryKey;index:idx_key;index:idx_key_value"`
	Value pq.StringArray `gorm:"type:text[];index:idx_key_value"`

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (t Tag) GetKey() string {
	return t.Key
}

func (t Tag) GetValue() []string {
	return t.Value
}

func TagStringsToTagMap(tags []string) map[string][]string {
	tagUniqueMap := make(map[string]map[string]bool)
	for _, tag := range tags {
		key, value, ok := strings.Cut(tag, "=")
		if !ok {
			continue
		}
		if v, ok := tagUniqueMap[key]; !ok {
			tagUniqueMap[key] = map[string]bool{value: true}
		} else {
			v[value] = true
		}
	}

	tagMap := make(map[string][]string)
	for key, values := range tagUniqueMap {
		for value := range values {
			tagMap[key] = append(tagMap[key], value)
		}
	}

	return tagMap
}
