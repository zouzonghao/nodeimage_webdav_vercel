document.addEventListener('DOMContentLoaded', () => {
    const syncButton = document.getElementById('syncButton');
    const logsContainer = document.getElementById('logs');

    syncButton.addEventListener('click', async () => {
        // 禁用按钮以防止重复点击
        syncButton.disabled = true;
        syncButton.textContent = '同步中...';

        // 清空旧的日志
        logsContainer.textContent = '正在请求服务器，请稍候...\n';

        try {
            const response = await fetch('/api/sync');
            
            // 获取纯文本响应
            const logs = await response.text();

            // 将完整的日志显示在日志容器中
            logsContainer.textContent = logs;
            
            if (!response.ok) {
                // 如果 HTTP 状态码不是 2xx，也在这里显示错误
                logsContainer.textContent += `\n❌ 服务器返回错误: ${response.status} ${response.statusText}\n`;
            }

        } catch (err) {
            console.error("Fetch failed:", err);
            logsContainer.textContent += '❌ 连接到服务器时发生错误。请检查控制台以获取详细信息。\n';
        } finally {
            // 自动滚动到日志底部
            logsContainer.scrollTop = logsContainer.scrollHeight;
            // 重新启用按钮
            syncButton.disabled = false;
            syncButton.textContent = '开始同步';
        }
    });
});