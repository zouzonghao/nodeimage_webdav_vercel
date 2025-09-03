package nodeimage

import (
	"context"
	"fmt"
	"os"
	"testing"

	"nodeimage_webdav_webui/pkg/logger"
	"nodeimage_webdav_webui/pkg/stats"

	"github.com/joho/godotenv"
)

// TestCompressionModes 是一个集成测试，用于验证两种 API 模式是否都成功请求并处理了 zstd 压缩。
//
// 如何运行:
// 1. 确保项目根目录下有一个有效的 .env 文件。
// 2. 在项目根目录下打开终端。
// 3. 运行命令: go test -v ./pkg/nodeimage
//
// 如何验证:
//   - 观察测试输出。
//   - 如果测试通过，并且在日志中看到 "使用 zstd 解压缩响应体" 的 DEBUG 消息，
//     则证明压缩在两种模式下都已成功启用。
func TestCompressionModes(t *testing.T) {
	// 加载 .env 文件，测试需要真实的凭据
	if err := godotenv.Load("../../.env"); err != nil {
		t.Fatalf("测试失败：无法加载 .env 文件。请确保项目根目录下有 .env 文件。错误: %v", err)
	}

	// 从环境变量中获取配置
	cookie := os.Getenv("NODEIMAGE_COOKIE")
	apiKey := os.Getenv("NODEIMAGE_API_KEY")
	baseURL := os.Getenv("NODEIMAGE_API_URL")

	if cookie == "" || apiKey == "" || baseURL == "" {
		t.Fatal("测试失败：.env 文件中缺少 NODEIMAGE_COOKIE, NODEIMAGE_API_KEY 或 NODEIMAGE_API_URL")
	}

	// 创建一个 DEBUG 级别的 logger，以便我们能看到解压缩的日志
	log := logger.New(logger.DEBUG, os.Stdout)
	st := stats.New()
	client := NewClient(cookie, baseURL, log, st)

	ctx := context.Background()

	// --- 测试 1: Cookie 模式 ---
	t.Run("CookieModeCompression", func(t *testing.T) {
		log.Info("--- 开始测试 Cookie 模式的压缩 ---")
		images, err := client.GetImageListCookie(ctx)
		if err != nil {
			t.Errorf("Cookie 模式测试失败: %v", err)
			return
		}
		log.Info("Cookie 模式测试成功，获取到 %d 张图片。", len(images))
		fmt.Println() // 增加一个空行以分隔测试
	})

	// --- 测试 2: API Key 模式 ---
	t.Run("APIKeyModeCompression", func(t *testing.T) {
		log.Info("--- 开始测试 API Key 模式的压缩 ---")
		images, err := client.GetImageListAPIKey(ctx, apiKey)
		if err != nil {
			t.Errorf("API Key 模式测试失败: %v", err)
			return
		}
		log.Info("API Key 模式测试成功，获取到 %d 张图片。", len(images))
		fmt.Println() // 增加一个空行
	})
}
