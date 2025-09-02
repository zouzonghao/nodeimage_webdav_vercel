document.addEventListener('DOMContentLoaded', () => {
    // --- DOM Elements ---
    const configInputs = {
        nodeimageUrl: document.getElementById('nodeimageUrl'),
        webdavUrl: document.getElementById('webdavUrl'),
        webdavUser: document.getElementById('webdavUser'),
        webdavPass: document.getElementById('webdavPass'),
        webdavPath: document.getElementById('webdavPath'),
        corsProxy: document.getElementById('corsProxy'),
    };
    const saveButton = document.getElementById('saveButton');
    const clearButton = document.getElementById('clearButton');
    const syncButton = document.getElementById('syncButton');
    const logsContainer = document.getElementById('logs');

    // --- Logger ---
    const log = (message, level = 'INFO') => {
        const timestamp = new Date().toISOString();
        const line = `[${timestamp}] [${level}] ${message}\n`;
        logsContainer.textContent += line;
        logsContainer.scrollTop = logsContainer.scrollHeight;
        console.log(message);
    };

    // --- Config Management ---
    const saveConfig = () => {
        try {
            const config = {};
            for (const key in configInputs) {
                config[key] = configInputs[key].value;
            }
            localStorage.setItem('syncConfig', JSON.stringify(config));
            log('配置已成功保存到浏览器本地存储。');
        } catch (error) {
            log(`保存配置失败: ${error}`, 'ERROR');
        }
    };

    const loadConfig = () => {
        try {
            const savedConfig = localStorage.getItem('syncConfig');
            if (savedConfig) {
                const config = JSON.parse(savedConfig);
                for (const key in config) {
                    if (configInputs[key]) {
                        configInputs[key].value = config[key];
                    }
                }
                log('已从浏览器本地存储加载配置。');
            }
        } catch (error) {
            log(`加载配置失败: ${error}`, 'ERROR');
        }
    };

    const clearConfig = () => {
        if (confirm('确定要清除所有配置信息吗？')) {
            for (const key in configInputs) {
                configInputs[key].value = '';
            }
            localStorage.removeItem('syncConfig');
            log('配置已清除。');
        }
    };

    // --- Core Sync Logic ---
    const startSync = async () => {
        syncButton.disabled = true;
        syncButton.textContent = '同步中...';
        logsContainer.textContent = ''; // Clear previous logs

        try {
            // 1. Get and validate config
            const config = {};
            for (const key in configInputs) {
                config[key] = configInputs[key].value;
            }
            if (!config.nodeimageUrl || !config.webdavUrl || !config.webdavUser || !config.webdavPass || !config.webdavPath || !config.corsProxy) {
                throw new Error('配置不完整，请填写所有必填项。');
            }
            
            const corsProxy = config.corsProxy.trim();
            const nodeimageUrl = corsProxy + config.nodeimageUrl;
            const webdavBaseUrl = new URL(config.webdavUrl).origin;
            const authHeader = `Basic ${btoa(`${config.webdavUser}:${config.webdavPass}`)}`;
            const headers = { authorization: authHeader };

            // 2. Get file lists
            log('🔍 正在扫描 NodeImage 图片...');
            const nodeImageFiles = await getNodeImageList(nodeimageUrl);
            log(`从 NodeImage 获取到 ${nodeImageFiles.length} 个文件。`);

            log('📁 正在扫描 WebDAV 目录...');
            const webdavFiles = await getWebdavFileList(webdavBaseUrl, config.webdavPath, headers);
            log(`从 WebDAV 获取到 ${webdavFiles.length} 个文件。`);

            // 3. Diff files
            const { toUpload, toDelete } = diffFiles(nodeImageFiles, webdavFiles, config.webdavPath);
            log(`🔄 同步计划: 需要上传 ${toUpload.length} 个文件, 需要删除 ${toDelete.length} 个文件。`);

            if (toUpload.length === 0 && toDelete.length === 0) {
                log('✅ 文件已是最新状态，无需同步。');
                return;
            }

            // 4. Execute sync
            for (const file of toUpload) {
                const targetUrl = `${webdavBaseUrl}${config.webdavPath}/${file.Filename}`;
                try {
                    log(`  - 正在下载: ${file.Filename}`);
                    const fileData = await downloadImage(corsProxy + file.URL);
                    log(`  - 正在上传: ${file.Filename} 到 ${targetUrl}`);
                    await tsdav.createObject({ url: targetUrl, data: fileData, headers });
                    log(`  ✔ 上传成功: ${file.Filename}`);
                } catch (error) {
                    log(`  ❌ 上传失败: ${file.Filename} - ${error}`, 'ERROR');
                }
            }

            for (const filePath of toDelete) {
                const targetUrl = `${webdavBaseUrl}${filePath}`;
                try {
                    log(`  - 正在删除: ${targetUrl}`);
                    await tsdav.deleteObject({ url: targetUrl, headers });
                    log(`  ✔ 删除成功: ${filePath}`);
                } catch (error) {
                    log(`  ❌ 删除失败: ${filePath} - ${error}`, 'ERROR');
                }
            }

            log('✅ 同步完成！');

        } catch (error) {
            log(`同步过程中发生严重错误: ${error}`, 'ERROR');
        } finally {
            syncButton.disabled = false;
            syncButton.textContent = '开始同步';
        }
    };

    // --- Helper Functions ---
    const getNodeImageList = async (apiUrl) => {
        let allImages = [];
        let page = 1;
        const limit = 100;

        while (true) {
            const url = `${apiUrl}?page=${page}&limit=${limit}`;
            log(`  - 正在获取 NodeImage 第 ${page} 页...`);
            const response = await fetch(url, {
                credentials: 'include',
                headers: { 'X-Requested-With': 'XMLHttpRequest' }
            });
            if (!response.ok) {
                throw new Error(`获取 NodeImage 列表失败: API 返回 ${response.status}。`);
            }
            const data = await response.json();
            allImages = allImages.concat(data.images.map(img => ({...img, Filename: new URL(img.url).pathname.split('/').pop() })));
            
            if (!data.pagination.hasNextPage) {
                break;
            }
            page++;
        }
        return allImages;
    };

    const getWebdavFileList = async (baseUrl, path, headers) => {
        const url = `${baseUrl}${path}`;
        try {
            const response = await tsdav.propfind({ url, headers, depth: '1' });
            const files = response.multistatus
                .filter(item => item.href !== path && item.href !== `${path}/`) // Exclude the directory itself
                .map(item => item.href);
            return files;
        } catch (error) {
            if (error.status === 404) {
                log(`WebDAV 目录 ${url} 不存在，将尝试创建...`, 'WARN');
                await tsdav.createCollection({ url, headers });
                log(`目录 ${url} 创建成功。`);
                return [];
            }
            throw error;
        }
    };

    const diffFiles = (nodeImageFiles, webdavFiles, webdavPath) => {
        const webdavFileSet = new Set(webdavFiles.map(f => f.substring(f.lastIndexOf('/') + 1)));
        const nodeImageFileMap = new Map(nodeImageFiles.map(f => [f.Filename, f]));

        const toUpload = nodeImageFiles.filter(f => !webdavFileSet.has(f.Filename));
        
        const toDelete = webdavFiles.filter(f => !nodeImageFileMap.has(f.substring(f.lastIndexOf('/') + 1)));

        return { toUpload, toDelete };
    };

    const downloadImage = async (url) => {
        const response = await fetch(url, {
            credentials: 'include',
            headers: { 'X-Requested-With': 'XMLHttpRequest' }
        });
        if (!response.ok) {
            throw new Error(`下载图片失败: 服务器返回 ${response.status}`);
        }
        return response.blob();
    };

    // --- Event Listeners ---
    saveButton.addEventListener('click', saveConfig);
    clearButton.addEventListener('click', clearConfig);
    syncButton.addEventListener('click', startSync);

    loadConfig();
});