# 爬虫任务管理系统 - 系统设计文档

## 📋 项目概述

基于现有的 **crawler-platform** 高性能爬虫基础设施，设计并实现了一个完整的**任务管理系统**，提供任务的创建、调度、执行、监控和可视化管理能力。

### 核心价值

- ✅ **可视化管理**: 精美的Web界面，直观管理所有爬虫任务
- ✅ **智能调度**: 支持Cron定时、间隔执行、一次性任务等多种调度方式
- ✅ **实时监控**: 实时查看任务状态、执行统计和系统资源使用情况
- ✅ **高性能**: 基于现有的UTLSHotConnPool，性能提升3-6倍
- ✅ **易扩展**: 模块化设计，支持自定义任务类型和执行器

## 🏗️ 系统架构

### 整体架构图

```
┌─────────────────────────────────────────────────────────────┐
│                      Web 管理界面                            │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐   │
│  │ 仪表板   │  │ 任务管理 │  │ 实时监控 │  │ 执行日志 │   │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘   │
└─────────────────────────────────────────────────────────────┘
                            ↓ HTTP/WebSocket
┌─────────────────────────────────────────────────────────────┐
│                      API 服务层                              │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐   │
│  │ 任务API  │  │ 统计API  │  │ 监控API  │  │ 日志API  │   │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘   │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│                      任务管理核心                            │
│  ┌──────────────────────────────────────────────────────┐  │
│  │              任务调度器 (Scheduler)                   │  │
│  │  • Cron调度  • 延迟队列  • 优先级队列  • 依赖管理   │  │
│  └──────────────────────────────────────────────────────┘  │
│                            ↓                                │
│  ┌──────────────────────────────────────────────────────┐  │
│  │              Worker Pool (工作池)                     │  │
│  │  ┌────────┐  ┌────────┐  ┌────────┐  ┌────────┐    │  │
│  │  │Worker 1│  │Worker 2│  │Worker 3│  │Worker N│    │  │
│  │  └────────┘  └────────┘  └────────┘  └────────┘    │  │
│  └──────────────────────────────────────────────────────┘  │
│                            ↓                                │
│  ┌──────────────────────────────────────────────────────┐  │
│  │              任务执行器 (Executor)                    │  │
│  │  • HTTP任务  • Google Earth任务  • 自定义任务       │  │
│  └──────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│                  UTLSHotConnPool (连接池)                   │
│  • TLS指纹伪装  • 连接复用  • 健康检查  • IP池管理        │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│                      存储层                                  │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐                 │
│  │ 任务配置 │  │ 执行记录 │  │ 系统日志 │                 │
│  │ (SQLite) │  │ (SQLite) │  │ (File)   │                 │
│  └──────────┘  └──────────┘  └──────────┘                 │
└─────────────────────────────────────────────────────────────┘
```

## 📦 模块设计

### 1. Web 管理界面

**技术栈**:
- HTML5 + CSS3 + Vanilla JavaScript
- 响应式设计，支持桌面和移动端
- 现代化UI设计，使用渐变色和微动画

**核心页面**:

#### 1.1 仪表板 (Dashboard)
- **统计卡片**: 总任务数、运行中任务、失败任务、成功率
- **任务执行趋势图**: 7天/30天/90天的执行趋势
- **任务状态分布图**: 饼图展示各状态任务占比
- **最近任务列表**: 最新执行的任务及状态
- **系统资源监控**: Worker使用率、连接池使用率、队列长度、内存使用

#### 1.2 任务管理 (Task Management)
- **任务列表**: 表格展示所有任务
- **创建任务**: 模态框表单创建新任务
- **任务操作**: 启动、停止、删除、编辑任务
- **过滤和搜索**: 按状态、类型筛选任务

#### 1.3 实时监控 (Real-time Monitor)
- **实时指标**: 每秒更新的系统指标
- **性能图表**: CPU、内存、网络使用情况
- **连接池状态**: 实时连接池健康状态

#### 1.4 执行日志 (Execution Logs)
- **日志查看器**: 实时日志流
- **日志过滤**: 按级别、时间、任务ID筛选
- **日志导出**: 导出日志文件

### 2. API 服务层

**RESTful API 设计**:

```
任务管理 API:
  POST   /api/tasks          - 创建任务
  GET    /api/tasks          - 获取任务列表
  GET    /api/tasks/:id      - 获取任务详情
  PUT    /api/tasks/:id      - 更新任务
  DELETE /api/tasks/:id      - 删除任务
  POST   /api/tasks/:id/start   - 启动任务
  POST   /api/tasks/:id/stop    - 停止任务
  POST   /api/tasks/:id/retry   - 重试任务

执行记录 API:
  GET    /api/executions     - 获取执行记录列表
  GET    /api/executions/:id - 获取执行记录详情

监控统计 API:
  GET    /api/stats          - 获取统计信息
  GET    /api/metrics        - 获取实时指标
  GET    /api/health         - 健康检查

日志 API:
  GET    /api/logs           - 获取日志列表
  GET    /api/logs/:task_id  - 获取任务日志
```

### 3. 任务调度器 (Scheduler)

**核心功能**:

```go
type Scheduler struct {
    cronScheduler  *cron.Cron           // Cron调度器
    delayQueue     *DelayQueue          // 延迟队列
    priorityQueue  *PriorityQueue       // 优先级队列
    taskManager    *TaskManager         // 任务管理器
    workerPool     *WorkerPool          // 工作池
}

// 调度类型
type ScheduleType string
const (
    ScheduleTypeOnce     ScheduleType = "once"      // 一次性
    ScheduleTypeCron     ScheduleType = "cron"      // Cron定时
    ScheduleTypeInterval ScheduleType = "interval"  // 间隔执行
    ScheduleTypeDelay    ScheduleType = "delay"     // 延迟执行
)

// 调度配置
type Schedule struct {
    Type       ScheduleType  `json:"type"`
    CronExpr   string        `json:"cronExpr,omitempty"`   // Cron表达式
    Interval   time.Duration `json:"interval,omitempty"`   // 间隔时间
    Delay      time.Duration `json:"delay,omitempty"`      // 延迟时间
    StartTime  *time.Time    `json:"startTime,omitempty"`  // 开始时间
    EndTime    *time.Time    `json:"endTime,omitempty"`    // 结束时间
}
```

**调度策略**:

1. **Cron调度**: 使用 `robfig/cron` 库实现
   - 支持标准Cron表达式: `0 */5 * * * *` (每5分钟)
   - 支持秒级精度
   - 支持时区设置

2. **优先级调度**: 使用堆实现优先级队列
   - 优先级范围: 1-10 (10最高)
   - 同优先级按FIFO顺序

3. **延迟调度**: 使用时间轮算法
   - 支持任意延迟时间
   - 高效的时间复杂度 O(1)

### 4. Worker Pool (工作池)

**设计模式**: 生产者-消费者模式

```go
type WorkerPool struct {
    workers      []*Worker
    taskQueue    chan *Task
    workerCount  int
    executor     Executor
    connPool     *utlsclient.UTLSHotConnPool
    mu           sync.RWMutex
    stats        WorkerPoolStats
}

type Worker struct {
    id        int
    pool      *WorkerPool
    taskChan  chan *Task
    quitChan  chan bool
    status    WorkerStatus
}

type WorkerStatus string
const (
    WorkerStatusIdle    WorkerStatus = "idle"     // 空闲
    WorkerStatusBusy    WorkerStatus = "busy"     // 忙碌
    WorkerStatusStopped WorkerStatus = "stopped"  // 已停止
)
```

**工作流程**:

```
1. 初始化 N 个 Worker
2. Worker 监听任务队列
3. 收到任务后:
   a. 标记为 Busy
   b. 从连接池获取连接
   c. 执行任务
   d. 收集结果
   e. 归还连接
   f. 标记为 Idle
4. 继续监听下一个任务
```

### 5. 任务执行器 (Executor)

**执行器接口**:

```go
type Executor interface {
    Execute(ctx context.Context, task *Task) (*TaskResult, error)
    Validate(task *Task) error
    GetType() TaskType
}
```

**内置执行器**:

#### 5.1 HTTP 执行器
```go
type HTTPExecutor struct {
    connPool *utlsclient.UTLSHotConnPool
}

func (e *HTTPExecutor) Execute(ctx context.Context, task *Task) (*TaskResult, error) {
    // 1. 从连接池获取连接
    conn, err := e.connPool.GetConnection(task.Config.URL)
    if err != nil {
        return nil, err
    }
    defer e.connPool.PutConnection(conn)
    
    // 2. 创建HTTP客户端
    client := utlsclient.NewUTLSClient(conn)
    
    // 3. 构建请求
    req, err := http.NewRequestWithContext(ctx, task.Config.Method, task.Config.URL, nil)
    if err != nil {
        return nil, err
    }
    
    // 4. 执行请求
    resp, err := client.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    // 5. 处理响应
    body, _ := io.ReadAll(resp.Body)
    
    return &TaskResult{
        StatusCode: resp.StatusCode,
        Body:       body,
        Headers:    resp.Header,
    }, nil
}
```

#### 5.2 Google Earth 执行器
```go
type GoogleEarthExecutor struct {
    connPool *utlsclient.UTLSHotConnPool
}

func (e *GoogleEarthExecutor) Execute(ctx context.Context, task *Task) (*TaskResult, error) {
    // 1. 解析四叉树路径
    quadtreePath := task.Config.QuadtreePath
    
    // 2. 获取连接并请求数据
    conn, _ := e.connPool.GetConnection("kh.google.com")
    defer e.connPool.PutConnection(conn)
    
    // 3. 下载地形/影像数据
    data, err := e.downloadTileData(ctx, conn, quadtreePath)
    if err != nil {
        return nil, err
    }
    
    // 4. 解析Protobuf数据
    parsed, err := e.parseProtobuf(data)
    if err != nil {
        return nil, err
    }
    
    return &TaskResult{
        Data: parsed,
    }, nil
}
```

#### 5.3 自定义执行器
```go
type CustomExecutor struct {
    handler func(context.Context, *Task) (*TaskResult, error)
}

func (e *CustomExecutor) Execute(ctx context.Context, task *Task) (*TaskResult, error) {
    return e.handler(ctx, task)
}
```

### 6. 数据存储

**数据库表设计**:

```sql
-- 任务配置表
CREATE TABLE tasks (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    type          TEXT NOT NULL,
    config        TEXT,           -- JSON格式的任务配置
    status        TEXT,
    priority      INTEGER,
    schedule_type TEXT,
    schedule_config TEXT,         -- JSON格式的调度配置
    retry_config  TEXT,           -- JSON格式的重试配置
    timeout       INTEGER,
    created_at    TIMESTAMP,
    updated_at    TIMESTAMP,
    created_by    TEXT,
    INDEX idx_status (status),
    INDEX idx_type (type),
    INDEX idx_created_at (created_at)
);

-- 任务执行记录表
CREATE TABLE task_executions (
    id            TEXT PRIMARY KEY,
    task_id       TEXT NOT NULL,
    status        TEXT,
    started_at    TIMESTAMP,
    finished_at   TIMESTAMP,
    duration      INTEGER,        -- 执行时长(毫秒)
    result        TEXT,           -- JSON格式的执行结果
    error         TEXT,
    retry_count   INTEGER,
    worker_id     INTEGER,
    FOREIGN KEY (task_id) REFERENCES tasks(id),
    INDEX idx_task_id (task_id),
    INDEX idx_started_at (started_at)
);

-- 任务日志表
CREATE TABLE task_logs (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id       TEXT,
    execution_id  TEXT,
    level         TEXT,           -- DEBUG, INFO, WARN, ERROR
    message       TEXT,
    timestamp     TIMESTAMP,
    metadata      TEXT,           -- JSON格式的额外信息
    FOREIGN KEY (task_id) REFERENCES tasks(id),
    INDEX idx_task_id (task_id),
    INDEX idx_execution_id (execution_id),
    INDEX idx_timestamp (timestamp)
);

-- 系统指标表
CREATE TABLE system_metrics (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    metric_type   TEXT,           -- worker_usage, queue_length, etc.
    metric_value  REAL,
    timestamp     TIMESTAMP,
    INDEX idx_metric_type (metric_type),
    INDEX idx_timestamp (timestamp)
);
```

## 🔄 核心流程

### 任务创建流程

```
1. 用户在Web界面填写任务表单
   ↓
2. 前端验证表单数据
   ↓
3. 发送 POST /api/tasks 请求
   ↓
4. API层验证请求数据
   ↓
5. 创建Task对象
   ↓
6. 保存到数据库
   ↓
7. 根据调度类型添加到调度器
   - Cron: 添加到Cron调度器
   - Interval: 添加到间隔调度器
   - Once: 添加到任务队列
   - Delay: 添加到延迟队列
   ↓
8. 返回任务ID给前端
   ↓
9. 前端更新任务列表
```

### 任务执行流程

```
1. 调度器触发任务
   ↓
2. 任务进入优先级队列
   ↓
3. Worker从队列获取任务
   ↓
4. Worker标记为Busy
   ↓
5. 根据任务类型选择执行器
   ↓
6. 执行器从连接池获取连接
   ↓
7. 执行任务逻辑
   - HTTP: 发送HTTP请求
   - Google Earth: 下载并解析数据
   - Custom: 执行自定义逻辑
   ↓
8. 收集执行结果
   ↓
9. 归还连接到连接池
   ↓
10. 保存执行记录到数据库
    ↓
11. 更新任务统计信息
    ↓
12. 如果失败且有重试配置:
    - 增加重试计数
    - 延迟后重新入队
    ↓
13. Worker标记为Idle
    ↓
14. 继续处理下一个任务
```

### 监控数据流

```
1. Worker执行任务时更新指标
   ↓
2. 指标收集器定期收集:
   - Worker状态统计
   - 队列长度
   - 连接池状态
   - 系统资源使用
   ↓
3. 保存到时序数据库
   ↓
4. Web界面通过WebSocket/SSE订阅
   ↓
5. 实时推送到前端
   ↓
6. 前端更新图表和统计卡片
```

## 🎯 性能优化

### 1. 连接池优化
- 使用现有的UTLSHotConnPool
- 预热连接，减少TLS握手时间
- 连接复用，性能提升3-6倍

### 2. 任务队列优化
- 使用优先级队列，高优先级任务优先执行
- 批量入队，减少锁竞争
- 队列容量限制，防止内存溢出

### 3. Worker Pool优化
- 动态调整Worker数量
- Worker本地缓存，减少锁竞争
- 任务窃取算法，负载均衡

### 4. 数据库优化
- 索引优化，加速查询
- 批量写入，减少IO
- 定期清理历史数据

### 5. 前端优化
- 虚拟滚动，处理大量数据
- 防抖和节流，减少请求
- 懒加载，按需加载资源

## 🔒 安全设计

### 1. 认证授权
- JWT Token认证
- RBAC权限控制
- API密钥管理

### 2. 数据安全
- 敏感数据加密存储
- 传输层TLS加密
- SQL注入防护

### 3. 访问控制
- IP白名单
- 请求频率限制
- CORS配置

### 4. 审计日志
- 操作日志记录
- 登录日志
- 异常行为监控

## 📊 监控指标

### 系统指标
- Worker使用率
- 队列长度
- 连接池使用率
- 内存使用
- CPU使用率

### 业务指标
- 任务总数
- 运行中任务数
- 成功任务数
- 失败任务数
- 平均执行时间
- 成功率

### 性能指标
- API响应时间
- 任务调度延迟
- 数据库查询时间
- 连接池获取时间

## 🚀 部署方案

### 单机部署
```bash
# 1. 编译
go build -o task-manager cmd/task-manager/main.go

# 2. 运行
./task-manager --config config.yaml

# 3. 访问
http://localhost:8080
```

### Docker部署
```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o task-manager cmd/task-manager/main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/task-manager .
COPY --from=builder /app/web ./web
EXPOSE 8080
CMD ["./task-manager"]
```

### 分布式部署
- 使用Redis作为任务队列
- 多节点部署Worker
- 负载均衡器分发请求
- 共享数据库存储

## 📈 扩展性设计

### 1. 插件系统
```go
type Plugin interface {
    Name() string
    Version() string
    Init(config map[string]interface{}) error
    Execute(ctx context.Context, params map[string]interface{}) (interface{}, error)
}

// 注册插件
pluginManager.Register("my-plugin", &MyPlugin{})

// 使用插件
result, err := pluginManager.Execute("my-plugin", ctx, params)
```

### 2. 自定义执行器
```go
// 实现Executor接口
type MyExecutor struct {}

func (e *MyExecutor) Execute(ctx context.Context, task *Task) (*TaskResult, error) {
    // 自定义执行逻辑
    return &TaskResult{}, nil
}

// 注册执行器
executorRegistry.Register("my-executor", &MyExecutor{})
```

### 3. 存储后端扩展
```go
type Storage interface {
    SaveTask(task *Task) error
    GetTask(id string) (*Task, error)
    ListTasks(filter TaskFilter) ([]*Task, error)
    DeleteTask(id string) error
}

// 支持多种存储后端
- SQLite (默认)
- MySQL
- PostgreSQL
- MongoDB
```

## 🎓 使用示例

### 创建HTTP任务
```go
task := &Task{
    Name: "爬取API数据",
    Type: TaskTypeHTTP,
    Config: TaskConfig{
        URL:    "https://api.example.com/data",
        Method: "GET",
        Headers: map[string]string{
            "Authorization": "Bearer token",
        },
    },
    Priority: 5,
    Schedule: &Schedule{
        Type:     ScheduleTypeCron,
        CronExpr: "0 */10 * * * *", // 每10分钟
    },
    Retry: RetryConfig{
        MaxRetries: 3,
        RetryDelay: 5 * time.Second,
    },
    Timeout: 30 * time.Second,
}

taskManager.CreateTask(task)
```

### 创建Google Earth任务
```go
task := &Task{
    Name: "下载地形数据",
    Type: TaskTypeGoogleEarth,
    Config: TaskConfig{
        QuadtreePath: "0123",
        DataType:     "terrain",
    },
    Priority: 8,
    Schedule: &Schedule{
        Type:     ScheduleTypeInterval,
        Interval: 1 * time.Hour,
    },
}

taskManager.CreateTask(task)
```

## 📝 总结

这个爬虫任务管理系统将 crawler-platform 从一个爬虫库升级为一个完整的爬虫平台，提供了：

✅ **完整的任务生命周期管理**
✅ **灵活的调度策略**
✅ **高性能的执行引擎**
✅ **精美的可视化界面**
✅ **实时监控和统计**
✅ **良好的扩展性**

系统设计遵循模块化、高性能、易扩展的原则，能够满足从小规模到大规模爬虫任务的需求。
