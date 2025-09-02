package stats

import "sync/atomic"

// Stats 用于跟踪流量统计信息。
type Stats struct {
	apiUpload      int64
	apiDownload    int64
	webdavUpload   int64
	webdavDownload int64
	apiFailed      int64
	webdavFailed   int64
}

// New 创建一个新的 Stats 实例。
func New() *Stats {
	return &Stats{}
}

// AddAPIStats 添加 API 相关的流量统计。
func (s *Stats) AddAPIStats(upload, download int64, failed bool) {
	atomic.AddInt64(&s.apiUpload, upload)
	atomic.AddInt64(&s.apiDownload, download)
	if failed {
		atomic.AddInt64(&s.apiFailed, 1)
	}
}

// AddWebDAVStats 添加 WebDAV 相关的流量统计。
func (s *Stats) AddWebDAVStats(upload, download int64, failed bool) {
	atomic.AddInt64(&s.webdavUpload, upload)
	atomic.AddInt64(&s.webdavDownload, download)
	if failed {
		atomic.AddInt64(&s.webdavFailed, 1)
	}
}

// GetStats 返回当前的统计信息。
func (s *Stats) GetStats() (apiUp, apiDown, webdavUp, webdavDown, apiFail, webdavFail int64) {
	apiUp = atomic.LoadInt64(&s.apiUpload)
	apiDown = atomic.LoadInt64(&s.apiDownload)
	webdavUp = atomic.LoadInt64(&s.webdavUpload)
	webdavDown = atomic.LoadInt64(&s.webdavDownload)
	apiFail = atomic.LoadInt64(&s.apiFailed)
	webdavFail = atomic.LoadInt64(&s.webdavFailed)
	return
}
