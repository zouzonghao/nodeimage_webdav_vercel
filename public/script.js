document.addEventListener('DOMContentLoaded', () => {
    // --- DOM Elements ---
    const configInputs = {
        githubOwner: document.getElementById('githubOwner'),
        githubRepo: document.getElementById('githubRepo'),
        githubPat: document.getElementById('githubPat'),
        githubBranch: document.getElementById('githubBranch'),
    };
    const saveButton = document.getElementById('saveButton');
    const clearButton = document.getElementById('clearButton');
    const syncButton = document.getElementById('syncButton');
    const statusContainer = document.getElementById('status');

    // --- Logger ---
    const updateStatus = (message, isError = false, isHtml = false) => {
        const timestamp = new Date().toISOString();
        const prefix = `[${timestamp}] `;
        
        if (isHtml) {
            statusContainer.innerHTML = `${prefix}${message}`;
        } else {
            statusContainer.textContent = `${prefix}${message}\n`;
        }

        if (isError) {
            statusContainer.style.color = 'red';
        } else {
            statusContainer.style.color = 'green';
        }
        console.log(message);
    };

    // --- Config Management ---
    const saveConfig = () => {
        try {
            const config = {};
            for (const key in configInputs) {
                config[key] = configInputs[key].value;
            }
            localStorage.setItem('githubConfig', JSON.stringify(config));
            updateStatus('配置已保存到浏览器本地存储。');
        } catch (error) {
            updateStatus(`保存配置失败: ${error}`, true);
        }
    };

    const loadConfig = () => {
        try {
            const savedConfig = localStorage.getItem('githubConfig');
            if (savedConfig) {
                const config = JSON.parse(savedConfig);
                for (const key in config) {
                    if (configInputs[key]) {
                        configInputs[key].value = config[key];
                    }
                }
                updateStatus('已从浏览器本地存储加载配置。');
            }
        } catch (error) {
            updateStatus(`加载配置失败: ${error}`, true);
        }
    };

    const clearConfig = () => {
        if (confirm('确定要清除所有已保存的 GitHub 配置吗？')) {
            for (const key in configInputs) {
                configInputs[key].value = '';
            }
            localStorage.removeItem('githubConfig');
            updateStatus('已清除本地存储的配置。');
        }
    };

    // --- Core Sync Logic ---
    const triggerSync = async () => {
        syncButton.disabled = true;
        syncButton.textContent = '触发中...';
        statusContainer.style.color = 'inherit';
        updateStatus('正在向 GitHub API 发送请求...');

        try {
            const { githubOwner, githubRepo, githubPat, githubBranch } = configInputs;
            if (!githubOwner.value || !githubRepo.value || !githubPat.value || !githubBranch.value) {
                throw new Error('配置不完整，请填写所有 GitHub 相关信息。');
            }

            const workflowFileName = 'sync.yml';
            const url = `https://api.github.com/repos/${githubOwner.value}/${githubRepo.value}/actions/workflows/${workflowFileName}/dispatches`;

            const response = await fetch(url, {
                method: 'POST',
                headers: {
                    'Accept': 'application/vnd.github.v3+json',
                    'Authorization': `token ${githubPat.value}`,
                },
                body: JSON.stringify({
                    ref: githubBranch.value,
                }),
            });

            if (response.status === 204) {
                const repoUrl = `https://github.com/${githubOwner.value}/${githubRepo.value}/actions`;
                const successMessage = `成功！同步工作流已触发。<br>请前往 <a href="${repoUrl}" target="_blank">GitHub Actions 页面</a> 查看日志。`;
                updateStatus(successMessage, false, true);
            } else {
                const errorText = await response.text();
                throw new Error(`GitHub API 返回错误 ${response.status}: ${errorText}`);
            }

        } catch (error) {
            updateStatus(`触发失败: ${error}`, true);
        } finally {
            syncButton.disabled = false;
            syncButton.textContent = '触发同步';
        }
    };

    // --- Event Listeners ---
    saveButton.addEventListener('click', saveConfig);
    clearButton.addEventListener('click', clearConfig);
    syncButton.addEventListener('click', triggerSync);

    loadConfig();
});