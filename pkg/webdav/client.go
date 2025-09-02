package webdav

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"nodeimage_webdav_vercel/pkg/logger"
	"nodeimage_webdav_vercel/pkg/stats"

	"golang.org/x/net/webdav"
)

// Client 是一个用于与 WebDAV 服务器交互的客户端。
type Client struct {
	client *webdav.Client
	stats  *stats.Stats
	logger logger.Logger
	url    string
}

// NewClient 创建一个新的 WebDAV 客户端实例。
func NewClient(url, username, password string, stats *stats.Stats, logger logger.Logger) *Client {
	transport := &http.Transport{}
	httpClient := &http.Client{Transport: transport, Timeout: 10 * time.Minute}

	return &Client{
		client: webdav.NewClient(httpClient, url, username, password),
		stats:  stats,
		logger: logger,
		url:    url,
	}
}

// Connect 测试与 WebDAV 服务器的连接。
func (c *Client) Connect(ctx context.Context, basePath string) error {
	err := c.client.Mkdir(ctx, basePath, 0755)
	if err != nil {
		// 如果目录已存在，这不是一个错误
		if e, ok := err.(*webdav.PathError); ok && e.Code == http.StatusMethodNotAllowed {
			return nil
		}
		// 检查是否是 "already exists" 类型的错误，不同服务器返回可能不同
		if strings.Contains(err.Error(), "exists") || strings.Contains(err.Error(), "conflict") {
			return nil
		}
		c.stats.AddWebDAVStats(0, 0, true)
		return fmt.Errorf("无法创建或访问 WebDAV 基础目录 '%s': %w", basePath, err)
	}
	return nil
}

// ListFiles 列出指定目录下的所有文件。
func (c *Client) ListFiles(ctx context.Context, dir string) ([]string, error) {
	files, err := c.client.ReadDir(ctx, dir)
	if err != nil {
		c.stats.AddWebDAVStats(0, 0, true)
		return nil, fmt.Errorf("读取 WebDAV 目录 '%s' 失败: %w", dir, err)
	}

	var filePaths []string
	for _, file := range files {
		if !file.IsDir() {
			filePaths = append(filePaths, path.Join(dir, file.Name()))
		}
	}
	c.stats.AddWebDAVStats(0, int64(len(files)*100), false) // 估算元数据大小
	return filePaths, nil
}

// UploadFile 上传文件到指定路径。
func (c *Client) UploadFile(ctx context.Context, path string, data []byte) error {
	reader := bytes.NewReader(data)
	writer, err := c.client.OpenFile(ctx, path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		c.stats.AddWebDAVStats(0, 0, true)
		return fmt.Errorf("打开 WebDAV 文件 '%s' 失败: %w", path, err)
	}
	defer writer.Close()

	written, err := io.Copy(writer, reader)
	if err != nil {
		c.stats.AddWebDAVStats(0, 0, true)
		return fmt.Errorf("写入 WebDAV 文件 '%s' 失败: %w", path, err)
	}
	c.stats.AddWebDAVStats(written, 0, false)
	return nil
}

// DeleteFile 删除指定路径的文件。
func (c *Client) DeleteFile(ctx context.Context, path string) error {
	err := c.client.Remove(ctx, path)
	if err != nil {
		c.stats.AddWebDAVStats(0, 0, true)
		return fmt.Errorf("删除 WebDAV 文件 '%s' 失败: %w", path, err)
	}
	c.stats.AddWebDAVStats(0, 0, false)
	return nil
}
