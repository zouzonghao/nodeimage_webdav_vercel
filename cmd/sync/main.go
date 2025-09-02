package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"nodeimage_webdav_vercel/pkg/logger"
	"nodeimage_webdav_vercel/pkg/nodeimage"
	"nodeimage_webdav_vercel/pkg/stats"
	"nodeimage_webdav_vercel/pkg/webdav"
)

func main() {
	log := logger.New(logger.INFO, os.Stdout)

	// --- 配置加载 ---
	// 必填项
	nodeImageCookie := os.Getenv("NODEIMAGE_COOKIE")
	webdavUsername := os.Getenv("WEBDAV_USERNAME")
	webdavPassword := os.Getenv("WEBDAV_PASSWORD")
	webdavBasePath := os.Getenv("WEBDAV_BASE_PATH")

	// 可选项 (带默认值)
	nodeImageApiURL := os.Getenv("NODEIMAGE_API_URL")
	if nodeImageApiURL == "" {
		nodeImageApiURL = "https://api.nodeimage.com/api/images"
	}
	webdavURL := os.Getenv("WEBDAV_URL")
	if webdavURL == "" {
		webdavURL = "https://dav.jianguoyun.com/dav"
	}

	// 验证必填项
	if nodeImageCookie == "" || webdavUsername == "" || webdavPassword == "" || webdavBasePath == "" {
		log.Error("错误：必需的环境变量 NODEIMAGE_COOKIE, WEBDAV_USERNAME, WEBDAV_PASSWORD, WEBDAV_BASE_PATH 未完全设置。")
		os.Exit(1)
	}

	// 执行同步逻辑
	err := runSyncLogic(context.Background(), log, nodeImageCookie, nodeImageApiURL, webdavURL, webdavUsername, webdavPassword, webdavBasePath)
	if err != nil {
		log.Error("同步过程中发生错误: %v", err)
		os.Exit(1)
	}

	log.Info("✅ 同步成功完成！")
}

// runSyncLogic 包含了核心的同步业务逻辑。
func runSyncLogic(ctx context.Context, log logger.Logger, nodeImageCookie, nodeImageApiURL, webdavURL, webdavUsername, webdavPassword, webdavBasePath string) error {
	stats := stats.New()

	// 初始化客户端
	nodeImageClient := nodeimage.NewClient(nodeImageCookie, nodeImageApiURL, log, stats)
	webdavClient := webdav.NewClient(webdavURL, webdavUsername, webdavPassword, stats, log)

	// 连接服务
	log.Info("正在连接服务...")
	if err := nodeImageClient.TestConnection(ctx); err != nil {
		return fmt.Errorf("连接 NodeImage 失败: %w", err)
	}
	if err := webdavClient.Connect(ctx, webdavBasePath); err != nil {
		return fmt.Errorf("连接 WebDAV 失败: %w", err)
	}
	log.Info("已成功连接到 NodeImage 和 WebDAV。")

	// 扫描文件
	log.Info("🔍 正在扫描 NodeImage 图片...")
	nodeImageFiles, err := nodeImageClient.GetImageList(ctx)
	if err != nil {
		return fmt.Errorf("获取 NodeImage 文件列表失败: %w", err)
	}
	log.Info("📁 正在扫描 WebDAV 图片...")
	webdavFiles, err := webdavClient.ListFiles(ctx, webdavBasePath)
	if err != nil {
		return fmt.Errorf("获取 WebDAV 文件列表失败: %w", err)
	}

	// 对比文件差异
	filesToUpload, filesToDelete := diffFiles(nodeImageFiles, webdavFiles)

	if len(filesToUpload) == 0 && len(filesToDelete) == 0 {
		log.Info("✅ 文件已是最新状态，无需同步。")
		return nil
	}

	// 打印同步计划
	var totalUploadSize int64
	for _, file := range filesToUpload {
		totalUploadSize += file.Size
	}
	log.Info("🔄 同步计划:")
	log.Info("   需要上传: %d 张 (总大小: %s)", len(filesToUpload), formatBytes(totalUploadSize))
	log.Info("   需要删除: %d 张", len(filesToDelete))

	// 执行同步
	log.Info("正在开始同步...")
	var wg sync.WaitGroup
	guard := make(chan struct{}, 5) // 增加并发数

	for _, file := range filesToUpload {
		wg.Add(1)
		go func(file nodeimage.ImageInfo) {
			defer wg.Done()
			guard <- struct{}{}
			defer func() { <-guard }()
			err := uploadFile(ctx, file, nodeImageClient, webdavClient, webdavBasePath)
			if err != nil {
				log.Error("上传失败 %s: %v", filepath.Base(file.URL), err)
			}
		}(file)
	}

	for _, file := range filesToDelete {
		wg.Add(1)
		go func(filePath string) {
			defer wg.Done()
			guard <- struct{}{}
			defer func() { <-guard }()
			err := webdavClient.DeleteFile(ctx, filePath)
			if err != nil {
				log.Error("删除失败 %s: %v", filePath, err)
			} else {
				log.Info("删除成功: %s", filePath)
			}
		}(file)
	}

	wg.Wait()
	return nil
}

// diffFiles 对比两边的文件列表，找出需要上传和删除的文件。
func diffFiles(nodeImageFiles []nodeimage.ImageInfo, webdavFiles []string) (toUpload []nodeimage.ImageInfo, toDelete []string) {
	webdavFileMap := make(map[string]string)
	for _, f := range webdavFiles {
		webdavFileMap[filepath.Base(f)] = f
	}

	for _, niFile := range nodeImageFiles {
		targetFilename := filepath.Base(niFile.URL)
		if _, exists := webdavFileMap[targetFilename]; !exists {
			toUpload = append(toUpload, niFile)
		}
		delete(webdavFileMap, targetFilename)
	}

	for _, fullPath := range webdavFileMap {
		toDelete = append(toDelete, fullPath)
	}
	return toUpload, toDelete
}

// uploadFile 下载并上传单个文件。
func uploadFile(ctx context.Context, file nodeimage.ImageInfo, niClient *nodeimage.Client, wdClient *webdav.Client, basePath string) error {
	data, err := niClient.DownloadImage(ctx, file.URL)
	if err != nil {
		return fmt.Errorf("下载失败: %w", err)
	}

	targetPath := filepath.Join(basePath, filepath.Base(file.URL))
	err = wdClient.UploadFile(ctx, targetPath, data)
	if err != nil {
		return fmt.Errorf("上传失败: %w", err)
	}
	log.Printf("上传成功: %s", filepath.Base(file.URL))
	return nil
}

// formatBytes 将字节数格式化为更易读的单位 (KB, MB, GB)。
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}
