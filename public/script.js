document.addEventListener('DOMContentLoaded', () => {
    const syncButton = document.getElementById('syncButton');
    const logsContainer = document.getElementById('logs');

    syncButton.addEventListener('click', () => {
        // 禁用按钮以防止重复点击
        syncButton.disabled = true;
        syncButton.textContent = '同步中...';

        // 清空旧的日志
        logsContainer.textContent = '';

        // 创建一个新的 EventSource 实例来连接到我们的 SSE 端点
        const eventSource = new EventSource('/api/sync');

        // 处理从服务器接收到的消息
        eventSource.onmessage = (event) => {
            // 将新的日志行附加到日志容器中
            logsContainer.textContent += event.data + '\n';
            // 自动滚动到日志底部
            logsContainer.scrollTop = logsContainer.scrollHeight;
        };

        // 处理错误事件
        eventSource.onerror = (err) => {
            console.error("EventSource failed:", err);
            logsContainer.textContent += '❌ 连接到服务器时发生错误。请检查控制台以获取详细信息。\n';
            logsContainer.scrollTop = logsContainer.scrollHeight;
            // 关闭连接
            eventSource.close();
            // 重新启用按钮
            syncButton.disabled = false;
            syncButton.textContent = '开始同步';
        };

        // 当服务器关闭连接时（例如，同步完成或出错），也会触发 onerror，
        // 但我们可以在这里添加一个自定义事件来更优雅地处理完成状态。
        // 为了简单起见，我们依赖 onerror 来重新启用按钮。
    });
});