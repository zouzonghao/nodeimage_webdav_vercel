// package nodeimage 提供了与 NodeImage API 进行交互的客户端。
// 它支持两种认证方式：
// 1. Cookie 认证：用于获取全量图片列表，需要用户提供有效的 Cookie。
// 2. API Key 认证：用于获取最近的图片列表（增量更新），更稳定。
package nodeimage

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"nodeimage_webdav_webui/pkg/logger"
	"nodeimage_webdav_webui/pkg/stats"

	"github.com/klauspost/compress/zstd"
)

// --- 数据结构定义 ---

// ImageInfo 是一个通用的图片信息结构体，用于统一两种 API 返回的数据。
type ImageInfo struct {
	ID         string `json:"imageId"`    // 图片的唯一 ID
	Filename   string `json:"filename"`   // 文件名
	Size       int64  `json:"size"`       // 文件大小（字节）
	URL        string `json:"url"`        // 图片的直接下载链接
	MimeType   string `json:"mimetype"`   // 文件类型
	UploadTime string `json:"uploadTime"` // 上传时间
}

// APIResponse 是 Cookie 认证 API (/api/images) 返回的响应结构。
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

// APIKeyImageInfo 是 API Key 认证 API (/api/v1/list) 返回的图片专属结构。
type APIKeyImageInfo struct {
	ImageID    string `json:"image_id"`
	Filename   string `json:"filename"`
	Size       int64  `json:"size"`
	UploadedAt string `json:"uploaded_at"`
	Links      struct {
		Direct string `json:"direct"`
	} `json:"links"`
}

// APIKeyResponse 是 API Key 认证 API 的完整响应结构。
type APIKeyResponse struct {
	Success bool              `json:"success"`
	Images  []APIKeyImageInfo `json:"images"`
}

// --- 客户端实现 ---

// Client 是一个用于与 NodeImage API 交互的客户端。
type Client struct {
	httpClient *http.Client  // 执行 HTTP 请求的客户端
	cookie     string        // 用于全量同步的 Cookie
	baseURL    string        // Cookie 认证 API 的基础 URL
	logger     logger.Logger // 日志记录器
	stats      *stats.Stats  // 统计信息收集器
}

// NewClient 创建一个新的 NodeImage API 客户端实例。
func NewClient(cookie, baseURL string, logger logger.Logger, stats *stats.Stats, httpClient *http.Client) *Client {
	return &Client{
		httpClient: httpClient,
		cookie:     cookie,
		baseURL:    baseURL,
		logger:     logger,
		stats:      stats,
	}
}

// TestConnection 使用 Cookie 认证方式测试与 NodeImage API 的连接是否正常。
func (c *Client) TestConnection(ctx context.Context) error {
	_, err := c.getImageListCookie(ctx, 1, 1) // 尝试获取1条记录
	if err != nil {
		return fmt.Errorf("测试连接失败: %w。请检查您的 Cookie 和 API URL 是否正确", err)
	}
	return nil
}

// GetImageListCookie 使用 Cookie 从 NodeImage API 获取完整的图片列表。
// 它首先获取第一页以确定总数，然后一次性获取所有图片信息。
func (c *Client) GetImageListCookie(ctx context.Context) ([]ImageInfo, error) {
	initialResp, err := c.getImageListCookie(ctx, 1, 1)
	if err != nil {
		return nil, fmt.Errorf("获取初始图片列表失败: %w", err)
	}
	if initialResp.Pagination.TotalCount == 0 {
		return []ImageInfo{}, nil
	}

	resp, err := c.getImageListCookie(ctx, 1, initialResp.Pagination.TotalCount)
	if err != nil {
		return nil, fmt.Errorf("获取完整图片列表失败: %w", err)
	}
	return resp.Images, nil
}

// GetImageListAPIKey 使用 API Key 获取最近的图片列表。
// 返回的数据会被转换为通用的 ImageInfo 结构体。
func (c *Client) GetImageListAPIKey(ctx context.Context, apiKey string) ([]ImageInfo, error) {
	url := "https://api.nodeimage.com/api/v1/list"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建 API Key 请求失败: %w", err)
	}

	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "zstd, gzip") // 添加压缩支持

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.stats.AddFailure()
		return nil, fmt.Errorf("执行 API Key 请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 根据 Content-Encoding 头选择合适的解压器
	reader, err := getDecompressionReader(resp, c.logger)
	if err != nil {
		c.stats.AddFailure()
		return nil, err
	}
	if rc, ok := reader.(io.ReadCloser); ok {
		defer rc.Close()
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		c.stats.AddFailure()
		return nil, fmt.Errorf("读取 API Key 响应体失败: %w", err)
	}
	c.stats.AddDownload(int64(len(body)))

	if resp.StatusCode != http.StatusOK {
		c.stats.AddFailure()
		return nil, fmt.Errorf("API Key API 返回了非预期的状态码: %d", resp.StatusCode)
	}

	var apiKeyResp APIKeyResponse
	if err := json.Unmarshal(body, &apiKeyResp); err != nil {
		return nil, fmt.Errorf("解析 API Key JSON 响应失败: %w", err)
	}

	if !apiKeyResp.Success {
		return nil, fmt.Errorf("API Key API 报告失败 (success: false)")
	}

	// 将 APIKeyImageInfo 转换为通用的 ImageInfo 结构，方便上层统一处理
	var images []ImageInfo
	for _, img := range apiKeyResp.Images {
		images = append(images, ImageInfo{
			ID:         img.ImageID,
			Filename:   img.Filename,
			Size:       img.Size,
			URL:        img.Links.Direct,
			UploadTime: img.UploadedAt,
		})
	}

	return images, nil
}

// getImageListCookie 是实际执行 Cookie 认证 API 请求的内部方法。
// 它支持 zstd 压缩，能自动解压响应体。
func (c *Client) getImageListCookie(ctx context.Context, page, limit int) (*APIResponse, error) {
	url := fmt.Sprintf("%s?page=%d&limit=%d", c.baseURL, page, limit)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Cookie", c.cookie)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "nodeimage-webdav-sync")
	req.Header.Set("Referer", "https://nodeimage.com/")
	req.Header.Set("Accept-Encoding", "zstd, gzip")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.stats.AddFailure()
		return nil, fmt.Errorf("执行请求失败: %w", err)
	}
	defer resp.Body.Close()

	reader, err := getDecompressionReader(resp, c.logger)
	if err != nil {
		c.stats.AddFailure()
		return nil, err
	}
	if rc, ok := reader.(io.ReadCloser); ok {
		defer rc.Close()
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		c.stats.AddFailure()
		return nil, fmt.Errorf("读取响应体失败: %w", err)
	}
	c.stats.AddDownload(int64(len(body)))

	if resp.StatusCode != http.StatusOK {
		c.stats.AddFailure()
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
		c.stats.AddFailure()
		return nil, fmt.Errorf("执行下载请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.stats.AddFailure()
		return nil, fmt.Errorf("下载时服务器返回了非预期的状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.stats.AddFailure()
		return nil, fmt.Errorf("读取下载文件内容失败: %w", err)
	}

	c.stats.AddDownload(int64(len(body)))
	return body, nil
}

// DownloadImageStream 根据给定的 URL 下载单张图片，并返回一个数据流。
// 调用者有责任关闭返回的 io.ReadCloser。
func (c *Client) DownloadImageStream(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建下载请求失败: %w", err)
	}
	req.Header.Set("Referer", "https://nodeimage.com/")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.stats.AddFailure()
		return nil, fmt.Errorf("执行下载请求失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		c.stats.AddFailure()
		resp.Body.Close() // 确保在出错时关闭 body
		return nil, fmt.Errorf("下载时服务器返回了非预期的状态码: %d", resp.StatusCode)
	}

	// 不使用 io.ReadAll，直接返回响应体。
	// 注意：我们不在这里更新下载统计，因为我们不知道最终读取了多少数据。
	// 统计将在数据被消费时（例如上传时）进行。
	return resp.Body, nil
}

// getDecompressionReader 是一个辅助函数，用于根据 HTTP 响应头选择合适的解压器。
func getDecompressionReader(resp *http.Response, logger logger.Logger) (io.Reader, error) {
	switch resp.Header.Get("Content-Encoding") {
	case "zstd":
		zstdReader, err := zstd.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("创建 zstd 解压器失败: %w", err)
		}
		logger.Debug("使用 zstd 解压缩响应体")
		return zstdReader.IOReadCloser(), nil
	case "gzip":
		gzipReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("创建 gzip 解压器失败: %w", err)
		}
		logger.Debug("使用 gzip 解压缩响应体")
		return gzipReader, nil
	default:
		return resp.Body, nil
	}
}
