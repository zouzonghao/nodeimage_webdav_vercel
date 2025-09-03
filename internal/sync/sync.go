// package sync 包含了项目最核心的同步业务逻辑。
// 它协调 nodeimage 和 webdav 两个客户端，执行文件列表的获取、对比、上传和删除操作。
package sync

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"nodeimage_webdav_webui/pkg/logger"
	"nodeimage_webdav_webui/pkg/nodeimage"
	"nodeimage_webdav_webui/pkg/stats"
	"nodeimage_webdav_webui/pkg/webdav"
)

// --- WebDAV 列表缓存 ---

var (
	// webdavCache 在内存中缓存 WebDAV 文件列表，避免在文件无变化时重复请求。
	webdavCache []webdav.FileInfo
	// cacheMutex 保护对 webdavCache 的并发读写。
	cacheMutex sync.RWMutex
)

// InvalidateWebdavCache 用于在文件系统发生变化（上传或删除）后清空缓存。
// 这是一个导出的函数，以便在需要时可以从其他包调用。
func InvalidateWebdavCache() {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()
	webdavCache = nil
}

// --- 同步逻辑 ---

// Config 聚合了执行一次同步所需的所有配置项。
type Config struct {
	NodeImageCookie string // 用于全量同步
	NodeImageAPIKey string // 用于增量同步
	NodeImageAPIURL string // NodeImage Cookie API 的基础 URL
	WebdavURL       string
	WebdavUsername  string
	WebdavPassword  string
	WebdavBasePath  string // WebDAV 上的同步根目录
}

// Result 包含了单次同步任务执行完成后的详细结果。
type Result struct {
	Success             bool          `json:"Success"`
	Message             string        `json:"Message"`
	Uploaded            int           `json:"Uploaded"`
	Deleted             int           `json:"Deleted"`
	UploadSize          int64         `json:"UploadSize"`
	Duration            time.Duration `json:"Duration"`
	Error               error         `json:"Error,omitempty"`
	TotalNodeImageFiles int           `json:"TotalNodeImageFiles"`
	TotalNodeImageSize  int64         `json:"TotalNodeImageSize"`
	TotalWebDAVFiles    int           `json:"TotalWebDAVFiles"`
	TotalWebDAVSize     int64         `json:"TotalWebDAVSize"`
}

// RunSync 是执行同步流程的主函数。
func RunSync(ctx context.Context, log logger.Logger, config Config, isFullSync bool) Result {
	startTime := time.Now()
	syncMode := "增量同步"
	if isFullSync {
		syncMode = "全量同步"
	}

	log.Info("<-----同步开始 (模式: %s)----->", syncMode)

	// --- 步骤 1: 配置验证 ---
	log.Info("[1/3] 验证配置...")
	if (isFullSync && config.NodeImageCookie == "") || (!isFullSync && config.NodeImageAPIKey == "") || config.WebdavUsername == "" || config.WebdavPassword == "" || config.WebdavBasePath == "" {
		err := fmt.Errorf("模式 '%s' 所需的配置未完全设置", syncMode)
		log.Error("  -> ❌ 配置验证失败: %v", err)
		return Result{Success: false, Message: err.Error(), Error: err}
	}
	if config.NodeImageAPIURL == "" {
		config.NodeImageAPIURL = "https://api.nodeimage.com/api/images"
	}
	if config.WebdavURL == "" {
		config.WebdavURL = "https://dav.jianguoyun.com/dav"
	}
	stats := stats.New()
	nodeImageClient := nodeimage.NewClient(config.NodeImageCookie, config.NodeImageAPIURL, log, stats)
	webdavClient := webdav.NewClient(config.WebdavURL, config.WebdavUsername, config.WebdavPassword, stats, log)

	// --- 步骤 2: 扫描文件 ---
	log.Info("[2/3] 扫描远程文件...")
	if err := webdavClient.Connect(ctx, config.WebdavBasePath); err != nil {
		log.Error("  -> ❌ 连接 WebDAV 失败: %v", err)
		return Result{Success: false, Message: fmt.Sprintf("连接 WebDAV 失败: %v", err), Error: err}
	}

	var nodeImageFiles []nodeimage.ImageInfo
	var err error
	if isFullSync {
		if err := nodeImageClient.TestConnection(ctx); err != nil {
			log.Error("  -> ❌ 连接 NodeImage 失败: %v", err)
			return Result{Success: false, Message: fmt.Sprintf("连接 NodeImage 失败: %v", err), Error: err}
		}
		nodeImageFiles, err = nodeImageClient.GetImageListCookie(ctx)
	} else {
		nodeImageFiles, err = nodeImageClient.GetImageListAPIKey(ctx, config.NodeImageAPIKey)
	}
	if err != nil {
		log.Error("  -> ❌ 获取 NodeImage 文件列表失败: %v", err)
		return Result{Success: false, Message: fmt.Sprintf("获取 NodeImage 文件列表失败: %v", err), Error: err}
	}
	log.Info("  -> [NodeImage] 发现 %d 张图片", len(nodeImageFiles))

	var totalNodeImageSize int64
	for _, file := range nodeImageFiles {
		totalNodeImageSize += file.Size
	}
	totalNodeImageFiles := len(nodeImageFiles)

	if isFullSync {
		InvalidateWebdavCache()
	}
	cacheMutex.RLock()
	cachedFiles := webdavCache
	cacheMutex.RUnlock()

	var webdavFileInfos []webdav.FileInfo
	if cachedFiles != nil {
		webdavFileInfos = cachedFiles
		log.Info("  -> [WebDAV] 从缓存加载 %d 个文件", len(webdavFileInfos))
	} else {
		infos, err := webdavClient.ListFilesWithStats(ctx, config.WebdavBasePath)
		if err != nil {
			log.Error("  -> ❌ 获取 WebDAV 文件列表失败: %v", err)
			return Result{Success: false, Message: fmt.Sprintf("获取 WebDAV 文件列表失败: %v", err), Error: err}
		}
		webdavFileInfos = infos
		cacheMutex.Lock()
		webdavCache = infos
		cacheMutex.Unlock()
		log.Info("  -> [WebDAV] 发现 %d 个文件", len(webdavFileInfos))
	}

	var totalWebDAVSize int64
	var webdavFiles []string
	for _, fileInfo := range webdavFileInfos {
		totalWebDAVSize += fileInfo.Size
		webdavFiles = append(webdavFiles, fileInfo.Path)
	}
	totalWebDAVFiles := len(webdavFiles)

	// --- 步骤 3: 分析并执行同步 ---
	log.Info("[3/3] 分析并执行同步...")
	filesToUpload, filesToDeleteRaw := diffFiles(nodeImageFiles, webdavFiles)
	var filesToDelete []string
	if isFullSync {
		filesToDelete = filesToDeleteRaw
	}

	if len(filesToUpload) == 0 && len(filesToDelete) == 0 {
		log.Info("  -> ✅ 文件已是最新状态，无需操作。")
		duration := time.Since(startTime)
		log.Info("  -> 任务完成，耗时: %s", duration.Round(time.Second))
		return Result{
			Success:             true,
			Message:             "文件已是最新状态，无需同步。",
			Duration:            duration,
			TotalNodeImageFiles: totalNodeImageFiles,
			TotalNodeImageSize:  totalNodeImageSize,
			TotalWebDAVFiles:    totalWebDAVFiles,
			TotalWebDAVSize:     totalWebDAVSize,
		}
	}

	var totalUploadSize int64
	for _, file := range filesToUpload {
		totalUploadSize += file.Size
	}
	log.Info("  -> [计划] 上传: %d 张 (%s)", len(filesToUpload), formatBytes(totalUploadSize))
	if isFullSync {
		log.Info("  -> [计划] 删除: %d 张", len(filesToDelete))
	}

	var wg sync.WaitGroup
	guard := make(chan struct{}, 5)
	var uploadCount, deleteCount int
	var uploadErrCount, deleteErrCount int

	for _, file := range filesToUpload {
		wg.Add(1)
		go func(file nodeimage.ImageInfo) {
			defer wg.Done()
			guard <- struct{}{}
			defer func() { <-guard }()
			err := uploadFile(ctx, file, nodeImageClient, webdavClient, config.WebdavBasePath, log)
			if err != nil {
				log.Error("  -> ❌ 上传失败 %s: %v", file.Filename, err)
				uploadErrCount++
			} else {
				uploadCount++
			}
		}(file)
	}

	if isFullSync {
		for _, file := range filesToDelete {
			wg.Add(1)
			go func(filePath string) {
				defer wg.Done()
				guard <- struct{}{}
				defer func() { <-guard }()
				err := webdavClient.DeleteFile(ctx, filePath)
				if err != nil {
					log.Error("  -> ❌ 删除失败 %s: %v", filePath, err)
					deleteErrCount++
				} else {
					log.Info("  -> ✅ 删除成功: %s", filepath.Base(filePath))
					deleteCount++
				}
			}(file)
		}
	}

	wg.Wait()

	if uploadCount > 0 || deleteCount > 0 {
		InvalidateWebdavCache()
	}

	duration := time.Since(startTime)
	message := fmt.Sprintf("上传: %d (失败: %d), 删除: %d (失败: %d)",
		uploadCount, uploadErrCount, deleteCount, deleteErrCount)

	result := Result{
		Uploaded:            uploadCount,
		Deleted:             deleteCount,
		UploadSize:          totalUploadSize,
		Duration:            duration,
		TotalNodeImageFiles: totalNodeImageFiles,
		TotalNodeImageSize:  totalNodeImageSize,
		TotalWebDAVFiles:    totalWebDAVFiles,
		TotalWebDAVSize:     totalWebDAVSize,
		Message:             message,
	}

	if uploadErrCount > 0 || deleteErrCount > 0 {
		log.Error("  -> ❗ 同步摘要: %s", message)
		result.Success = false
		result.Error = fmt.Errorf("同步过程中有 %d 个上传和 %d 个删除操作失败", uploadErrCount, deleteErrCount)
	} else {
		log.Info("  -> ✅ 同步摘要: %s", message)
		result.Success = true
	}
	log.Info("  -> 任务完成，耗时: %s", duration.Round(time.Second))

	return result
}

// diffFiles 对比 NodeImage 和 WebDAV 的文件列表，找出需要上传和删除的文件。
func diffFiles(nodeImageFiles []nodeimage.ImageInfo, webdavFiles []string) (toUpload []nodeimage.ImageInfo, toDelete []string) {
	webdavFileMap := make(map[string]string)
	for _, f := range webdavFiles {
		webdavFileMap[filepath.Base(f)] = f
	}

	for _, niFile := range nodeImageFiles {
		targetFilename := niFile.Filename
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

// uploadFile 封装了单个文件的下载和上传流程。
func uploadFile(ctx context.Context, file nodeimage.ImageInfo, niClient *nodeimage.Client, wdClient *webdav.Client, basePath string, log logger.Logger) error {
	data, err := niClient.DownloadImage(ctx, file.URL)
	if err != nil {
		return fmt.Errorf("下载失败: %w", err)
	}

	targetPath := filepath.Join(basePath, file.Filename)
	err = wdClient.UploadFile(ctx, targetPath, data)
	if err != nil {
		return fmt.Errorf("上传失败: %w", err)
	}
	log.Info("  -> ✅ 上传成功: %s", file.Filename)
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
	return fmt.Sprintf("%.2f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
