// package stats 提供了一个线程安全的计数器，用于跟踪同步过程中的各项统计数据。
package stats

import "sync/atomic"

// Stats 用于以原子方式跟踪上传、删除、流量等统计信息。
// 所有字段都是非导出的，以强制使用原子操作方法来修改，确保并发安全。
type Stats struct {
	uploads       int64 // 已上传文件总数
	deletes       int64 // 已删除文件总数
	uploadBytes   int64 // 上传总字节数
	downloadBytes int64 // 下载总字节数
	failed        int64 // 失败操作总数
}

// Snapshot 是 Stats 在某个时间点的快照，主要用于方便地进行 JSON 序列化。
type Snapshot struct {
	Uploads       int64 `json:"uploads"`
	Deletes       int64 `json:"deletes"`
	UploadBytes   int64 `json:"uploadBytes"`
	DownloadBytes int64 `json:"downloadBytes"`
	Failed        int64 `json:"failed"`
}

// New 创建并返回一个新的 Stats 实例。
func New() *Stats {
	return &Stats{}
}

// AddUpload 原子地增加上传计数和上传字节数。
func (s *Stats) AddUpload(bytes int64) {
	atomic.AddInt64(&s.uploads, 1)
	atomic.AddInt64(&s.uploadBytes, bytes)
}

// AddDelete 原子地增加删除计数。
func (s *Stats) AddDelete() {
	atomic.AddInt64(&s.deletes, 1)
}

// AddDownload 原子地增加下载字节数。
// 注意：下载操作的次数不单独计数，因为一次下载可能对应多个文件（如列表），或单个文件。
func (s *Stats) AddDownload(bytes int64) {
	atomic.AddInt64(&s.downloadBytes, bytes)
}

// AddFailure 原子地增加失败操作的计数。
func (s *Stats) AddFailure() {
	atomic.AddInt64(&s.failed, 1)
}

// Get 返回一个包含当前所有统计信息值的快照结构体。
// 这是一个线程安全的操作。
func (s *Stats) Get() Snapshot {
	return Snapshot{
		Uploads:       atomic.LoadInt64(&s.uploads),
		Deletes:       atomic.LoadInt64(&s.deletes),
		UploadBytes:   atomic.LoadInt64(&s.uploadBytes),
		DownloadBytes: atomic.LoadInt64(&s.downloadBytes),
		Failed:        atomic.LoadInt64(&s.failed),
	}
}

// GetStats 以多个返回值的形式获取当前统计信息，以兼容旧代码。
func (s *Stats) GetStats() (uploads, deletes, uploadBytes, downloadBytes, failed int64) {
	stats := s.Get()
	return stats.Uploads, stats.Deletes, stats.UploadBytes, stats.DownloadBytes, stats.Failed
}
