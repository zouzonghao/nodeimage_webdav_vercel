document.addEventListener('DOMContentLoaded', () => {
    // --- 元素选择器 ---
    const incrementalSyncBtn = document.getElementById('incremental-sync-btn');
    const fullSyncBtn = document.getElementById('full-sync-btn');
    const cookieInput = document.getElementById('cookie-input');
    const saveCookieBtn = document.getElementById('save-cookie-btn');
    const syncActionMessage = document.getElementById('sync-action-message');
    const configActionMessage = document.getElementById('config-action-message');
    const logContainer = document.getElementById('log-container');
    const clearLogBtn = document.getElementById('clear-log-btn');

    let ws;

    // --- WebSocket 核心逻辑 ---
    function connectWebSocket() {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        ws = new WebSocket(`${protocol}//${window.location.host}/ws`);

        ws.onopen = () => {
            appendLog('WebSocket 连接成功', 'info');
            checkCookieStatus();
        };

        ws.onmessage = (event) => {
            const message = JSON.parse(event.data);
            switch (message.type) {
                case 'log':
                    const logEntry = document.createElement('p');
                    logEntry.innerHTML = message.content;
                    logContainer.appendChild(logEntry);
                    logContainer.scrollTop = logContainer.scrollHeight;
                    break;
                case 'syncStatus':
                    setSyncing(message.content === 'syncing');
                    break;
                case 'syncResult':
                    // 可以在这里添加一个最终的摘要日志，如果需要的话
                    break;
            }
        };

        ws.onclose = () => {
            appendLog('WebSocket 连接断开，3秒后尝试重连...', 'error');
            setTimeout(connectWebSocket, 3000);
        };

        ws.onerror = (error) => {
            console.error('WebSocket error:', error);
            appendLog('WebSocket 连接错误', 'error');
        };
    }

    // --- UI 控制 ---
    function setSyncing(syncing) {
        incrementalSyncBtn.disabled = syncing;
        fullSyncBtn.disabled = syncing;
        saveCookieBtn.disabled = syncing;
        if (syncing) {
            incrementalSyncBtn.textContent = '增量同步中...';
            fullSyncBtn.textContent = '全量同步中...';
        } else {
            incrementalSyncBtn.textContent = '增量同步 (API Key)';
            fullSyncBtn.textContent = '全量同步 (Cookie)';
        }
    }

    function showActionMessage(element, message, type) {
        element.textContent = message;
        element.className = `action-message msg-${type}`;
        element.style.display = 'block';
        setTimeout(() => {
            element.style.display = 'none';
        }, 3000);
    }

    // --- 日志辅助函数 ---
    function appendLog(message, level) {
        const logEntry = document.createElement('p');
        logEntry.className = `log-${level.toUpperCase()}`;
        logEntry.textContent = `[${new Date().toLocaleTimeString()}] [${level.toUpperCase()}] ${message}`;
        logContainer.appendChild(logEntry);
        logContainer.scrollTop = logContainer.scrollHeight;
    }

    // --- API 调用 ---
    async function triggerSync(isFull = false) {
        let url = '/api/sync';
        if (isFull) {
            url += '?mode=full';
        }
        showActionMessage(syncActionMessage, '同步任务已启动...', 'info');
        try {
            const response = await fetch(url, { method: 'POST' });
            if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`);
        } catch (error) {
            console.error('Failed to trigger sync:', error);
            showActionMessage(syncActionMessage, '启动同步失败', 'error');
        }
    }

    async function checkCookieStatus() {
        try {
            const response = await fetch('/api/config');
            const config = await response.json();
            cookieInput.placeholder = config.isCookieSet 
                ? "后端已配置 Cookie，可在此处覆盖" 
                : "后端未配置 Cookie，请在此处输入";
        } catch (error) {
            console.error('Failed to check cookie status:', error);
        }
    }

    async function saveCookie() {
        const cookie = cookieInput.value.trim();
        if (!cookie) {
            showActionMessage(configActionMessage, 'Cookie 不能为空', 'error');
            return;
        }
        try {
            const response = await fetch('/api/config', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ cookie }),
            });
            if (response.ok) {
                showActionMessage(configActionMessage, 'Cookie 已成功在后端更新', 'success');
            } else {
                throw new Error('Failed to save cookie');
            }
        } catch (error) {
            console.error('Failed to save cookie:', error);
            showActionMessage(configActionMessage, '保存 Cookie 失败', 'error');
        }
    }

    // --- 事件监听 ---
    incrementalSyncBtn.addEventListener('click', () => triggerSync(false));
    fullSyncBtn.addEventListener('click', () => triggerSync(true));
    saveCookieBtn.addEventListener('click', saveCookie);
    clearLogBtn.addEventListener('click', () => {
        logContainer.innerHTML = '';
        appendLog('日志已清除', 'info');
    });

    // --- 初始加载 ---
    connectWebSocket();
});