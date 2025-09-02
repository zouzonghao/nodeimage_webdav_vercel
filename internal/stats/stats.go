package stats

import (
	"math"
)

type Category struct {
	Requests     int64 `json:"requests,omitempty"`
	Bytes        int64 `json:"bytes"`
	Uncompressed int64 `json:"uncompressed"`
	Compressed   int64 `json:"compressed"`
}

type Total struct {
	Bytes             int64   `json:"bytes"`
	Uncompressed      int64   `json:"uncompressed"`
	Savings           int64   `json:"savings"`
	SavingsPercentage float64 `json:"savingsPercentage"`
}

type Summary struct {
	API      Category `json:"api"`
	Download Category `json:"download"`
	Upload   Category `json:"upload"`
	Total    Total    `json:"total"`
}
type Stats struct {
	APIRequests          int64
	APIBytes             int64
	APIUncompressed      int64
	DownloadBytes        int64
	DownloadUncompressed int64
	UploadBytes          int64
	UploadUncompressed   int64
	CompressedAPI        int64
	CompressedDownloads  int64
	CompressedUploads    int64
}

func New() *Stats {
	return &Stats{}
}

func (s *Stats) Reset() {
	*s = Stats{}
}

func (s *Stats) AddAPIStats(bytes, uncompressed int64, compressed bool) {
	s.APIRequests++
	s.APIBytes += bytes
	s.APIUncompressed += uncompressed
	if compressed {
		s.CompressedAPI++
	}
}

func (s *Stats) AddDownloadStats(bytes, uncompressed int64, compressed bool) {
	s.DownloadBytes += bytes
	s.DownloadUncompressed += uncompressed
	if compressed {
		s.CompressedDownloads++
	}
}

func (s *Stats) AddUploadStats(bytes, uncompressed int64, compressed bool) {
	s.UploadBytes += bytes
	s.UploadUncompressed += uncompressed
	if compressed {
		s.CompressedUploads++
	}
}

func (s *Stats) GetSummary() Summary {
	totalBytes := s.APIBytes + s.DownloadBytes + s.UploadBytes
	totalUncompressed := s.APIUncompressed + s.DownloadUncompressed + s.UploadUncompressed
	totalSavings := totalUncompressed - totalBytes
	savingsPercentage := float64(0)
	if totalUncompressed > 0 {
		savingsPercentage = float64(totalSavings) / float64(totalUncompressed) * 100
	}

	return Summary{
		API: Category{
			Requests:     s.APIRequests,
			Bytes:        s.APIBytes,
			Uncompressed: s.APIUncompressed,
			Compressed:   s.CompressedAPI,
		},
		Download: Category{
			Bytes:        s.DownloadBytes,
			Uncompressed: s.DownloadUncompressed,
			Compressed:   s.CompressedDownloads,
		},
		Upload: Category{
			Bytes:        s.UploadBytes,
			Uncompressed: s.UploadUncompressed,
			Compressed:   s.CompressedUploads,
		},
		Total: Total{
			Bytes:             totalBytes,
			Uncompressed:      totalUncompressed,
			Savings:           totalSavings,
			SavingsPercentage: math.Round(savingsPercentage*100) / 100,
		},
	}
}
