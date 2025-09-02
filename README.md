# NodeImage WebDAV Sync - Vercel 版

这是一个可以通过 Vercel 一键部署的 Web 应用，用于将您的 NodeImage 图库同步到 WebDAV 存储。

它提供了一个简单的 Web 界面来触发同步过程，并实时显示同步日志。

## 功能特性

- **Web 界面**：通过简单的按钮点击即可启动同步。
- **实时日志**：使用服务器发送事件 (SSE) 技术，在浏览器中实时显示同步进度。
- **环境变量配置**：所有敏感信息（如 Cookie、密码）都通过 Vercel 的环境变量进行安全配置。
- **Serverless**：后端逻辑部署为 Go Serverless Function，具有高弹性和低成本。
- **零依赖**：WebDAV 客户端完全自实现，不依赖任何第三方库。

## 如何部署

部署此项目到 Vercel 非常简单，只需按照以下步骤操作：

### 1. Fork 本项目

首先，您需要将此 GitHub 项目 Fork 到您自己的 GitHub 账户下。

### 2. 在 Vercel 上导入项目

- 登录到您的 [Vercel](https://vercel.com/) 账户。
- 点击 "Add New..." -> "Project"。
- 从列表中选择您刚刚 Fork 的 GitHub 仓库，然后点击 "Import"。

### 3. 配置项目

- **框架预设 (Framework Preset)**：Vercel 应该会自动检测到这是一个 Go 项目。如果没有，请选择 "Other"。
- **根目录 (Root Directory)**：**这一点非常重要！** 请将根目录设置为 `vercel`。
- 点击 "Deploy" 按钮。

### 4. 配置环境变量

部署开始后，Vercel 会提示您缺少环境变量。请转到项目的 "Settings" -> "Environment Variables" 页面，并添加以下变量：

| 变量名                  | 必填 | 描述                                                              | 示例值                                            |
| ----------------------- | ---- | ----------------------------------------------------------------- | ------------------------------------------------- |
| `NODEIMAGE_COOKIE`      | 是   | 您登录 NodeImage 网站后获取的完整 Cookie 字符串。                 | `session_id=...; cf_clearance=...`                |
| `WEBDAV_URL`            | 是   | 您的 WebDAV 服务器的完整 URL。                                    | `https://dav.jianguoyun.com/dav`                  |
| `WEBDAV_USERNAME`       | 是   | 您的 WebDAV 用户名。                                              | `your_email@example.com`                          |
| `WEBDAV_PASSWORD`       | 是   | 您的 WebDAV 密码或应用专用密码。                                  | `your_app_specific_password`                      |
| `NODEIMAGE_API_URL`     | 否   | NodeImage 的 API URL。默认为 `https://api.nodeimage.com/api/images`。 |                                                   |
| `WEBDAV_BASE_PATH`      | 否   | 您希望在 WebDAV 上存储图片的基础目录。默认为 `/images`。          | `/my-photos`                                      |

**重要提示**：请确保您在 WebDAV 服务器上手动创建了 `WEBDAV_BASE_PATH` 所指定的目录，因为本程序不会自动创建它。

### 5. 重新部署

添加完所有环境变量后，请回到项目的部署页面，触发一次新的部署 (Redeploy)。

部署成功后，您就可以访问 Vercel 提供的域名，开始使用您的 Web 同步工具了！