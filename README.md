# NodeImage-WebDAV WebUI

这是一个高效、可靠的 Go 应用，用于将 [NodeImage](https://nodeimage.com/) 图床的图片同步到任何支持 WebDAV 协议的存储服务（如坚果云、Nextcloud 等）。项目提供了一个简单的 Web UI 用于实时监控同步状态。

[![Go Report Card](https://goreportcard.com/badge/github.com/your-username/nodeimage_webdav_webui)](https://goreportcard.com/report/github.com/your-username/nodeimage_webdav_webui)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

---

## 核心特性

- **高效同步**：通过对比两侧文件列表，仅上传新增图片和删除已不存在的图片，最小化网络传输。
- **智能压缩**：在与 NodeImage API 通信时，优先启用 **Zstandard (zstd)** 压缩，大幅减少元数据下载流量。
- **WebDAV 缓存**：内置 WebDAV 文件列表内存缓存，在文件无变化时避免重复请求，节省带宽和时间。
- **手动 WebDAV 实现**：不依赖第三方库，使用 Go 标准 `net/http` 包手动实现 WebDAV 客户端，代码轻量且可控。
- **分页支持**：能够处理 WebDAV 服务器（如坚果云）返回的超长分页列表，确保获取所有文件信息。
- **实时 Web UI**：提供一个简单的 Web 界面，通过 WebSocket 实时显示同步状态和日志，并支持手动触发同步。

## 实现原理

### 1. 同步流程

项目核心的同步逻辑位于 `internal/sync/sync.go` 中，其工作流程如下：

1.  **初始化**：启动时，程序首先初始化 NodeImage 和 WebDAV 的客户端。
2.  **连接测试**：分别尝试连接 NodeImage 和 WebDAV 服务，确保网络和凭据配置正确。
3.  **获取远程列表**：
    *   **NodeImage**：调用 `GetImageList`，通过 `Accept-Encoding: zstd` 请求压缩后的全量图片列表。
    *   **WebDAV**：检查内存中是否存在文件列表缓存。
        *   **有缓存**：直接使用缓存数据。
        *   **无缓存**：通过 `PROPFIND` 请求获取 WebDAV 指定目录下的所有文件，并处理可能的分页（`Link` 头），然后将结果存入缓存。
4.  **差异对比**：对比两侧文件列表的**文件名**，生成一个需要上传的列表和一个需要删除的列表。
5.  **执行同步**：
    *   并发地从 NodeImage 下载需要上传的图片，并 `PUT` 到 WebDAV。
    *   并发地向 WebDAV 发送 `DELETE` 请求，删除多余文件。
6.  **缓存失效**：如果本次同步执行了任何上传或删除操作，则清空 WebDAV 文件列表缓存，确保下次同步时能获取最新的状态。

### 2. Zstandard 压缩

为了减少从 NodeImage API 获取图片列表时的网络流量，客户端在 `pkg/nodeimage/client.go` 中实现了智能压缩：
-   请求头中加入 `Accept-Encoding: zstd, gzip`，表示客户端优先接受 `zstd` 格式的压缩数据。
-   收到响应后，检查 `Content-Encoding` 头。
-   如果值为 `zstd`，则使用 `github.com/klauspost/compress/zstd` 库进行解压。
-   如果值为 `gzip`，则使用标准库 `compress/gzip` 解压。
-   否则，直接读取响应体。

### 3. WebDAV 缓存

WebDAV 目录通常不频繁变动。为了避免每次同步都完整拉取文件列表（这可能是一个耗时且耗流量的操作），在 `internal/sync/sync.go` 中实现了一个简单的包级别内存缓存：
-   一个名为 `webdavCache` 的切片变量用于存储 `[]webdav.FileInfo`。
-   一个读写互斥锁 `cacheMutex` 用于保证并发访问安全。
-   每次 `RunSync` 开始时，会先尝试读取缓存。
-   只有在缓存为空（首次运行）或文件被修改（上传/删除操作成功）后，缓存才会被清空 (`InvalidateWebdavCache`)。

## 部署与运行指南

1.  **克隆代码**
    ```bash
    git clone https://github.com/your-username/nodeimage_webdav_webui.git
    cd nodeimage_webdav_webui
    ```

2.  **创建 `.env` 文件**
    在项目根目录创建一个名为 `.env` 的文件，并填入您的配置信息。
    ```dotenv
    # NodeImage 配置
    NODEIMAGE_COOKIE="从浏览器获取的完整 Cookie"

    # WebDAV 配置
    WEBDAV_URL=https://dav.jianguoyun.com/dav
    WEBDAV_USERNAME=your-email@example.com
    WEBDAV_PASSWORD=your-app-password

    # 同步配置
    WEBDAV_FOLDER=/images
    ```

3.  **构建并运行**
    你可以使用 `go run` 直接运行，或者构建成二进制文件。

    **直接运行:**
    ```bash
    go run main.go
    ```

    **构建二进制文件:**
    ```bash
    go build -o nodeimage-sync .
    ./nodeimage-sync
    ```

4.  **访问 Web UI**
    程序启动后，将在本地启动一个 Web 服务 (默认端口 `37372`)。
    -   在浏览器中打开 `http://localhost:37372`，您可以看到实时日志界面。
    -   点击页面上的 "立即同步" 按钮来手动触发一次同步。
    -   程序还内置了定时任务，会根据 `SYNC_INTERVAL` 的设置自动执行同步。

## 配置说明

应用通过环境变量或根目录下的 `.env` 文件进行配置。

| 变量名 | 描述 | 默认值 |
| :--- | :--- | :--- |
| `NODEIMAGE_COOKIE` | **必需**。登录 NodeImage 后，从浏览器开发者工具中获取的完整 `Cookie` 请求头值。 | |
| `WEBDAV_URL` | **必需**。您的 WebDAV 服务地址。 | |
| `WEBDAV_USERNAME` | **必需**。您的 WebDAV 登录用户名。 | |
| `WEBDAV_PASSWORD` | **必需**。您的 WebDAV **应用专用密码**，通常需要在服务提供商的安全设置中生成。 | |
| `WEBDAV_FOLDER` | **必需**。指定在 WebDAV 根目录下用于存放图片的文件夹路径，以 `/` 开头。 | |
| `NODEIMAGE_API_URL` | NodeImage 的图片列表 API 地址。 | `https://api.nodeimage.com/api/images` |
| `PORT` | 本地运行时监听的端口。 | `37372` |
| `SYNC_INTERVAL` | 自动同步的间隔分钟数。 | `10` |