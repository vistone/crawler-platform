// ===== 全局状态管理 =====
const state = {
    currentPage: 'dashboard',
    tasks: [],
    stats: {
        total: 0,
        running: 0,
        failed: 0,
        successRate: 0
    },
    monitorStartTime: Date.now(),
    tasksLoaded: false,
    taskRefreshTimer: null,
    ip: {
        activeTab: 'local',
        local: [],
        whitelist: [],
        blacklist: [],
        settings: null
    },
    poolStats: null,
    poolTimer: null
};

// ===== 初始化 =====
document.addEventListener('DOMContentLoaded', () => {
    initNavigation();
    initModal();
    initCharts();
    initTaskTable();
    initUserMenu();
    initEditUserModal();
    initIPManagement();
    initPoolStats();
    if (typeof window.initMonitor === 'function') {
        window.initMonitor();
    }
    // loadMockData(); // 移除 Mock 数据加载
    // startRealTimeUpdates(); // 暂时移除模拟实时更新
});

// ===== 导航功能 =====
function initNavigation() {
    const navItems = document.querySelectorAll('.nav-item');
    const menuToggle = document.getElementById('menuToggle');
    const sidebar = document.getElementById('sidebar');
    const pageTitle = document.getElementById('pageTitle');

    navItems.forEach(item => {
        item.addEventListener('click', () => {
            const page = item.dataset.page;
            switchPage(page);

            // 更新导航状态
            navItems.forEach(nav => nav.classList.remove('active'));
            item.classList.add('active');

            // 更新页面标题
            const titles = {
                'dashboard': '仪表板',
                'tasks': '任务管理',
                'monitor': '实时监控',
                'logs': '执行日志',
                'settings': '系统设置'
            };
            pageTitle.textContent = titles[page] || '仪表板';

            // 移动端关闭侧边栏
            if (window.innerWidth <= 768) {
                sidebar.classList.remove('active');
            }
        });
    });

    // 菜单切换
    if (menuToggle) {
        menuToggle.addEventListener('click', () => {
            sidebar.classList.toggle('active');
        });
    }
}

function switchPage(page) {
    const pages = document.querySelectorAll('.page-content');
    pages.forEach(p => p.classList.remove('active'));

    const targetPage = document.getElementById(`${page}-page`);
    if (targetPage) {
        targetPage.classList.add('active');
        state.currentPage = page;
    }
}

// ===== 模态框功能 =====
function initModal() {
    const createTaskBtn = document.getElementById('createTaskBtn');
    const modal = document.getElementById('createTaskModal');
    const closeModalBtn = document.getElementById('closeModalBtn');
    const cancelTaskBtn = document.getElementById('cancelTaskBtn');
    const createTaskForm = document.getElementById('createTaskForm');
    const taskScheduleSelect = document.getElementById('taskSchedule');
    const cronExpressionGroup = document.getElementById('cronExpressionGroup');

    // 打开模态框
    if (createTaskBtn) {
        createTaskBtn.addEventListener('click', () => {
            modal.classList.add('active');
        });
    }

    // 关闭模态框
    const closeModal = () => {
        modal.classList.remove('active');
        createTaskForm.reset();
        cronExpressionGroup.style.display = 'none';
    };

    if (closeModalBtn) {
        closeModalBtn.addEventListener('click', closeModal);
    }

    if (cancelTaskBtn) {
        cancelTaskBtn.addEventListener('click', closeModal);
    }

    // 点击遮罩关闭
    modal.querySelector('.modal-overlay').addEventListener('click', closeModal);

    // 调度类型变化
    if (taskScheduleSelect) {
        taskScheduleSelect.addEventListener('change', (e) => {
            if (e.target.value === 'cron') {
                cronExpressionGroup.style.display = 'block';
            } else {
                cronExpressionGroup.style.display = 'none';
            }
        });
    }

    // 表单提交
    if (createTaskForm) {
        createTaskForm.addEventListener('submit', (e) => {
            e.preventDefault();
            handleCreateTask();
        });
    }
}

async function handleCreateTask() {
    const formData = {
        name: document.getElementById('taskName').value,
        type: document.getElementById('taskType').value,
        url: document.getElementById('taskUrl').value,
        priority: parseInt(document.getElementById('taskPriority').value),
        timeout: parseInt(document.getElementById('taskTimeout').value),
        schedule: document.getElementById('taskSchedule').value,
        cronExpression: document.getElementById('cronExpression').value
    };

    console.log('创建任务:', formData);

    try {
        const response = await fetch('/api/tasks/create', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(formData)
        });

        const result = await response.json();

        if (result.success) {
            // 将新任务添加到列表开头
            if (state.tasksLoaded) {
                state.tasks.unshift(normalizeTaskFromAPI(result.data));
                updateTaskTable();
            } else {
                await loadTasksFromServer();
            }

            // 关闭模态框
            document.getElementById('createTaskModal').classList.remove('active');
            document.getElementById('createTaskForm').reset();

            // 显示成功消息
            showNotification('任务创建成功！', 'success');
        } else {
            showNotification('任务创建失败: ' + (result.message || '未知错误'), 'error');
        }
    } catch (error) {
        console.error('创建任务出错:', error);
        showNotification('网络错误，请稍后重试', 'error');
    }
}

async function fetchJSON(url, options = {}) {
    const response = await fetch(url, options);
    let result = null;
    try {
        result = await response.json();
    } catch (err) {
        if (!response.ok) {
            throw new Error(response.statusText || '请求失败');
        }
        return null;
    }

    if (!response.ok || result?.success === false) {
        throw new Error(result?.message || response.statusText || '请求失败');
    }

    return result?.data ?? null;
}

// ===== 图表初始化 =====
function initCharts() {
    // 任务执行趋势图
    const trendCtx = document.getElementById('taskTrendChart');
    if (trendCtx) {
        const trendChart = new Chart(trendCtx, {
            type: 'line',
            data: {
                labels: ['周一', '周二', '周三', '周四', '周五', '周六', '周日'],
                datasets: [
                    {
                        label: '成功',
                        data: [165, 178, 192, 185, 201, 189, 196],
                        borderColor: '#10b981',
                        backgroundColor: 'rgba(16, 185, 129, 0.1)',
                        tension: 0.4,
                        fill: true
                    },
                    {
                        label: '失败',
                        data: [5, 8, 4, 6, 3, 7, 5],
                        borderColor: '#ef4444',
                        backgroundColor: 'rgba(239, 68, 68, 0.1)',
                        tension: 0.4,
                        fill: true
                    }
                ]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: {
                        display: true,
                        position: 'top',
                        labels: {
                            usePointStyle: true,
                            padding: 15
                        }
                    }
                },
                scales: {
                    y: {
                        beginAtZero: true,
                        grid: {
                            color: '#f3f4f6'
                        }
                    },
                    x: {
                        grid: {
                            display: false
                        }
                    }
                }
            }
        });
    }

    // 任务状态分布图
    const statusCtx = document.getElementById('taskStatusChart');
    if (statusCtx) {
        const statusChart = new Chart(statusCtx, {
            type: 'doughnut',
            data: {
                labels: ['已完成', '运行中', '等待中', '失败'],
                datasets: [{
                    data: [1068, 156, 45, 23],
                    backgroundColor: [
                        '#10b981',
                        '#3b82f6',
                        '#f59e0b',
                        '#ef4444'
                    ],
                    borderWidth: 0
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: {
                        position: 'bottom',
                        labels: {
                            usePointStyle: true,
                            padding: 15
                        }
                    }
                },
                cutout: '70%'
            }
        });
    }
}

// ===== 任务表格 =====
async function initTaskTable() {
    await loadTasksFromServer();
    if (!state.taskRefreshTimer) {
        state.taskRefreshTimer = setInterval(loadTasksFromServer, 10000);
    }
}

async function loadTasksFromServer() {
    try {
        const response = await fetch('/api/tasks');
        const result = await response.json();
        if (result.success && Array.isArray(result.data)) {
            state.tasks = result.data.map(normalizeTaskFromAPI);
            state.tasksLoaded = true;
            updateTaskTable();
        } else {
            throw new Error(result.message || '加载任务失败');
        }
    } catch (error) {
        console.error('加载任务列表失败:', error);
        if (!state.tasksLoaded) {
            showTaskEmptyState('无法加载任务数据，请稍后重试');
        } else {
            showNotification('刷新任务列表失败', 'error');
        }
    }
}

function updateTaskTable() {
    const tbody = document.getElementById('taskTableBody');
    if (!tbody) return;

    if (!state.tasks.length) {
        showTaskEmptyState(state.tasksLoaded ? '暂无任务，请先创建任务' : '正在加载任务数据...');
        return;
    }

    tbody.innerHTML = state.tasks.map(task => `
        <tr>
            <td>
                <div style="font-weight: 500;">${task.name}</div>
                <div class="task-meta-sub">${task.target || ''}</div>
            </td>
            <td>
                <span class="task-type">${getTaskTypeLabel(task.type)}</span>
            </td>
            <td>
                <span class="task-status status-${task.status}">${getStatusLabel(task.status)}</span>
            </td>
            <td>
                <div style="display: flex; align-items: center; gap: 4px;">
                    ${getPriorityStars(task.priority)}
                </div>
            </td>
            <td>${getScheduleLabel(task.schedule)}</td>
            <td>${formatTime(task.lastExecution || task.createdAt)}</td>
            <td>
                <div style="display: flex; align-items: center; gap: 8px;">
                    <div class="progress-bar" style="width: 60px; height: 6px;">
                        <div class="progress-fill" style="width: ${Math.min(task.successRate || 0, 100)}%"></div>
                    </div>
                    <span style="font-size: 12px; font-weight: 500;">${(task.successRate || 0).toFixed(1)}%</span>
                </div>
            </td>
            <td>
                <div style="display: flex; gap: 8px;">
                    <button class="action-btn" onclick="handleTaskAction('${task.id}', 'start')" title="启动">
                        <svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                            <path d="M8 5V19L19 12L8 5Z" fill="currentColor"/>
                        </svg>
                    </button>
                    <button class="action-btn" onclick="handleTaskAction('${task.id}', 'stop')" title="停止">
                        <svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                            <rect x="6" y="6" width="12" height="12" fill="currentColor"/>
                        </svg>
                    </button>
                    <button class="action-btn" onclick="handleTaskAction('${task.id}', 'delete')" title="删除">
                        <svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                            <path d="M3 6H5H21M8 6V4C8 3.44772 8.44772 3 9 3H15C15.5523 3 16 3.44772 16 4V6M19 6V20C19 20.5523 18.5523 21 18 21H6C5.44772 21 5 20.5523 5 20V6H19Z" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
                        </svg>
                    </button>
                </div>
            </td>
        </tr>
    `).join('');
}

function showTaskEmptyState(message) {
    const tbody = document.getElementById('taskTableBody');
    if (!tbody) return;
    tbody.innerHTML = `
        <tr>
            <td colspan="8" style="text-align: center; padding: 32px; color: #6b7280;">
                ${message}
            </td>
        </tr>
    `;
}

function normalizeTaskFromAPI(task) {
    return {
        id: task.id || `task-${Date.now()}`,
        name: task.name || '未命名任务',
        type: task.type || 'custom',
        status: task.status || 'pending',
        priority: typeof task.priority === 'number' ? task.priority : 5,
        schedule: task.schedule || 'once',
        lastExecution: task.lastExecution || task.createdAt || new Date().toISOString(),
        createdAt: task.createdAt || new Date().toISOString(),
        successRate: typeof task.successRate === 'number' ? task.successRate : 0,
        target: task.target || '',
        progress: task.progress || 0
    };
}

// ===== 任务操作 =====
function handleTaskAction(taskId, action) {
    const task = state.tasks.find(t => t.id === taskId);
    if (!task) return;

    switch (action) {
        case 'start':
            task.status = 'running';
            showNotification(`任务 "${task.name}" 已启动`, 'success');
            break;
        case 'stop':
            task.status = 'pending';
            showNotification(`任务 "${task.name}" 已停止`, 'info');
            break;
        case 'delete':
            if (confirm(`确定要删除任务 "${task.name}" 吗？`)) {
                state.tasks = state.tasks.filter(t => t.id !== taskId);
                showNotification(`任务 "${task.name}" 已删除`, 'success');
            }
            break;
    }

    updateTaskTable();
}

// ===== 加载模拟数据 =====
function loadMockData() {
    state.tasks = [
        {
            id: 'task-1',
            name: 'Google Earth 数据采集',
            type: 'google_earth',
            status: 'running',
            priority: 10,
            schedule: 'cron',
            lastExecution: new Date(Date.now() - 2 * 60 * 1000).toISOString(),
            successRate: 99.5
        },
        {
            id: 'task-2',
            name: '地形数据解析任务',
            type: 'custom',
            status: 'completed',
            priority: 8,
            schedule: 'interval',
            lastExecution: new Date(Date.now() - 5 * 60 * 1000).toISOString(),
            successRate: 98.2
        },
        {
            id: 'task-3',
            name: '四叉树数据爬取',
            type: 'google_earth',
            status: 'completed',
            priority: 7,
            schedule: 'cron',
            lastExecution: new Date(Date.now() - 10 * 60 * 1000).toISOString(),
            successRate: 97.8
        },
        {
            id: 'task-4',
            name: '批量URL采集',
            type: 'http',
            status: 'failed',
            priority: 5,
            schedule: 'once',
            lastExecution: new Date(Date.now() - 15 * 60 * 1000).toISOString(),
            successRate: 85.3
        },
        {
            id: 'task-5',
            name: '定时数据同步',
            type: 'http',
            status: 'completed',
            priority: 6,
            schedule: 'cron',
            lastExecution: new Date(Date.now() - 20 * 60 * 1000).toISOString(),
            successRate: 99.1
        },
        {
            id: 'task-6',
            name: 'API 数据抓取',
            type: 'http',
            status: 'pending',
            priority: 4,
            schedule: 'interval',
            lastExecution: new Date(Date.now() - 30 * 60 * 1000).toISOString(),
            successRate: 96.5
        },
        {
            id: 'task-7',
            name: '影像数据下载',
            type: 'google_earth',
            status: 'running',
            priority: 9,
            schedule: 'cron',
            lastExecution: new Date(Date.now() - 1 * 60 * 1000).toISOString(),
            successRate: 98.9
        }
    ];

    updateTaskTable();
}

// ===== 实时更新 =====
function startRealTimeUpdates() {
    // 模拟实时数据更新
    setInterval(() => {
        // 随机更新统计数据
        state.stats.running = Math.floor(Math.random() * 50) + 130;
        state.stats.failed = Math.floor(Math.random() * 10) + 20;

        // 更新页面上的统计卡片（如果需要）
        updateStatsCards();
    }, 5000);
}

function updateStatsCards() {
    // 这里可以更新统计卡片的数据
    // 实际项目中应该从API获取最新数据
}

// ===== 辅助函数 =====
function getTaskTypeLabel(type) {
    const labels = {
        'http': 'HTTP',
        'google_earth': 'Google Earth',
        'custom': 'Custom'
    };
    return labels[type] || type;
}

function getStatusLabel(status) {
    const labels = {
        'pending': '等待中',
        'running': '运行中',
        'completed': '已完成',
        'failed': '失败',
        'cancelled': '已取消',
        'retrying': '重试中'
    };
    return labels[status] || status;
}

function getScheduleLabel(schedule) {
    const labels = {
        'once': '一次性',
        'cron': '定时任务',
        'interval': '间隔执行'
    };
    return labels[schedule] || schedule;
}

function getPriorityStars(priority) {
    const stars = Math.ceil(priority / 2);
    const color = priority >= 8 ? '#ef4444' : priority >= 5 ? '#f59e0b' : '#6b7280';
    return '★'.repeat(stars).split('').map(star =>
        `<span style="color: ${color};">${star}</span>`
    ).join('');
}

function formatTime(isoString) {
    const date = new Date(isoString);
    const now = new Date();
    const diff = Math.floor((now - date) / 1000);

    if (diff < 60) return `${diff}秒前`;
    if (diff < 3600) return `${Math.floor(diff / 60)}分钟前`;
    if (diff < 86400) return `${Math.floor(diff / 3600)}小时前`;
    return `${Math.floor(diff / 86400)}天前`;
}

function showNotification(message, type = 'info') {
    // 简单的通知实现
    const notification = document.createElement('div');
    notification.className = `notification notification-${type}`;
    notification.textContent = message;
    notification.style.cssText = `
        position: fixed;
        top: 20px;
        right: 20px;
        padding: 16px 24px;
        background: ${type === 'success' ? '#10b981' : type === 'error' ? '#ef4444' : '#3b82f6'};
        color: white;
        border-radius: 8px;
        box-shadow: 0 10px 15px -3px rgba(0, 0, 0, 0.1);
        z-index: 3000;
        animation: slideIn 0.3s ease;
    `;

    document.body.appendChild(notification);

    setTimeout(() => {
        notification.style.animation = 'slideOut 0.3s ease';
        setTimeout(() => notification.remove(), 300);
    }, 3000);
}

// ===== 添加操作按钮样式 =====
const style = document.createElement('style');
style.textContent = `
    .action-btn {
        width: 32px;
        height: 32px;
        border: none;
        background: transparent;
        cursor: pointer;
        color: var(--text-secondary);
        border-radius: 6px;
        transition: all 0.15s;
        display: flex;
        align-items: center;
        justify-content: center;
    }
    
    .action-btn:hover {
        background: var(--gray-100);
        color: var(--primary-color);
    }
    
    .action-btn svg {
        width: 16px;
        height: 16px;
    }
    
    @keyframes slideIn {
        from {
            transform: translateX(100%);
            opacity: 0;
        }
        to {
            transform: translateX(0);
            opacity: 1;
        }
    }
    
    @keyframes slideOut {
        from {
            transform: translateX(0);
            opacity: 1;
        }
        to {
            transform: translateX(100%);
            opacity: 0;
        }
    }
`;
document.head.appendChild(style);

// ===== IP 管理 =====
function initIPManagement() {
    const tabContainer = document.getElementById('ipTabs');
    if (!tabContainer) return;

    tabContainer.querySelectorAll('.tab-button').forEach((btn) => {
        btn.addEventListener('click', () => {
            switchIPTab(btn.dataset.tab);
        });
    });

    const addLocalForm = document.getElementById('addLocalIPForm');
    if (addLocalForm) {
        addLocalForm.addEventListener('submit', async (e) => {
            e.preventDefault();
            const address = document.getElementById('localIPInput').value.trim();
            const source = document.getElementById('localIPSource').value.trim();
            if (!address) return;
            try {
                await fetchJSON('/api/ip/local', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ address, source })
                });
                document.getElementById('localIPInput').value = '';
                document.getElementById('localIPSource').value = '';
                showNotification('IP 添加成功', 'success');
                refreshLocalIPs();
            } catch (err) {
                showNotification(err.message || '添加失败', 'error');
            }
        });
    }

    const whitelistForm = document.getElementById('addWhitelistForm');
    if (whitelistForm) {
        whitelistForm.addEventListener('submit', async (e) => {
            e.preventDefault();
            const ip = document.getElementById('whitelistInput').value.trim();
            if (!ip) return;
            try {
                await fetchJSON('/api/ip/whitelist', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ ip })
                });
                document.getElementById('whitelistInput').value = '';
                showNotification('已加入白名单', 'success');
                refreshWhitelist();
                refreshPoolStats();
            } catch (err) {
                showNotification(err.message || '操作失败', 'error');
            }
        });
    }

    const blacklistForm = document.getElementById('addBlacklistForm');
    if (blacklistForm) {
        blacklistForm.addEventListener('submit', async (e) => {
            e.preventDefault();
            const ip = document.getElementById('blacklistInput').value.trim();
            if (!ip) return;
            try {
                await fetchJSON('/api/ip/blacklist', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ ip })
                });
                document.getElementById('blacklistInput').value = '';
                showNotification('已加入黑名单', 'success');
                refreshBlacklist();
                refreshPoolStats();
            } catch (err) {
                showNotification(err.message || '操作失败', 'error');
            }
        });
    }

    const ipSettingsForm = document.getElementById('ipSettingsForm');
    if (ipSettingsForm) {
        ipSettingsForm.addEventListener('submit', async (e) => {
            e.preventDefault();
            try {
                const payload = {
                    preheatConnections: parseInt(document.getElementById('preheatConnections').value, 10) || 0,
                    maxFailures: parseInt(document.getElementById('maxFailures').value, 10) || 0,
                    rotateIntervalSeconds: parseInt(document.getElementById('rotateIntervalSeconds').value, 10) || 0,
                    autoRecoverSeconds: parseInt(document.getElementById('autoRecoverSeconds').value, 10) || 0
                };
                await fetchJSON('/api/ip/settings', {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(payload)
                });
                showNotification('策略已保存', 'success');
                refreshIPSettings();
            } catch (err) {
                showNotification(err.message || '保存失败', 'error');
            }
        });
    }

    refreshIPData();
}

async function refreshIPData() {
    await Promise.all([refreshLocalIPs(), refreshWhitelist(), refreshBlacklist(), refreshIPSettings()]);
}

async function refreshLocalIPs() {
    try {
        const data = await fetchJSON('/api/ip/local');
        state.ip.local = Array.isArray(data) ? data : [];
        renderLocalIPTable();
    } catch (err) {
        console.error(err);
        showNotification('加载本地 IP 失败: ' + err.message, 'error');
    }
}

async function refreshWhitelist() {
    try {
        const data = await fetchJSON('/api/ip/whitelist');
        state.ip.whitelist = Array.isArray(data) ? data : [];
        renderAccessList('whitelistContainer', state.ip.whitelist, 'white');
    } catch (err) {
        console.error(err);
        showNotification('加载白名单失败: ' + err.message, 'error');
    }
}

async function refreshBlacklist() {
    try {
        const data = await fetchJSON('/api/ip/blacklist');
        state.ip.blacklist = Array.isArray(data) ? data : [];
        renderAccessList('blacklistContainer', state.ip.blacklist, 'black');
    } catch (err) {
        console.error(err);
        showNotification('加载黑名单失败: ' + err.message, 'error');
    }
}

async function refreshIPSettings() {
    try {
        const data = await fetchJSON('/api/ip/settings');
        state.ip.settings = data;
        if (data) {
            document.getElementById('preheatConnections').value = data.preheatConnections ?? 0;
            document.getElementById('maxFailures').value = data.maxFailures ?? 0;
            document.getElementById('rotateIntervalSeconds').value = data.rotateIntervalSeconds ?? 0;
            document.getElementById('autoRecoverSeconds').value = data.autoRecoverSeconds ?? 0;
        }
    } catch (err) {
        console.error(err);
        showNotification('加载策略失败: ' + err.message, 'error');
    }
}

function renderLocalIPTable() {
    const tbody = document.getElementById('localIPTableBody');
    if (!tbody) return;

    if (!state.ip.local.length) {
        tbody.innerHTML = `<tr><td colspan="5" class="placeholder-cell">暂无数据</td></tr>`;
        return;
    }

    tbody.innerHTML = state.ip.local.map(ip => `
        <tr>
            <td>
                <div style="font-weight:500">${ip.address}</div>
                <div class="task-meta-sub">${ip.source || '-'}</div>
            </td>
            <td><span class="badge">${ip.type || '-'}</span></td>
            <td>${ip.source || '-'}</td>
            <td><span class="task-status status-${ip.status || 'pending'}">${ip.status || 'pending'}</span></td>
            <td>
                <button class="action-btn" title="删除" onclick="removeLocalIP('${ip.id}')">
                    <svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                        <path
                            d="M3 6H5H21M8 6V4C8 3.44772 8.44772 3 9 3H15C15.5523 3 16 3.44772 16 4V6M19 6V20C19 20.5523 18.5523 21 18 21H6C5.44772 21 5 20.5523 5 20V6H19Z"
                            stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" />
                    </svg>
                </button>
            </td>
        </tr>
    `).join('');
}

function renderAccessList(containerId, list, type) {
    const container = document.getElementById(containerId);
    if (!container) return;

    if (!list.length) {
        container.innerHTML = `<div class="placeholder-text">暂无数据</div>`;
        return;
    }

    container.innerHTML = list.map(ip => `
        <span class="ip-tag">
            <span>${ip}</span>
            <button onclick="removeAccessIP('${type}','${ip}')">×</button>
        </span>
    `).join('');
}

async function removeLocalIP(id) {
    if (!confirm('确定删除该 IP 吗？')) return;
    try {
        await fetchJSON(`/api/ip/local/${id}`, { method: 'DELETE' });
        showNotification('已删除 IP', 'success');
        refreshLocalIPs();
    } catch (err) {
        showNotification(err.message || '删除失败', 'error');
    }
}

async function removeAccessIP(type, ip) {
    const endpoint = type === 'white' ? '/api/ip/whitelist' : '/api/ip/blacklist';
    try {
        await fetchJSON(`${endpoint}?ip=${encodeURIComponent(ip)}`, { method: 'DELETE' });
        showNotification('已移除', 'success');
        if (type === 'white') {
            refreshWhitelist();
        } else {
            refreshBlacklist();
        }
        refreshPoolStats();
    } catch (err) {
        showNotification(err.message || '操作失败', 'error');
    }
}

function switchIPTab(tab) {
    state.ip.activeTab = tab;
    document.querySelectorAll('#ipTabs .tab-button').forEach(btn => {
        btn.classList.toggle('active', btn.dataset.tab === tab);
    });
    document.querySelectorAll('.tab-content').forEach(content => {
        content.classList.toggle('active', content.id === `ip-tab-${tab}`);
    });
}

window.removeLocalIP = removeLocalIP;
window.removeAccessIP = removeAccessIP;
window.refreshIPData = refreshIPData;

// ===== 连接池状态 =====
function initPoolStats() {
    if (state.poolTimer) {
        clearInterval(state.poolTimer);
    }
    refreshPoolStats();
    state.poolTimer = setInterval(refreshPoolStats, 5000);
}

async function refreshPoolStats() {
    try {
        const data = await fetchJSON('/api/pool/stats');
        state.poolStats = data;
        updatePoolStatsCard(data);
    } catch (err) {
        console.error(err);
    }
}

function updatePoolStatsCard(stats) {
    if (!stats) return;
    const format = (value, fallback = 0) => (value ?? fallback);
    document.getElementById('poolTotal').textContent = format(stats.TotalConnections);
    document.getElementById('poolActive').textContent = format(stats.ActiveConnections);
    document.getElementById('poolIdle').textContent = format(stats.IdleConnections);
    document.getElementById('poolHealthy').textContent = format(stats.HealthyConnections);
    document.getElementById('poolSuccessRate').textContent = formatPercent(stats.SuccessRate);
    document.getElementById('poolReuseRate').textContent = formatPercent(stats.ConnReuseRate);
    document.getElementById('poolWhitelist').textContent = format(stats.WhitelistIPs);
    document.getElementById('poolBlacklist').textContent = format(stats.BlacklistIPs);
    document.getElementById('poolUpdated').textContent = stats.LastUpdateTime
        ? new Date(stats.LastUpdateTime).toLocaleTimeString()
        : '-';
}

function formatPercent(value) {
    if (value == null || isNaN(value)) return '0%';
    if (value > 1) {
        return `${value.toFixed(1)}%`;
    }
    return `${(value * 100).toFixed(1)}%`;
}

window.refreshPoolStats = refreshPoolStats;

// ===== 用户菜单功能 =====
function initUserMenu() {
    const userMenuBtn = document.getElementById('userMenuBtn');
    const userMenu = document.getElementById('userMenu');
    
    if (userMenuBtn && userMenu) {
        // 点击用户菜单按钮
        userMenuBtn.addEventListener('click', (e) => {
            e.stopPropagation();
            userMenu.classList.toggle('active');
        });
        
        // 点击外部关闭菜单
        document.addEventListener('click', (e) => {
            if (!userMenu.contains(e.target) && !userMenuBtn.contains(e.target)) {
                userMenu.classList.remove('active');
            }
        });
    }
}

// ===== 编辑用户功能（设置页面） =====
function initEditUserModal() {
    const editUserForm = document.getElementById('editUserForm');
    if (editUserForm) {
        editUserForm.addEventListener('submit', (e) => {
            e.preventDefault();
            handleEditUser();
        });
    }
}

function showEditUserModal(username, email, role, status) {
    const modal = document.getElementById('editUserModal');
    if (modal) {
        document.getElementById('editUserId').value = username;
        document.getElementById('editUserUsername').value = username;
        document.getElementById('editUserEmail').value = email;
        document.getElementById('editUserRole').value = role;
        document.getElementById('editUserStatus').checked = status;
        modal.classList.add('active');
    }
}

function closeEditUserModal() {
    const modal = document.getElementById('editUserModal');
    if (modal) {
        modal.classList.remove('active');
    }
}

async function handleEditUser() {
    const formData = {
        id: document.getElementById('editUserId').value,
        username: document.getElementById('editUserUsername').value,
        email: document.getElementById('editUserEmail').value,
        role: document.getElementById('editUserRole').value,
        status: document.getElementById('editUserStatus').checked
    };
    
    console.log('更新用户:', formData);
    
    // 这里可以添加 API 调用保存到服务器
    // try {
    //     const response = await fetch(`/api/users/${formData.id}`, {
    //         method: 'PUT',
    //         headers: { 'Content-Type': 'application/json' },
    //         body: JSON.stringify(formData)
    //     });
    //     const result = await response.json();
    //     if (result.success) {
    //         showNotification('用户更新成功！', 'success');
    //         // 刷新用户列表
    //         loadUserList();
    //     }
    // } catch (error) {
    //     showNotification('更新失败，请稍后重试', 'error');
    // }
    
    showNotification('用户更新成功！', 'success');
    closeEditUserModal();
    
    // 如果更新的是当前用户，同步更新显示
    const currentUser = localStorage.getItem('username') || sessionStorage.getItem('username');
    if (currentUser === formData.id) {
        const userNameElement = document.querySelector('.user-name');
        if (userNameElement) {
            userNameElement.textContent = formData.username;
        }
    }
}
