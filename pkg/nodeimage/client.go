package nodeimage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"nodeimage_webdav_vercel/pkg/logger"
	"nodeimage_webdav_vercel/pkg/stats"
)

// ImageInfo 代表从 NodeImage API 获取的单张图片信息。
type ImageInfo struct {
	ID         string `json:"imageId"`
	Filename   string `json:"filename"`
	Size       int64  `json:"size"`
	URL        string `json:"url"`
	MimeType   string `json:"mimetype"`
	UploadTime string `json:"uploadTime"`
}

// APIResponse 代表 NodeImage API 返回的完整响应结构，包含了图片列表和分页信息。
type APIResponse struct {
	Images     []ImageInfo `json:"images"`
	Pagination struct {
		CurrentPage int  `json:"currentPage"`
		TotalPages  int  `json:"totalPages"`
		TotalCount  int  `json:"totalCount"`
		HasNextPage bool `json:"hasNextPage"`
		HasPrevPage bool `json:"hasPrevPage"`
	} `json:"pagination"`
}

// Client 是一个用于与 NodeImage API 交互的客户端。
type Client struct {
	httpClient *http.Client  // 用于发送 HTTP 请求的客户端
	cookie     string        // 用于 API 认证的 Cookie
	baseURL    string        // NodeImage API 的基础 URL
	logger     logger.Logger // 日志记录器
	stats      *stats.Stats  // 流量统计器
}

// NewClient 创建一个新的 NodeImage API 客户端实例。
func NewClient(cookie, baseURL string, logger logger.Logger, stats *stats.Stats) *Client {
	// 检查环境变量中是否配置了代理
	proxyStr := os.Getenv("HTTP_PROXY_URL")
	var transport *http.Transport
	if proxyStr != "" {
		proxyURL, err := url.Parse(proxyStr)
		if err != nil {
			logger.Warn("警告：无法解析代理 URL '%s'，将不使用代理: %v", proxyStr, err)
		} else {
			logger.Info("检测到代理配置，将通过 %s 发送请求", proxyURL.Host)
			transport = &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			}
		}
	}

	return &Client{
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport, // 如果 transport 为 nil，则使用默认 transport
		},
		cookie:  cookie,
		baseURL: baseURL,
		logger:  logger,
		stats:   stats,
	}
}

// TestConnection 测试与 NodeImage API 的连接是否正常。
func (c *Client) TestConnection(ctx context.Context) error {
	_, err := c.getImageList(ctx, 1, 1)
	if err != nil {
		return fmt.Errorf("测试连接失败: %w。请检查您的 Cookie 和 API URL 是否正确，以及网络连接是否正常", err)
	}
	return nil
}

// GetImageList 从 NodeImage API 获取完整的图片列表。
func (c *Client) GetImageList(ctx context.Context) ([]ImageInfo, error) {
	initialResp, err := c.getImageList(ctx, 1, 1)
	if err != nil {
		return nil, fmt.Errorf("获取初始图片列表失败: %w", err)
	}
	if initialResp.Pagination.TotalCount == 0 {
		return []ImageInfo{}, nil
	}

	resp, err := c.getImageList(ctx, 1, initialResp.Pagination.TotalCount)
	if err != nil {
		return nil, fmt.Errorf("获取完整图片列表失败: %w", err)
	}
	return resp.Images, nil
}

func (c *Client) getImageList(ctx context.Context, page, limit int) (*APIResponse, error) {
	url := fmt.Sprintf("%s?page=%d&limit=%d", c.baseURL, page, limit)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Cookie", c.cookie)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	req.Header.Set("Referer", "https://nodeimage.com/")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.stats.AddAPIStats(0, 0, true)
		return nil, fmt.Errorf("执行请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.stats.AddAPIStats(0, 0, true)
		return nil, fmt.Errorf("读取响应体失败: %w", err)
	}
	c.stats.AddAPIStats(0, int64(len(body)), false)

	if resp.StatusCode != http.StatusOK {
		c.stats.AddAPIStats(0, 0, true)
		return nil, fmt.Errorf("API 返回了非预期的状态码: %d", resp.StatusCode)
	}

	var apiResp APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		preview := string(body)
		if len(preview) > 100 {
			preview = preview[:100]
		}
		return nil, fmt.Errorf("解析 JSON 响应失败: %w。响应体开头: '%s'", err, preview)
	}

	return &apiResp, nil
}

// DownloadImage 根据给定的 URL 下载单张图片。
func (c *Client) DownloadImage(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建下载请求失败: %w", err)
	}
	req.Header.Set("Referer", "https://nodeimage.com/")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.stats.AddAPIStats(0, 0, true)
		return nil, fmt.Errorf("执行下载请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.stats.AddAPIStats(0, 0, true)
		return nil, fmt.Errorf("下载时服务器返回了非预期的状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.stats.AddAPIStats(0, 0, true)
		return nil, fmt.Errorf("读取下载文件内容失败: %w", err)
	}

	c.stats.AddAPIStats(0, int64(len(body)), false)
	return body, nil
}
