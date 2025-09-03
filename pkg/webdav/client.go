// package webdav 提供了与 WebDAV 服务器交互的客户端。
// 本实现不依赖任何第三方 WebDAV 库，而是直接使用 Go 的标准 `net/http` 包
// 手动构造和发送 PROPFIND, MKCOL, PUT, DELETE 等请求。
package webdav

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"

	"nodeimage_webdav_webui/pkg/logger"
	"nodeimage_webdav_webui/pkg/stats"
)

// Client 封装了与 WebDAV 服务器交互所需的状态和方法。
type Client struct {
	baseURL    string        // WebDAV 服务器的基础 URL, 例如 "https://dav.jianguoyun.com/dav"
	username   string        // 登录用户名
	password   string        // 登录密码或应用专用密码
	httpClient *http.Client  // 用于执行 HTTP 请求的客户端
	stats      *stats.Stats  // 用于记录统计信息
	log        logger.Logger // 用于记录日志
}

// FileInfo 包含了从 WebDAV 服务器获取的单个文件的核心信息。
type FileInfo struct {
	Path string // 文件在 WebDAV 上的完整路径
	Size int64  // 文件大小（字节）
}

// NewClient 创建并返回一个新的 WebDAV 客户端实例。
func NewClient(url, username, password string, stats *stats.Stats, log logger.Logger) *Client {
	return &Client{
		baseURL:    url,
		username:   username,
		password:   password,
		httpClient: &http.Client{},
		stats:      stats,
		log:        log,
	}
}

// Connect 测试与 WebDAV 服务器的连接，并确保基础路径存在。
// 如果基础路径不存在，它会尝试使用 MKCOL 命令创建它。
func (c *Client) Connect(ctx context.Context, basePath string) error {
	// 使用 PROPFIND (Depth: 0) 来检查单个路径是否存在
	req, err := c.newRequest(ctx, "PROPFIND", basePath, nil)
	if err != nil {
		return fmt.Errorf("创建 PROPFIND 请求失败: %w", err)
	}
	req.Header.Set("Depth", "0")

	resp, err := c.do(req)
	if err != nil {
		return fmt.Errorf("连接 WebDAV 失败: %w", err)
	}
	defer resp.Body.Close()

	// 如果服务器返回 404 Not Found，说明目录不存在
	if resp.StatusCode == http.StatusNotFound {
		// 使用 MKCOL 命令创建新目录
		mkcolReq, err := c.newRequest(ctx, "MKCOL", basePath, nil)
		if err != nil {
			return fmt.Errorf("创建 MKCOL 请求失败: %w", err)
		}
		mkcolResp, err := c.do(mkcolReq)
		if err != nil {
			return fmt.Errorf("创建 WebDAV 基础目录 '%s' 失败: %w", basePath, err)
		}
		defer mkcolResp.Body.Close()
		// 201 Created 是成功创建的标准状态码
		if mkcolResp.StatusCode != http.StatusCreated {
			return fmt.Errorf("创建 WebDAV 基础目录 '%s' 失败，状态码: %d", basePath, mkcolResp.StatusCode)
		}
		return nil
	}

	// 207 Multi-Status 或 200 OK 都表示路径存在
	if resp.StatusCode != http.StatusMultiStatus && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("检查 WebDAV 路径 '%s' 失败，状态码: %d", basePath, resp.StatusCode)
	}

	return nil
}

// ListFiles 列出指定路径下的所有文件，只返回文件路径列表。
func (c *Client) ListFiles(ctx context.Context, p string) ([]string, error) {
	infos, err := c.listFilesInternal(ctx, p)
	if err != nil {
		return nil, err
	}
	var filePaths []string
	for _, info := range infos {
		filePaths = append(filePaths, info.Path)
	}
	return filePaths, nil
}

// ListFilesWithStats 列出文件并返回包含大小等统计信息的 FileInfo 列表。
func (c *Client) ListFilesWithStats(ctx context.Context, p string) ([]FileInfo, error) {
	return c.listFilesInternal(ctx, p)
}

// UploadFile 使用 PUT 方法将数据上传到指定路径。
func (c *Client) UploadFile(ctx context.Context, p string, data []byte) error {
	c.stats.AddUpload(int64(len(data)))
	req, err := c.newRequest(ctx, "PUT", p, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("创建 PUT 请求失败: %w", err)
	}
	resp, err := c.do(req)
	if err != nil {
		return fmt.Errorf("上传文件 '%s' 失败: %w", p, err)
	}
	defer resp.Body.Close()

	// 201 Created, 200 OK, 或 204 No Content 都可视为成功
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("上传文件 '%s' 失败，状态码: %d", p, resp.StatusCode)
	}
	return nil
}

// UploadFileStream 使用 PUT 方法从一个 io.Reader 流上传数据到指定路径。
// 这比 UploadFile 更节省内存，因为它避免将整个文件读入内存。
func (c *Client) UploadFileStream(ctx context.Context, p string, data io.Reader, size int64) error {
	c.stats.AddUpload(size)
	// 更新下载统计信息，因为数据流来自下载
	c.stats.AddDownload(size)

	req, err := c.newRequest(ctx, "PUT", p, data)
	if err != nil {
		return fmt.Errorf("创建 PUT 请求失败: %w", err)
	}
	// 设置 Content-Length 对 PUT 请求很重要
	req.ContentLength = size

	resp, err := c.do(req)
	if err != nil {
		return fmt.Errorf("上传文件 '%s' 失败: %w", p, err)
	}
	defer resp.Body.Close()

	// 201 Created, 200 OK, 或 204 No Content 都可视为成功
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("上传文件 '%s' 失败，状态码: %d", p, resp.StatusCode)
	}
	return nil
}

// DeleteFile 使用 DELETE 方法删除指定路径的文件。
func (c *Client) DeleteFile(ctx context.Context, p string) error {
	c.stats.AddDelete()
	req, err := c.newRequest(ctx, "DELETE", p, nil)
	if err != nil {
		return fmt.Errorf("创建 DELETE 请求失败: %w", err)
	}
	resp, err := c.do(req)
	if err != nil {
		return fmt.Errorf("删除文件 '%s' 失败: %w", p, err)
	}
	defer resp.Body.Close()

	// 204 No Content 或 200 OK 都可视为成功
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("删除文件 '%s' 失败，状态码: %d", p, resp.StatusCode)
	}
	return nil
}

// --- 内部辅助方法 ---

// newRequest 是一个创建 HTTP 请求的辅助函数。
// 它能智能处理相对路径和绝对 URL（用于分页）。
func (c *Client) newRequest(ctx context.Context, method, p string, body io.Reader) (*http.Request, error) {
	parsedP, err := url.Parse(p)
	if err != nil {
		return nil, fmt.Errorf("无法解析路径 '%s': %w", p, err)
	}

	var targetURL string
	// 如果 p 是一个完整的 URL (例如，来自 Link 头)，则直接使用它
	if parsedP.IsAbs() {
		targetURL = parsedP.String()
	} else {
		// 否则，将其与 baseURL 拼接
		u, err := url.Parse(c.baseURL)
		if err != nil {
			return nil, err
		}
		u.Path = path.Join(u.Path, parsedP.Path)
		u.RawQuery = parsedP.RawQuery
		targetURL = u.String()
	}

	req, err := http.NewRequestWithContext(ctx, method, targetURL, body)
	if err != nil {
		return nil, err
	}
	// 添加 Basic Auth 认证头
	req.SetBasicAuth(c.username, c.password)
	return req, nil
}

// do 是执行 HTTP 请求的简单封装。
func (c *Client) do(req *http.Request) (*http.Response, error) {
	return c.httpClient.Do(req)
}

// linkNextRegex 用于从 Link 响应头中提取下一页的 URL。
var linkNextRegex = regexp.MustCompile(`<(.+?)>; rel="next"`)

// listFilesInternal 是实现文件列表获取的核心逻辑，支持分页。
func (c *Client) listFilesInternal(ctx context.Context, p string) ([]FileInfo, error) {
	var allFileInfos []FileInfo
	nextPagePath := p // 初始路径用于第一个请求

	for {
		// PROPFIND 请求体，只请求必要的信息以节省流量
		body := `<?xml version="1.0"?>
<d:propfind xmlns:d="DAV:">
  <d:prop>
    <d:displayname/>
    <d:getcontentlength/>
  </d:prop>
</d:propfind>`

		req, err := c.newRequest(ctx, "PROPFIND", nextPagePath, strings.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("创建 PROPFIND 请求失败: %w", err)
		}
		req.Header.Set("Depth", "1") // Depth: 1 表示获取当前目录及其直接子级
		req.Header.Set("Content-Type", "application/xml")

		resp, err := c.do(req)
		if err != nil {
			return nil, fmt.Errorf("读取目录 '%s' 失败: %w", nextPagePath, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusMultiStatus {
			bodyBytes, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("读取目录 '%s' 失败，状态码: %d, 响应: %s", nextPagePath, resp.StatusCode, string(bodyBytes))
		}

		var ms multistatus
		if err := xml.NewDecoder(resp.Body).Decode(&ms); err != nil {
			return nil, fmt.Errorf("解析目录 '%s' 的 XML 响应失败: %w", nextPagePath, err)
		}

		for _, r := range ms.Responses {
			href, err := url.PathUnescape(r.Href)
			if err != nil {
				continue
			}
			// 跳过目录自身，因为 PROPFIND 会把它也包含进来
			currentReqURL, _ := url.Parse(req.URL.String())
			if strings.HasSuffix(strings.TrimRight(href, "/"), strings.TrimRight(currentReqURL.Path, "/")) {
				continue
			}

			// 跳过目录（通常目录没有 getcontentlength 属性）
			if r.Propstat.Prop.GetContentLength == "" {
				continue
			}

			size, _ := strconv.ParseInt(r.Propstat.Prop.GetContentLength, 10, 64)
			allFileInfos = append(allFileInfos, FileInfo{
				Path: path.Join(p, path.Base(href)), // 路径始终基于初始请求路径 p
				Size: size,
			})
		}

		// 检查 Link 头以处理分页
		linkHeader := resp.Header.Get("Link")
		matches := linkNextRegex.FindStringSubmatch(linkHeader)
		if len(matches) > 1 {
			// Link 头提供的是完整的 URL，直接用于下一次请求
			nextPagePath = matches[1]
		} else {
			break // 没有下一页了，退出循环
		}
	}

	return allFileInfos, nil
}

// --- XML 解析结构体 ---
// 这些结构体用于将 WebDAV 服务器返回的 XML 响应 unmarshal 为 Go 对象。
// 字段标签 `xml:"..."` 定义了 XML 元素与结构体字段的映射关系。

type multistatus struct {
	XMLName   xml.Name   `xml:"DAV: multistatus"`
	Responses []response `xml:"response"`
}

type response struct {
	Href     string   `xml:"href"`
	Propstat propstat `xml:"propstat"`
}

type propstat struct {
	Prop   prop   `xml:"prop"`
	Status string `xml:"status"`
}

type prop struct {
	DisplayName      string `xml:"displayname"`
	GetContentLength string `xml:"getcontentlength"`
}
