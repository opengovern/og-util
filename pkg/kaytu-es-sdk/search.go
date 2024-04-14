package kaytu

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/opensearch-project/opensearch-go/v2/opensearchapi"
	"io"
	"strings"
)

type PointInTime struct {
	ID        string `json:"id"`
	KeepAlive string `json:"keep_alive"`
}

type SearchRequest struct {
	Size        *int64                   `json:"size,omitempty"`
	Query       interface{}              `json:"query,omitempty"`
	PIT         *PointInTime             `json:"pit,omitempty"`
	Sort        []map[string]interface{} `json:"sort,omitempty"`
	SearchAfter []interface{}            `json:"search_after,omitempty"`
}

type SearchTotal struct {
	Value    int64  `json:"value"`
	Relation string `json:"relation"`
}

func (c Client) Search(ctx context.Context, index string, query string, response any) error {
	return c.SearchWithTrackTotalHits(ctx, index, query, nil, response, false)
}

func (c Client) SearchWithFilterPath(ctx context.Context, index string, query string, filterPath []string, response any) error {
	return c.SearchWithTrackTotalHits(ctx, index, query, filterPath, response, false)
}

type CountResponse struct {
	Count int64 `json:"count"`
}

func (c Client) Count(ctx context.Context, index string) (int64, error) {
	opts := []func(count *opensearchapi.CountRequest){
		c.es.Count.WithContext(ctx),
		c.es.Count.WithIndex(index),
	}

	res, err := c.es.Count(opts...)
	defer CloseSafe(res)
	if err != nil {
		return 0, err
	} else if err := CheckError(res); err != nil {
		if IsIndexNotFoundErr(err) {
			return 0, nil
		}
		return 0, err
	}

	b, err := io.ReadAll(res.Body)
	if err != nil {
		return 0, fmt.Errorf("read response: %w", err)
	}

	var response CountResponse
	if err := json.Unmarshal(b, &response); err != nil {
		return 0, fmt.Errorf("unmarshal response: %w", err)
	}
	return response.Count, nil
}

func removeControlChars(s string) string {
	for i := 0; i < 32; i++ {
		s = strings.ReplaceAll(s, string(rune(i)), "")
	}
	return s
}

func (c Client) SearchWithTrackTotalHits(ctx context.Context, index string, query string, filterPath []string, response any, trackTotalHits any) error {
	query = removeControlChars(query)
	opts := []func(*opensearchapi.SearchRequest){
		c.es.Search.WithContext(ctx),
		c.es.Search.WithBody(strings.NewReader(query)),
		c.es.Search.WithTrackTotalHits(trackTotalHits),
		c.es.Search.WithIndex(index),
		c.es.Search.WithFilterPath(filterPath...),
	}

	res, err := c.es.Search(opts...)
	defer CloseSafe(res)
	if err != nil {
		var b []byte
		if res != nil {
			b, _ = io.ReadAll(res.Body)
		}
		fmt.Printf("failure while querying es: %v\n%s\n", err, string(b))
		return err
	} else if err := CheckError(res); err != nil {
		if IsIndexNotFoundErr(err) {
			return nil
		}
		var b []byte
		if res != nil {
			b, _ = io.ReadAll(res.Body)
		}
		fmt.Printf("failure while querying es: %v\n%s\n", err, string(b))
		return err
	}

	b, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if err := json.Unmarshal(b, response); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}
	return nil
}

func (c Client) GetByID(ctx context.Context, index string, id string, response any) error {
	opts := []func(request *opensearchapi.GetRequest){
		c.es.Get.WithContext(ctx),
	}

	res, err := c.es.Get(index, id, opts...)
	defer CloseSafe(res)
	if err != nil {
		return err
	} else if err := CheckError(res); err != nil {
		if IsIndexNotFoundErr(err) {
			return nil
		}
		return err
	}

	b, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if err := json.Unmarshal(b, response); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}
	return nil
}
