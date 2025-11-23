// ===== 全局状态管理 =====
const state = {
    currentPage: 'dashboard',
    tasks: [],
    stats: {
        total: 1247,
        running: 156,
        failed: 23,
        successRate: 98.2
    }
};

// ===== 初始化 =====
document.addEventListener('DOMContentLoaded', () => {
    initNavigation();
    initModal();
    initCharts();
    initTaskTable();
    loadMockData();
    startRealTimeUpdates();
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

function handleCreateTask() {
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
    
    // 模拟创建任务
    const newTask = {
        id: `task-${Date.now()}`,
        name: formData.name,
        type: formData.type,
        status: 'pending',
        priority: formData.priority,
        schedule: formData.schedule,
        lastExecution: new Date().toISOString(),
        successRate: 100
    };
    
    state.tasks.unshift(newTask);
    updateTaskTable();
    
    // 关闭模态框
    document.getElementById('createTaskModal').classList.remove('active');
    document.getElementById('createTaskForm').reset();
    
    // 显示成功消息
    showNotification('任务创建成功！', 'success');
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
function initTaskTable() {
    updateTaskTable();
}

function updateTaskTable() {
    const tbody = document.getElementById('taskTableBody');
    if (!tbody) return;
    
    tbody.innerHTML = state.tasks.map(task => `
        <tr>
            <td>
                <div style="font-weight: 500;">${task.name}</div>
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
            <td>${formatTime(task.lastExecution)}</td>
            <td>
                <div style="display: flex; align-items: center; gap: 8px;">
                    <div class="progress-bar" style="width: 60px; height: 6px;">
                        <div class="progress-fill" style="width: ${task.successRate}%"></div>
                    </div>
                    <span style="font-size: 12px; font-weight: 500;">${task.successRate}%</span>
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

// ===== Chart.js 简化版本（用于演示） =====
// 实际项目中应该引入完整的 Chart.js 库
class Chart {
    constructor(ctx, config) {
        this.ctx = ctx;
        this.config = config;
        this.render();
    }
    
    render() {
        // 这里是简化的渲染逻辑
        // 实际应该使用 Chart.js 库
        const canvas = this.ctx;
        if (!canvas) return;
        
        canvas.style.height = '300px';
        canvas.style.width = '100%';
        
        // 简单的占位符
        const parent = canvas.parentElement;
        parent.style.position = 'relative';
        parent.style.minHeight = '300px';
        parent.style.display = 'flex';
        parent.style.alignItems = 'center';
        parent.style.justifyContent = 'center';
        
        const placeholder = document.createElement('div');
        placeholder.style.cssText = `
            color: #9ca3af;
            font-size: 14px;
            text-align: center;
        `;
        placeholder.innerHTML = `
            <svg width="48" height="48" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg" style="margin-bottom: 8px; opacity: 0.5;">
                <path d="M3 3V16C3 17.1046 3.89543 18 5 18H16M21 21L16 16M16 7V16H7" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
            </svg>
            <div>图表数据加载中...</div>
            <div style="font-size: 12px; margin-top: 4px;">请引入 Chart.js 库以显示完整图表</div>
        `;
        
        canvas.style.display = 'none';
        parent.appendChild(placeholder);
    }
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
