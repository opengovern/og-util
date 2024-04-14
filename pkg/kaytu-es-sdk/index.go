package kaytu

import (
	"context"
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"io"
)

func (c Client) CreateIndexIfNotExist(ctx context.Context, logger *zap.Logger, index string) error {
	res, err := c.es.Indices.Create(index)
	defer CloseSafe(res)
	if err != nil {
		var b []byte
		if res != nil {
			b, _ = io.ReadAll(res.Body)
		}
		logger.Error("failure while creating index", zap.Error(err), zap.String("response", string(b)))
		return err
	} else if err := CheckError(res); err != nil {
		if IsIndexNotFoundErr(err) {
			return nil
		}
		var b []byte
		if res != nil {
			b, _ = io.ReadAll(res.Body)
		}
		logger.Error("failure while creating index", zap.Error(err), zap.String("response", string(b)))
		return err
	}

	return nil
}

func (c Client) ListIndices(ctx context.Context) ([]string, error) {
	res, err := c.es.Cat.Indices()
	defer CloseSafe(res)
	if err != nil {
		return nil, err
	} else if err := CheckError(res); err != nil {
		if IsIndexNotFoundErr(err) {
			return nil, nil
		}
		return nil, err
	}

	b, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var response []map[string]string
	if err := json.Unmarshal(b, &response); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	indices := make([]string, 0)
	for _, index := range response {
		indices = append(indices, index["index"])
	}

	return indices, nil
}