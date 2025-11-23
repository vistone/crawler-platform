# 爬虫任务管理系统实现计划

## 项目概述

为 crawler-platform 设计并实现一个完整的任务管理系统，支持任务的创建、调度、执行、监控和管理。

## 系统架构

```
crawler-task-manager/
├── task/                    # 任务核心模块
│   ├── task.go             # 任务定义
│   ├── scheduler.go        # 任务调度器
│   ├── executor.go         # 任务执行器
│   ├── queue.go            # 任务队列
│   └── worker_pool.go      # 工作池
├── storage/                 # 存储模块
│   ├── interface.go        # 存储接口
│   ├── memory.go           # 内存存储
│   ├── sqlite.go           # SQLite存储
│   └── models.go           # 数据模型
├── monitor/                 # 监控模块
│   ├── metrics.go          # 指标收集
│   ├── stats.go            # 统计信息
│   └── reporter.go         # 报告生成
├── api/                     # API模块
│   ├── server.go           # HTTP服务器
│   ├── handlers.go         # 请求处理器
│   └── middleware.go       # 中间件
├── web/                     # Web界面
│   ├── index.html          # 主页面
│   ├── index.css           # 样式
│   └── index.js            # 前端逻辑
└── cmd/
    └── task-manager/       # 主程序入口
        └── main.go
```

## 核心功能设计

### 1. 任务定义 (Task)

```go
type Task struct {
    ID          string                 // 任务ID
    Name        string                 // 任务名称
    Type        TaskType               // 任务类型
    Config      TaskConfig             // 任务配置
    Status      TaskStatus             // 任务状态
    Priority    int                    // 优先级 (1-10)
    Schedule    *Schedule              // 调度配置
    Retry       RetryConfig            // 重试配置
    Timeout     time.Duration          // 超时时间
    CreatedAt   time.Time              // 创建时间
    UpdatedAt   time.Time              // 更新时间
    StartedAt   *time.Time             // 开始时间
    FinishedAt  *time.Time             // 完成时间
    Result      *TaskResult            // 执行结果
    Error       string                 // 错误信息
}

type TaskType string
const (
    TaskTypeHTTP        TaskType = "http"         // HTTP请求任务
    TaskTypeGoogleEarth TaskType = "google_earth" // Google Earth数据采集
    TaskTypeCustom      TaskType = "custom"       // 自定义任务
)

type TaskStatus string
const (
    TaskStatusPending   TaskStatus = "pending"    // 等待中
    TaskStatusRunning   TaskStatus = "running"    // 运行中
    TaskStatusCompleted TaskStatus = "completed"  // 已完成
    TaskStatusFailed    TaskStatus = "failed"     // 失败
    TaskStatusCancelled TaskStatus = "cancelled"  // 已取消
    TaskStatusRetrying  TaskStatus = "retrying"   // 重试中
)
```

### 2. 任务调度器 (Scheduler)

**功能**:
- 定时任务调度 (Cron表达式支持)
- 一次性任务调度
- 延迟任务调度
- 任务依赖管理
- 并发控制

**特性**:
- 支持 Cron 表达式 (如: `0 */5 * * * *` 每5分钟执行)
- 支持任务优先级
- 支持任务分组
- 支持任务链 (一个任务完成后触发下一个)

### 3. 任务执行器 (Executor)

**功能**:
- 基于 Worker Pool 模式
- 集成现有的 UTLSHotConnPool
- 支持并发控制
- 自动重试机制
- 超时控制

**工作流程**:
1. 从队列获取任务
2. 分配给空闲 Worker
3. Worker 使用连接池执行任务
4. 收集执行结果
5. 更新任务状态
6. 处理重试逻辑

### 4. 任务队列 (Queue)

**类型**:
- **优先级队列**: 高优先级任务优先执行
- **延迟队列**: 支持延迟执行
- **FIFO队列**: 先进先出

**实现**:
- 使用 Go channels + heap 实现优先级队列
- 支持持久化 (可选)
- 支持分布式 (未来扩展)

### 5. 监控系统 (Monitor)

**指标收集**:
- 任务执行统计 (成功/失败/总数)
- 执行时间统计 (平均/最大/最小)
- 队列长度监控
- Worker 使用率
- 连接池状态
- 错误率统计

**报告生成**:
- 实时统计报告
- 历史趋势分析
- 性能报告

### 6. Web 管理界面

**功能页面**:
1. **仪表板** - 系统概览和实时统计
2. **任务管理** - 创建、编辑、删除、启动、停止任务
3. **任务列表** - 查看所有任务及状态
4. **执行历史** - 查看任务执行记录
5. **监控面板** - 实时监控图表
6. **日志查看** - 查看任务执行日志
7. **配置管理** - 系统配置

**技术栈**:
- 后端: Go + net/http
- 前端: HTML5 + CSS3 + Vanilla JavaScript
- 图表: Chart.js
- 实时更新: Server-Sent Events (SSE) 或 WebSocket

## 数据存储设计

### 数据库表结构

**tasks 表** - 任务配置
```sql
CREATE TABLE tasks (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    config TEXT,
    status TEXT,
    priority INTEGER,
    schedule TEXT,
    retry_config TEXT,
    timeout INTEGER,
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);
```

**task_executions 表** - 任务执行记录
```sql
CREATE TABLE task_executions (
    id TEXT PRIMARY KEY,
    task_id TEXT,
    status TEXT,
    started_at TIMESTAMP,
    finished_at TIMESTAMP,
    duration INTEGER,
    result TEXT,
    error TEXT,
    FOREIGN KEY (task_id) REFERENCES tasks(id)
);
```

**task_logs 表** - 任务日志
```sql
CREATE TABLE task_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id TEXT,
    execution_id TEXT,
    level TEXT,
    message TEXT,
    timestamp TIMESTAMP,
    FOREIGN KEY (task_id) REFERENCES tasks(id)
);
```

## API 设计

### RESTful API 端点

**任务管理**:
- `POST /api/tasks` - 创建任务
- `GET /api/tasks` - 获取任务列表
- `GET /api/tasks/:id` - 获取任务详情
- `PUT /api/tasks/:id` - 更新任务
- `DELETE /api/tasks/:id` - 删除任务
- `POST /api/tasks/:id/start` - 启动任务
- `POST /api/tasks/:id/stop` - 停止任务
- `POST /api/tasks/:id/retry` - 重试任务

**执行记录**:
- `GET /api/executions` - 获取执行记录列表
- `GET /api/executions/:id` - 获取执行记录详情

**监控统计**:
- `GET /api/stats` - 获取统计信息
- `GET /api/metrics` - 获取实时指标
- `GET /api/health` - 健康检查

**日志**:
- `GET /api/logs` - 获取日志列表
- `GET /api/logs/:task_id` - 获取任务日志

## 实现步骤

### Phase 1: 核心功能 (Week 1)
- [x] 任务数据结构定义
- [ ] 任务队列实现
- [ ] Worker Pool 实现
- [ ] 基础任务执行器
- [ ] 内存存储实现

### Phase 2: 调度系统 (Week 2)
- [ ] Cron 调度器实现
- [ ] 延迟任务支持
- [ ] 任务重试机制
- [ ] 超时控制

### Phase 3: 存储与持久化 (Week 3)
- [ ] SQLite 存储实现
- [ ] 任务执行记录
- [ ] 日志存储
- [ ] 数据迁移工具

### Phase 4: API 服务 (Week 4)
- [ ] HTTP 服务器
- [ ] RESTful API 实现
- [ ] 中间件 (认证、日志、CORS)
- [ ] API 文档

### Phase 5: Web 界面 (Week 5-6)
- [ ] 仪表板页面
- [ ] 任务管理页面
- [ ] 监控面板
- [ ] 日志查看器
- [ ] 响应式设计

### Phase 6: 监控与优化 (Week 7)
- [ ] 指标收集系统
- [ ] 实时监控
- [ ] 性能优化
- [ ] 压力测试

### Phase 7: 文档与测试 (Week 8)
- [ ] 单元测试
- [ ] 集成测试
- [ ] 使用文档
- [ ] API 文档
- [ ] 部署指南

## 使用示例

### 创建 HTTP 爬虫任务

```go
task := &Task{
    Name: "爬取Google Earth数据",
    Type: TaskTypeHTTP,
    Config: TaskConfig{
        URL: "https://kh.google.com/rt/earth/PlanetoidMetadata",
        Method: "GET",
        Headers: map[string]string{
            "User-Agent": "Mozilla/5.0...",
        },
        UseConnectionPool: true,
    },
    Priority: 5,
    Schedule: &Schedule{
        Type: ScheduleTypeCron,
        Cron: "0 */10 * * * *", // 每10分钟执行
    },
    Retry: RetryConfig{
        MaxRetries: 3,
        RetryDelay: 5 * time.Second,
    },
    Timeout: 30 * time.Second,
}

// 添加任务
taskManager.AddTask(task)
```

### 启动任务管理器

```go
// 创建配置
config := &ManagerConfig{
    WorkerCount: 10,
    QueueSize: 1000,
    Storage: storage.NewSQLiteStorage("tasks.db"),
    ConnPool: utlsclient.NewUTLSHotConnPool(nil),
}

// 创建任务管理器
manager := task.NewManager(config)

// 启动管理器
manager.Start()

// 启动 Web 服务器
server := api.NewServer(manager, ":8080")
server.Start()
```

## 性能目标

- 支持 **10,000+** 并发任务
- 任务调度延迟 < **100ms**
- API 响应时间 < **50ms**
- Web 界面加载时间 < **1s**
- 系统可用性 > **99.9%**

## 扩展性设计

### 未来扩展方向

1. **分布式支持**
   - 使用 Redis 作为任务队列
   - 支持多节点部署
   - 任务分片和负载均衡

2. **插件系统**
   - 自定义任务类型
   - 自定义执行器
   - 自定义存储后端

3. **高级功能**
   - 任务依赖图 (DAG)
   - 条件触发
   - 动态调度
   - A/B 测试支持

4. **集成能力**
   - Webhook 通知
   - 邮件通知
   - Slack/钉钉集成
   - Prometheus 指标导出

## 安全考虑

- API 认证 (JWT Token)
- 任务配置加密
- 敏感数据脱敏
- 访问控制 (RBAC)
- 审计日志

## 总结

这个任务管理系统将为 crawler-platform 提供完整的任务调度和管理能力，使其从一个爬虫库升级为一个完整的爬虫平台。系统设计遵循模块化、可扩展、高性能的原则，能够满足从小规模到大规模爬虫任务的需求。
