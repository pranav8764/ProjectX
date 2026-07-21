package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

const aiServiceTimeout = 12 * time.Second
const aiLLMServiceTimeout = 90 * time.Second

type aiServiceResponse struct {
	StatusCode int
	Body       []byte
}

func nullableUUID(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func floatValue(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func jsonTextList(raw []byte) []string {
	if len(raw) == 0 {
		return []string{}
	}

	var values []string
	if err := json.Unmarshal(raw, &values); err == nil {
		return values
	}

	var objects []map[string]interface{}
	if err := json.Unmarshal(raw, &objects); err == nil {
		values = []string{}
		for _, item := range objects {
			for _, key := range []string{"event", "cause", "action", "recommendation", "description", "summary"} {
				if value, ok := item[key]; ok {
					values = append(values, fmt.Sprint(value))
					break
				}
			}
		}
		return values
	}

	var generic []interface{}
	if err := json.Unmarshal(raw, &generic); err == nil {
		values = []string{}
		for _, item := range generic {
			values = append(values, fmt.Sprint(item))
		}
		return values
	}

	return []string{string(raw)}
}

func auditMetadataJSON(c *gin.Context, metadata map[string]interface{}) []byte {
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metadata["requestId"] = c.GetString("requestID")
	metadata["method"] = c.Request.Method
	path := c.FullPath()
	if path == "" {
		path = c.Request.URL.Path
	}
	metadata["path"] = path

	payload, err := json.Marshal(metadata)
	if err != nil {
		return []byte("{}")
	}
	return payload
}

func recordAuditEvent(c *gin.Context, dbPool *pgxpool.Pool, plantID, eventType, resourceType, resourceID string, metadata map[string]interface{}) {
	_, _ = dbPool.Exec(c.Request.Context(), `
		INSERT INTO audit.events (organization_id, plant_id, actor_user_id, event_type, resource_type, resource_id, correlation_id, metadata_json)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, c.GetString("orgID"), nullableUUID(plantID), nullableUUID(c.GetString("userID")), eventType, resourceType, resourceID, c.GetString("requestID"), auditMetadataJSON(c, metadata))
}

func callAIService(ctx context.Context, method, aiServiceURL, path string, body []byte) (aiServiceResponse, error) {
	return callAIServiceOpts(ctx, method, aiServiceURL, path, body, aiServiceTimeout, nil)
}

func callAIServiceOpts(ctx context.Context, method, aiServiceURL, path string, body []byte, timeout time.Duration, headers map[string]string) (aiServiceResponse, error) {
	if timeout <= 0 {
		timeout = aiServiceTimeout
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	target := strings.TrimRight(aiServiceURL, "/") + path
	req, err := http.NewRequestWithContext(reqCtx, method, target, bytes.NewReader(body))
	if err != nil {
		return aiServiceResponse{}, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		if v != "" {
			req.Header.Set(k, v)
		}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return aiServiceResponse{}, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return aiServiceResponse{}, err
	}

	return aiServiceResponse{StatusCode: resp.StatusCode, Body: respBody}, nil
}

// aiForwardHeaders propagates request/user identity so the AI service can correlate
// model calls (ai.model_calls.correlation_id) and apply per-user rate limiting.
func aiForwardHeaders(c *gin.Context) map[string]string {
	return map[string]string{
		"X-Request-ID": c.GetString("requestID"),
		"X-User-Id":    c.GetString("userID"),
	}
}

func aiFailureReason(err error, response aiServiceResponse) string {
	if err != nil {
		return err.Error()
	}
	if len(response.Body) == 0 {
		return fmt.Sprintf("AI service returned HTTP %d", response.StatusCode)
	}
	body := strings.TrimSpace(string(response.Body))
	if len(body) > 300 {
		body = body[:300]
	}
	return fmt.Sprintf("AI service returned HTTP %d: %s", response.StatusCode, body)
}
