package kaytu

import (
	"context"
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
