package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/kaytu-io/kaytu-util/pkg/kafka"
	"io"
	"net/http"
	"strings"
	"time"
)

func sendToPipelineIndividually(ingestionPipelineEndpoint string, resourcesToSend []kafka.Doc) error {
	httpClient := &http.Client{Timeout: 10 * time.Second}
	if len(resourcesToSend) == 0 {
		return nil
	}

	for _, resource := range resourcesToSend {
		jsonResourcesToSend, err := json.Marshal(resource)
		if err != nil {
			return err
		}

		req, err := http.NewRequest(
			http.MethodPost,
			ingestionPipelineEndpoint,
			strings.NewReader(string(jsonResourcesToSend)),
		)
		req.Header.Add("Content-Type", "application/json")

		cfg, err := config.LoadDefaultConfig(context.TODO())
		if err != nil {
			return err
		}

		creds, err := cfg.Credentials.Retrieve(context.Background())
		if err != nil {
			return err
		}

		signer := v4.NewSigner()
		err = signer.SignHTTP(context.TODO(), creds, req,
			fmt.Sprintf("%x", sha256.Sum256(jsonResourcesToSend)),
			"osis", "us-east-2", time.Now())
		if err != nil {
			return err
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			return err
		}

		defer resp.Body.Close()
		// check status
		if resp.StatusCode != http.StatusOK {
			bodyStr := ""
			if resp.Body != nil {
				bodyBytes, err := io.ReadAll(resp.Body)
				if err == nil {
					bodyStr = string(bodyBytes)
				}
			}
			return fmt.Errorf("failed to send resources individually to OpenSearch, statusCode=%d, body=%s, requestSize=%d", resp.StatusCode, bodyStr, len(jsonResourcesToSend))
		}
	}
	return nil
}

func SendToPipeline(ingestionPipelineEndpoint string, resourcesToSend []kafka.Doc) error {
	httpClient := &http.Client{Timeout: 10 * time.Second}
	if len(resourcesToSend) == 0 {
		return nil
	}

	for page := 0; page < len(resourcesToSend); page += 100 {
		jsonResourcesToSend, err := json.Marshal(resourcesToSend[page:min(page+100, len(resourcesToSend))])
		if err != nil {
			return err
		}

		req, err := http.NewRequest(
			http.MethodPost,
			ingestionPipelineEndpoint,
			strings.NewReader(string(jsonResourcesToSend)),
		)
		req.Header.Add("Content-Type", "application/json")

		cfg, err := config.LoadDefaultConfig(context.TODO())
		if err != nil {
			return err
		}

		creds, err := cfg.Credentials.Retrieve(context.Background())
		if err != nil {
			return err
		}

		signer := v4.NewSigner()
		err = signer.SignHTTP(context.TODO(), creds, req,
			fmt.Sprintf("%x", sha256.Sum256(jsonResourcesToSend)),
			"osis", "us-east-2", time.Now())
		if err != nil {
			return err
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			return err
		}

		defer resp.Body.Close()
		// check status
		if resp.StatusCode != http.StatusOK {
			if resp.StatusCode == http.StatusRequestEntityTooLarge {
				err = sendToPipelineIndividually(ingestionPipelineEndpoint, resourcesToSend[page:min(page+100, len(resourcesToSend))])
				if err != nil {
					return err
				}
			}
			bodyStr := ""
			if resp.Body != nil {
				bodyBytes, err := io.ReadAll(resp.Body)
				if err == nil {
					bodyStr = string(bodyBytes)
				}
			}
			return fmt.Errorf("failed to send resources to OpenSearch, statusCode=%d, body=%s, requestSize=%d", resp.StatusCode, bodyStr, len(jsonResourcesToSend))
		}
	}
	return nil
}
