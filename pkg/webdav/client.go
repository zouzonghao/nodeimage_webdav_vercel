package webdav

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"nodeimage_webdav_vercel/pkg/logger"
	"nodeimage_webdav_vercel/pkg/stats"
)

// Client 代表一个精简的、自实现的 WebDAV 客户端，仅使用 Go 标准库。
type Client struct {
	httpClient *http.Client
	endpoint   *url.URL
	username   string
	password   string
	stats      *stats.Stats
	logger     logger.Logger
}

// NewClient 创建一个新的、精简的 WebDAV 客户端实例。
func NewClient(endpoint, username, password string, stats *stats.Stats, logger logger.Logger) *Client {
	baseURL, err := url.Parse(strings.TrimSuffix(endpoint, "/"))
	if err != nil {
		panic(fmt.Sprintf("无效的 WebDAV URL: %v", err))
	}

	return &Client{
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		endpoint: baseURL,
		username: username,
		password: password,
		stats:    stats,
		logger:   logger,
	}
}

func (c *Client) buildURL(p string) string {
	return c.endpoint.JoinPath(p).String()
}

func (c *Client) buildURLFromHref(href string) string {
	newURL := *c.endpoint
	newURL.Path = href
	return newURL.String()
}

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
	// 对于自实现客户端，我们可以在 ListFiles 中隐式处理目录检查
	// 这里仅作打印日志用
	c.logger.Info("正在连接并检查 WebDAV 路径: %s", path)
	return nil
}

func (c *Client) checkDirExists(ctx context.Context, dirPath string) (bool, error) {
	fullURL := c.buildURL(dirPath)
	req, err := c.newRequest(ctx, "PROPFIND", fullURL, nil)
	if err != nil {
		return false, fmt.Errorf("创建 PROPFIND 请求失败: %w", err)
	}
	req.Header.Set("Depth", "0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("执行 PROPFIND 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true, nil
	}
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	return false, fmt.Errorf("检查目录时返回了非预期的状态码: %d", resp.StatusCode)
}

// UploadFile 使用 PUT 请求将文件数据上传到指定的 WebDAV 路径。
func (c *Client) UploadFile(ctx context.Context, filePath string, data []byte) error {
	fullURL := c.buildURL(filePath)
	req, err := c.newRequest(ctx, http.MethodPut, fullURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("创建 PUT 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.stats.AddWebDAVStats(0, 0, true)
		return fmt.Errorf("执行 PUT 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.stats.AddWebDAVStats(0, 0, true)
		return fmt.Errorf("上传文件失败，服务器返回状态码: %d", resp.StatusCode)
	}

	c.stats.AddWebDAVStats(int64(len(data)), 0, false)
	return nil
}

// ListFiles 使用 PROPFIND 请求列出指定目录下的所有文件。
func (c *Client) ListFiles(ctx context.Context, dirPath string) ([]string, error) {
	fullURL := c.buildURL(dirPath)
	req, err := c.newRequest(ctx, "PROPFIND", fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建 PROPFIND 请求失败: %w", err)
	}
	req.Header.Set("Depth", "1")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.stats.AddWebDAVStats(0, 0, true)
		return nil, fmt.Errorf("执行 PROPFIND 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		c.logger.Warn("远程目录 '%s' 不存在，将尝试创建...", dirPath)
		err := c.createCollection(ctx, dirPath)
		if err != nil {
			return nil, fmt.Errorf("尝试创建目录 '%s' 失败: %w", dirPath, err)
		}
		return []string{}, nil // 返回空列表，因为目录是新建的
	}

	if resp.StatusCode != http.StatusMultiStatus {
		c.stats.AddWebDAVStats(0, 0, true)
		return nil, fmt.Errorf("PROPFIND 请求返回了非预期的状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.stats.AddWebDAVStats(0, 0, true)
		return nil, fmt.Errorf("读取 PROPFIND 响应失败: %w", err)
	}
	c.stats.AddWebDAVStats(0, int64(len(body)), false)

	return c.parseWebDAVResponse(string(body), dirPath)
}

func (c *Client) createCollection(ctx context.Context, dirPath string) error {
	fullURL := c.buildURL(dirPath)
	req, err := c.newRequest(ctx, "MKCOL", fullURL, nil)
	if err != nil {
		return fmt.Errorf("创建 MKCOL 请求失败: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("执行 MKCOL 请求失败: %w", err)
	}
	defer resp.Body.Close()
	// 201 Created is success. 405 Method Not Allowed can mean it already exists.
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusMethodNotAllowed {
		return fmt.Errorf("创建目录失败，服务器返回状态码: %d", resp.StatusCode)
	}
	c.logger.Info("成功创建远程目录: %s", dirPath)
	return nil
}

func (c *Client) parseWebDAVResponse(xmlResponse, dirPath string) ([]string, error) {
	var files []string
	hrefPattern := `<d:href[^>]*>([^<]+)</d:href>`
	re := regexp.MustCompile(hrefPattern)
	matches := re.FindAllStringSubmatch(xmlResponse, -1)

	// 规范化基础路径以进行比较
	normalizedDirPath := strings.TrimSuffix(c.endpoint.Path+dirPath, "/")

	for _, match := range matches {
		if len(match) > 1 {
			href, err := url.PathUnescape(match[1])
			if err != nil {
				c.logger.Warn("无法解码 href: %s", match[1])
				continue
			}

			normalizedHref := strings.TrimSuffix(href, "/")

			// 跳过目录本身
			if normalizedHref == normalizedDirPath {
				continue
			}
			// 跳过子目录（它们以 "/" 结尾）
			if strings.HasSuffix(href, "/") {
				continue
			}
			files = append(files, href)
		}
	}
	return files, nil
}

// DeleteFile 使用 DELETE 请求从 WebDAV 服务器删除一个文件。
func (c *Client) DeleteFile(ctx context.Context, filePath string) error {
	fullURL := c.buildURLFromHref(filePath)
	req, err := c.newRequest(ctx, http.MethodDelete, fullURL, nil)
	if err != nil {
		return fmt.Errorf("创建 DELETE 请求失败: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.stats.AddWebDAVStats(0, 0, true)
		return fmt.Errorf("执行 DELETE 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusNotFound {
			c.logger.Debug("文件删除时已不存在 (404)，视为成功", "路径", filePath)
			return nil
		}
		c.stats.AddWebDAVStats(0, 0, true)
		return fmt.Errorf("删除文件失败，服务器返回状态码: %d", resp.StatusCode)
	}

	c.stats.AddWebDAVStats(0, 0, false)
	return nil
}
