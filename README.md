# NodeImage-WebDAV-Sync

这是一个高效、可靠的 Go 应用，用于将 [NodeImage](https://nodeimage.com/) 图床的图片同步到任何支持 WebDAV 协议的存储服务（如坚果云、Nextcloud 等）。项目提供了一个简洁的 Web UI，用于实时监控同步状态和手动触发同步任务。

[![Go Report Card](https://goreportcard.com/badge/github.com/your-username/nodeimage_webdav_vercel)](https://goreportcard.com/report/github.com/your-username/nodeimage_webdav_vercel)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

---

## 核心特性

- **双同步模式**：
  - **全量同步 (Cookie)**：使用 `NODEIMAGE_COOKIE` 获取 NodeImage 上的**全部**图片列表，与 WebDAV 对比，执行上传和删除，确保两侧文件完全一致。
  - **增量同步 (API Key)**：使用 `NODEIMAGE_API_KEY` 获取 NodeImage 上的**最新**图片列表，仅上传 WebDAV 缺失的新图片，**不执行删除**。速度更快，适合定时任务。
- **高效传输**：
  - **智能压缩**：与 NodeImage API 通信时，优先启用 **Zstandard (zstd)** 压缩，大幅减少元数据下载流量。
  - **流式处理**：所有文件的下载和上传均采用流式处理，无需将整个文件读入内存，同步超大文件时也能保持极低的内存占用。
- **优化性能**：
  - **WebDAV 缓存**：内置 WebDAV 文件列表内存缓存，在文件无变化时避免重复请求，节省带宽和时间。
  - **并发同步**：所有上传和删除操作均并发执行，可配置并发数，最大化利用网络带宽。
- **强大兼容性**：
  - **手动 WebDAV 实现**：不依赖第三方库，使用 Go 标准 `net/http` 包手动实现 WebDAV 客户端，代码轻量且可控。
  - **分页支持**：能够自动处理 WebDAV 服务器（如坚果云）返回的超长分页列表，确保在文件数量巨大时也能获取所有文件信息。
- **友好交互**：
  - **实时 Web UI**：提供一个简单的 Web 界面，通过 WebSocket 实时显示同步状态和日志。
  - **在线更新凭据**：支持在 Web UI 上临时输入 Cookie 或 API Key，无需修改配置文件或重启服务即可执行一次性同步任务。

## 实现原理

### 1. 同步逻辑

项目核心的同步逻辑位于 `internal/sync/sync.go` 中，其工作流程如下：

1.  **初始化**：根据环境变量或 `.env` 文件加载配置，初始化 NodeImage 和 WebDAV 的客户端。
2.  **模式选择**：根据用户触发的是“全量”还是“增量”同步，选择不同的凭据（Cookie 或 API Key）和不同的逻辑分支。
3.  **获取远程列表**：
    *   **NodeImage**：
        *   **全量模式**：使用 Cookie 调用 `/api/images` 接口，获取所有图片信息。
        *   **增量模式**：使用 API Key 调用 `/api/v1/list` 接口，获取最新图片信息。
        *   *(两个接口都优先使用 `zstd` 压缩传输)*
    *   **WebDAV**：检查内存中是否存在文件列表缓存。
        *   **有缓存**：直接使用缓存数据（仅限增量模式）。
        *   **无缓存**：通过 `PROPFIND` 请求获取 WebDAV 指定目录下的所有文件，并自动处理可能的分页（`Link` 头），然后将结果存入缓存。
4.  **差异对比**：对比两侧文件列表的**文件名**，生成一个需要上传的列表和一个需要删除的列表。
    *   *（注：增量模式下，删除列表会被忽略）*
5.  **执行同步**：
    *   并发地从 NodeImage **流式下载**需要上传的图片，并**流式上传**到 WebDAV。
    *   并发地向 WebDAV 发送 `DELETE` 请求，删除多余文件（仅限全量模式）。
6.  **缓存失效**：如果本次同步执行了任何上传或删除操作，则清空 WebDAV 文件列表缓存，确保下次同步时能获取最新的状态。

### 2. Web UI 交互

Web UI 通过 `main.go` 中定义的 API 与后端通信：

-   `/`：提供 `./public` 目录下的静态文件（HTML, CSS, JS）。
-   `/ws`：建立 WebSocket 连接，后端通过它实时推送日志和状态更新。
-   `/api/config`：
    -   `GET`：检查后端是否已配置凭据，用于前端 UI 显示。
    -   `POST`：允许前端提交一个临时的 Cookie 或 API Key。这个凭据仅保存在内存中，用于当次同步，不会写入 `.env` 文件。
-   `/api/sync`：
    -   `POST`：触发一次同步任务。通过 `?mode=full` 查询参数来区分是全量还是增量同步。

## 部署与运行指南

1.  **克隆代码**


2.  **创建 `.env` 文件**
    在项目根目录创建一个名为 `.env` 的文件。
    ```dotenv
    # --- NodeImage 配置 (二选一或全选) ---
    # 用于“全量同步”，从浏览器获取的完整 Cookie
    NODEIMAGE_COOKIE=""
    # 用于“增量同步”，在 NodeImage 个人主页申请
    NODEIMAGE_API_KEY=""

    # --- WebDAV 配置 (必需) ---
    WEBDAV_URL="https://dav.jianguoyun.com/dav"
    WEBDAV_USERNAME="your-email@example.com"
    WEBDAV_PASSWORD="your-app-password"
    WEBDAV_FOLDER="/NodeImageBackup" # 必须以 / 开头

    # --- 可选配置 ---
    PORT="37372" # Web UI 访问端口
    SYNC_INTERVAL="10" # 自动执行增量同步的间隔分钟数，0为禁用
    SYNC_CONCURRENCY="5" # 同步时的并发数
    LOG_LEVEL="info" # 日志级别 (debug, info, warn, error)
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
    程序启动后，在浏览器中打开 `http://localhost:37372` (或您自定义的端口)。
    -   您可以看到实时日志界面。
    -   点击 "增量同步" 或 "全量同步" 按钮来手动触发任务。
    -   您也可以在输入框中粘贴临时的凭据来覆盖 `.env` 中的配置，执行一次性的同步。

## 配置说明

应用通过环境变量或根目录下的 `.env` 文件进行配置。

| 变量名 | 描述 | 默认值 |
| :--- | :--- | :--- |
| `NODEIMAGE_COOKIE` | **全量同步必需**。登录 NodeImage 后，从浏览器开发者工具中获取的完整 `Cookie` 请求头值。 | |
| `NODEIMAGE_API_KEY` | **增量同步必需**。在 NodeImage 个人主页申请的 API Key。 | |
| `WEBDAV_URL` | **必需**。您的 WebDAV 服务地址。 | |
| `WEBDAV_USERNAME` | **必需**。您的 WebDAV 登录用户名。 | |
| `WEBDAV_PASSWORD` | **必需**。您的 WebDAV **应用专用密码**，通常需要在服务提供商的安全设置中生成。 | |
| `WEBDAV_FOLDER` | **必需**。指定在 WebDAV 根目录下用于存放图片的文件夹路径，以 `/` 开头。 | |
| `NODEIMAGE_API_URL` | NodeImage 的图片列表 API 地址 (Cookie 模式使用)。 | `https://api.nodeimage.com/api/images` |
| `PORT` | 本地运行时监听的端口。 | `373722` |
| `SYNC_INTERVAL` | 自动**增量**同步的间隔分钟数。设为 `0` 则禁用。 | `0` |
| `SYNC_CONCURRENCY` | 上传/删除操作的并发线程数。 | `5` |
| `LOG_LEVEL` | 应用的日志级别，可选 `debug`, `info`, `warn`, `error`。 | `info` |
| `PASSWORD` | 网页的登录密码，用于保护 Web UI。 |  |