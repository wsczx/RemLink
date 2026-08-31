package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// 执行 HTTP 请求并将 JSON 响应解析到 out。timeout=0 默认 10s。
func fetchJSON(apiName, method, url string, body io.Reader, headers map[string]string, out any, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return fmt.Errorf("[%s] 创建请求失败: %w", apiName, err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("[%s] 请求失败: %w", apiName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		preview, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("[%s] 返回非 200 状态码: %d, 响应摘要: %.512s", apiName, resp.StatusCode, string(preview))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("[%s] 读取响应体失败: %w", apiName, err)
	}

	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("[%s] JSON 解析失败: %w, 响应摘要: %.256s", apiName, err, string(respBody))
	}

	return nil
}
