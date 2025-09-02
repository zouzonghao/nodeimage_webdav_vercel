package webdav

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"nodeimage_webdav_vercel/pkg/logger"
	"nodeimage_webdav_vercel/pkg/stats"
)

// Client 代表一个精简的、自实现的 WebDAV 客户端，仅使用 Go 标准库。
type Client struct {
	httpClient *http.Client  // 用于发送所有 HTTP 请求的客户端
	endpoint   *url.URL      // 解析后的 WebDAV 服务器基础 URL
	username   string        // WebDAV 用户名
	password   string        // WebDAV 密码
	stats      *stats.Stats  // 流量统计器
	logger     logger.Logger // 日志记录器
}

// NewClient 创建一个新的、精简的 WebDAV 客户端实例。
func NewClient(endpoint, username, password string, stats *stats.Stats, logger logger.Logger) *Client {
	// 解析并验证传入的 endpoint URL
	baseURL, err := url.Parse(strings.TrimSuffix(endpoint, "/"))
	if err != nil {
		// 如果 URL 无效，这是一个致命的配置错误，程序应立即退出。
		panic(fmt.Sprintf("无效的 WebDAV URL: %v", err))
	}

	return &Client{
		httpClient: &http.Client{
			Timeout: 60 * time.Second, // 为所有请求设置一个合理的超时时间
		},
		endpoint: baseURL,
		username: username,
		password: password,
		stats:    stats,
		logger:   logger,
	}
}

// buildURL 安全地将一个相对路径（如 "/images/pic.jpg"）附加到基础 URL 后面。
func (c *Client) buildURL(p string) string {
	// url.JoinPath 能正确处理路径连接中的斜杠问题。
	return c.endpoint.JoinPath(p).String()
}

// buildURLFromHref 根据基础 URL 的 scheme/host 和一个绝对路径（href）构建一个完整的 URL。
// 这对于处理 PROPFIND 返回的、可能包含不同基础路径的 href 非常重要。
func (c *Client) buildURLFromHref(href string) string {
	// 复制基础 URL 以避免修改原始对象
	newURL := *c.endpoint
	// 将 URL 的路径部分替换为 PROPFIND 返回的完整路径
	newURL.Path = href
	return newURL.String()
}

// newRequest 创建一个新的 HTTP 请求，并自动添加 Basic Auth 认证头。
func (c *Client) newRequest(ctx context.Context, method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.username, c.password)
	return req, nil
}

// Connect 验证与 WebDAV 服务器的连接，并检查指定的基础路径是否存在。
func (c *Client) Connect(ctx context.Context, path string) error {
	exists, err := c.checkDirExists(ctx, path)
	if err != nil {
		return fmt.Errorf("验证 WebDAV 路径 '%s' 失败: %w", path, err)
	}
	if !exists {
		return fmt.Errorf("连接 WebDAV 服务器成功，但指定的路径 '%s' 不存在", path)
	}
	return nil
}

// checkDirExists 使用 PROPFIND 请求（Depth: 0）来验证单个目录是否存在。
// 这是比 Stat 更可靠的方法，特别是对于某些特殊的 WebDAV 服务器实现。
func (c *Client) checkDirExists(ctx context.Context, dirPath string) (bool, error) {
	fullURL := c.buildURL(dirPath)
	c.logger.Debug("使用 PROPFIND 检查目录是否存在", "URL", fullURL)

	req, err := c.newRequest(ctx, "PROPFIND", fullURL, nil)
	if err != nil {
		return false, fmt.Errorf("创建 PROPFIND 请求失败: %w", err)
	}
	req.Header.Set("Depth", "0") // Depth: 0 表示只查询目标本身

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("执行 PROPFIND 请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 任何 2xx 状态码都表示成功，目录存在
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true, nil
	}
	// 404 表示明确不存在
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	// 特殊处理：某些服务器（如坚果云）对已存在的目录返回 410，我们也视其为存在。
	if resp.StatusCode == http.StatusGone {
		c.logger.Debug("服务器为已存在的目录返回 410 Gone，视为存在", "路径", dirPath)
		return true, nil
	}

	return false, fmt.Errorf("检查目录时返回了非预期的状态码: %d", resp.StatusCode)
}

// UploadFile 使用 PUT 请求将文件数据上传到指定的 WebDAV 路径。
// 在上传前，它会先验证目标目录是否存在。
func (c *Client) UploadFile(ctx context.Context, filePath string, data []byte) error {
	// 提取文件所在的目录
	dir := path.Dir(filePath)
	// 根目录不需要检查
	if dir != "/" && dir != "." {
		exists, err := c.checkDirExists(ctx, dir)
		if err != nil {
			return fmt.Errorf("检查远程目录 '%s' 失败: %w", dir, err)
		}
		if !exists {
			return fmt.Errorf("上传失败: 远程目录 '%s' 不存在。请先在 WebDAV 服务器上手动创建该目录", dir)
		}
	}

	fullURL := c.buildURL(filePath)
	c.logger.Debug("正在上传文件 (PUT)", "URL", fullURL)

	req, err := c.newRequest(ctx, http.MethodPut, fullURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("创建 PUT 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream") // 指定为二进制流

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("执行 PUT 请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 任何非 2xx 的状态码都表示上传失败
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("上传文件失败，服务器返回状态码: %d", resp.StatusCode)
	}

	// 记录上传流量
	c.stats.AddUploadStats(int64(len(data)), int64(len(data)), false)
	c.logger.Debug("上传文件成功", "路径", filePath, "大小", len(data))
	return nil
}

// ListFiles 使用 PROPFIND 请求列出指定目录下的所有文件。
func (c *Client) ListFiles(ctx context.Context, dirPath string) ([]string, error) {
	// 规范化目录路径，确保以 "/" 结尾
	cleanPath := path.Clean(dirPath)
	if !strings.HasPrefix(cleanPath, "/") {
		cleanPath = "/" + cleanPath
	}
	if !strings.HasSuffix(cleanPath, "/") {
		cleanPath += "/"
	}

	c.logger.Debug("使用 PROPFIND 方法列出 WebDAV 目录", "路径", cleanPath)
	return c.listFilesWithPROPFIND(ctx, cleanPath)
}

// listFilesWithPROPFIND 是执行 PROPFIND (Depth: 1) 请求的内部实现。
func (c *Client) listFilesWithPROPFIND(ctx context.Context, dirPath string) ([]string, error) {
	fullURL := c.buildURL(dirPath)
	req, err := c.newRequest(ctx, "PROPFIND", fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建 PROPFIND 请求失败: %w", err)
	}
	req.Header.Set("Depth", "1") // Depth: 1 表示查询目标及其直接子级

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("执行 PROPFIND 请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 如果目录不存在，服务器可能返回 404 或 410，这两种情况都视为空目录。
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		c.logger.Debug("远程目录不存在，视为空目录", "路径", dirPath, "状态码", resp.StatusCode)
		return []string{}, nil
	}

	// PROPFIND 的成功响应码是 207 Multi-Status
	if resp.StatusCode != http.StatusMultiStatus {
		return nil, fmt.Errorf("PROPFIND 请求返回了非预期的状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 PROPFIND 响应失败: %w", err)
	}

	return c.parseWebDAVResponse(string(body), dirPath)
}

// parseWebDAVResponse 使用正则表达式从 PROPFIND 的 XML 响应中解析出文件路径列表。
func (c *Client) parseWebDAVResponse(xmlResponse, dirPath string) ([]string, error) {
	var files []string
	// 正则表达式，用于匹配 <d:href>...</d:href> 标签中的内容
	hrefPattern := `<d:href[^>]*>([^<]+)</d:href>`
	re := regexp.MustCompile(hrefPattern)
	matches := re.FindAllStringSubmatch(xmlResponse, -1)
	normalizedDirPath := strings.TrimSuffix(path.Clean(dirPath), "/")

	for _, match := range matches {
		if len(match) > 1 {
			href := match[1]
			normalizedHref := strings.TrimSuffix(href, "/")
			// 跳过目录本身
			if normalizedHref == normalizedDirPath || strings.HasSuffix(href, dirPath) {
				continue
			}
			// 跳过子目录（它们以 "/" 结尾）
			if strings.HasSuffix(href, "/") {
				continue
			}
			files = append(files, href)
		}
	}
	c.logger.Debug("解析到的文件列表", "文件", files, "数量", len(files))
	return files, nil
}

// DeleteFile 使用 DELETE 请求从 WebDAV 服务器删除一个文件。
func (c *Client) DeleteFile(ctx context.Context, filePath string) error {
	// 从 ListFiles 获取的 filePath 是一个绝对路径 (href)，所以我们使用 buildURLFromHref
	fullURL := c.buildURLFromHref(filePath)
	c.logger.Debug("正在删除文件 (DELETE)", "URL", fullURL)

	req, err := c.newRequest(ctx, http.MethodDelete, fullURL, nil)
	if err != nil {
		return fmt.Errorf("创建 DELETE 请求失败: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("执行 DELETE 请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 任何非 2xx 的状态码都表示删除失败
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// 特殊处理：如果文件已经不存在 (404)，我们认为删除是成功的。
		if resp.StatusCode == http.StatusNotFound {
			c.logger.Debug("文件删除时已不存在 (404)，视为成功", "路径", filePath)
			return nil
		}
		return fmt.Errorf("删除文件失败，服务器返回状态码: %d", resp.StatusCode)
	}

	c.logger.Debug("删除文件成功", "路径", filePath)
	return nil
}
