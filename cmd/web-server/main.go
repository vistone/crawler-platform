package main

import (
	"crawler-platform/scheduler"
	"crawler-platform/utlsclient"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	taskQueue   *scheduler.TaskQueue
	worker      *scheduler.Worker
	ipManager   *IPManager
	hotConnPool *utlsclient.UTLSHotConnPool
)

func main() {
	// 1. 初始化 Redis 任务队列
	var err error
	// 假设 Redis 运行在本地默认端口
	taskQueue, err = scheduler.NewTaskQueue("localhost:6379", "", 0)
	if err != nil {
		log.Fatalf("无法连接 Redis: %v", err)
	}
	defer taskQueue.Close()

	// 2. 初始化连接池
	poolConfig := utlsclient.DefaultPoolConfig()
	hotConnPool = utlsclient.NewUTLSHotConnPool(poolConfig)
	// 注意：不在这里关闭热连接池，因为它是全局共享资源，应该保持运行状态

	// 3. 启动 Worker
	worker = scheduler.NewWorker("worker-1", taskQueue, hotConnPool)
	worker.Start()
	defer worker.Stop()

	// 4. 初始化 IP 管理器
	ipManager = NewIPManager()

	// 获取Web目录的绝对路径
	webDir := filepath.Join(".", "web")
	if _, err := os.Stat(webDir); os.IsNotExist(err) {
		log.Fatalf("Web目录不存在: %s", webDir)
	}

	// 静态文件服务
	fs := http.FileServer(http.Dir(webDir))
	http.Handle("/", fs)

	// API路由
	http.HandleFunc("/api/tasks", handleTasks)
	http.HandleFunc("/api/tasks/create", handleCreateTask)
	http.HandleFunc("/api/stats", handleStats)
	http.HandleFunc("/api/health", handleHealth)
	http.HandleFunc("/api/ip/local", handleLocalIPs)
	http.HandleFunc("/api/ip/local/", handleLocalIPDetail)
	http.HandleFunc("/api/ip/whitelist", handleWhitelist)
	http.HandleFunc("/api/ip/blacklist", handleBlacklist)
	http.HandleFunc("/api/ip/settings", handleIPSettings)
	http.HandleFunc("/api/pool/stats", handlePoolStats)

	// 启动服务器
	port := ":8080"
	fmt.Printf("\n🚀 爬虫任务管理系统已启动\n")
	fmt.Printf("📊 访问地址: http://localhost%s\n", port)
	fmt.Printf("📡 API地址: http://localhost%s/api\n", port)
	fmt.Printf("⏰ 启动时间: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))

	log.Fatal(http.ListenAndServe(port, nil))
}

// 处理任务列表请求 (目前仅返回Mock数据，后续需对接Redis/SQLite)
func handleTasks(w http.ResponseWriter, r *http.Request) {
	setJSONHeaders(w)

	if r.Method == http.MethodGet {
		// 从 Redis 获取真实任务列表
		tasks, err := taskQueue.GetTasks()
		if err != nil {
			log.Printf("获取任务列表失败: %v", err)
			// 出错时返回空列表
			tasks = []*scheduler.Task{}
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"data":    tasks,
		})
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// 处理创建任务请求
func handleCreateTask(w http.ResponseWriter, r *http.Request) {
	setJSONHeaders(w)

	if r.Method == http.MethodPost {
		var req struct {
			Name           string `json:"name"`
			Type           string `json:"type"`
			URL            string `json:"url"`
			Priority       int    `json:"priority"`
			Timeout        int    `json:"timeout"`
			Schedule       string `json:"schedule"`
			CronExpression string `json:"cronExpression"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// 构建任务对象
		newTask := &scheduler.Task{
			ID:        fmt.Sprintf("task-%d", time.Now().UnixNano()),
			Name:      req.Name,
			Type:      scheduler.TaskType(req.Type),
			Status:    scheduler.StatusPending,
			Priority:  req.Priority,
			Schedule:  req.Schedule,
			CronExpr:  req.CronExpression,
			Target:    req.URL,
			Timeout:   req.Timeout,
			CreatedAt: time.Now(),
		}

		// 提交到 Redis 队列
		if err := taskQueue.PushTask(newTask); err != nil {
			http.Error(w, fmt.Sprintf("提交任务失败: %v", err), http.StatusInternalServerError)
			return
		}

		log.Printf("收到新任务: %s (%s)", newTask.Name, newTask.ID)

		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "任务创建成功",
			"data":    newTask,
		})
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// 处理统计信息请求
func handleStats(w http.ResponseWriter, r *http.Request) {
	setJSONHeaders(w)

	// TODO: 从 Redis 获取真实统计信息
	stats := map[string]interface{}{
		"totalTasks":  0,
		"running":     0,
		"failed":      0,
		"successRate": 0,
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    stats,
	})
}

// 健康检查
func handleHealth(w http.ResponseWriter, r *http.Request) {
	setJSONHeaders(w)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "healthy",
		"time":    time.Now().Format(time.RFC3339),
		"version": "v0.0.26",
		"worker":  "running",
	})
}

// ===== IP 管理 API =====

func handleLocalIPs(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		setCORSHeaders(w, "GET,POST,OPTIONS")
		return
	}
	setCORSHeaders(w, "GET,POST")
	setJSONHeaders(w)

	switch r.Method {
	case http.MethodGet:
		writeSuccess(w, ipManager.ListLocalIPs())
	case http.MethodPost:
		var req struct {
			Address string `json:"address"`
			Source  string `json:"source"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("解析请求失败: %v", err))
			return
		}
		if strings.TrimSpace(req.Address) == "" {
			writeError(w, http.StatusBadRequest, "address 不能为空")
			return
		}
		entry := ipManager.AddLocalIP(strings.TrimSpace(req.Address), strings.TrimSpace(req.Source))
		writeSuccess(w, entry)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func handleLocalIPDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		setCORSHeaders(w, "DELETE,OPTIONS")
		return
	}
	setCORSHeaders(w, "DELETE")
	setJSONHeaders(w)

	id := strings.TrimPrefix(r.URL.Path, "/api/ip/local/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "缺少 IP ID")
		return
	}

	switch r.Method {
	case http.MethodDelete:
		if ipManager.DeleteLocalIP(id) {
			writeSuccess(w, map[string]bool{"deleted": true})
			return
		}
		writeError(w, http.StatusNotFound, "未找到对应的IP")
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func handleWhitelist(w http.ResponseWriter, r *http.Request) {
	ctrl := hotConnPool.AccessController()
	handleAccessList(w, r, ctrl.GetAllowedIPs, ctrl.AddToWhitelist, ctrl.RemoveFromWhitelist, true)
}

func handleBlacklist(w http.ResponseWriter, r *http.Request) {
	ctrl := hotConnPool.AccessController()
	add := func(ip string) { ctrl.AddIP(ip, false) }
	remove := func(ip string) { ctrl.RemoveFromBlacklist(ip) }
	// reuse handler but convert add signature
	handleAccessList(w, r, ctrl.GetBlockedIPs, add, remove, false)
}

func handleAccessList(
	w http.ResponseWriter,
	r *http.Request,
	getList func() []string,
	addFunc func(string),
	removeFunc func(string),
	isWhite bool,
) {
	if r.Method == http.MethodOptions {
		setCORSHeaders(w, "GET,POST,DELETE,OPTIONS")
		return
	}
	setCORSHeaders(w, "GET,POST,DELETE")
	setJSONHeaders(w)

	switch r.Method {
	case http.MethodGet:
		writeSuccess(w, getList())
	case http.MethodPost:
		var req struct {
			IP string `json:"ip"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("解析请求失败: %v", err))
			return
		}
		ip := strings.TrimSpace(req.IP)
		if ip == "" {
			writeError(w, http.StatusBadRequest, "ip 不能为空")
			return
		}
		addFunc(ip)
		writeSuccess(w, map[string]string{"ip": ip})
	case http.MethodDelete:
		ip := r.URL.Query().Get("ip")
		ip = strings.TrimSpace(ip)
		if ip == "" {
			writeError(w, http.StatusBadRequest, "请通过 ?ip= 指定要删除的 IP")
			return
		}
		removeFunc(ip)
		writeSuccess(w, map[string]string{"ip": ip, "removed": "true"})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func handleIPSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		setCORSHeaders(w, "GET,PUT,OPTIONS")
		return
	}
	setCORSHeaders(w, "GET,PUT")
	setJSONHeaders(w)

	switch r.Method {
	case http.MethodGet:
		writeSuccess(w, ipManager.Settings())
	case http.MethodPut:
		var req IPSettings
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("解析请求失败: %v", err))
			return
		}
		ipManager.UpdateSettings(req)
		writeSuccess(w, req)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func handlePoolStats(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		setCORSHeaders(w, "GET,OPTIONS")
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	setCORSHeaders(w, "GET")
	setJSONHeaders(w)

	stats := hotConnPool.GetStats()
	writeSuccess(w, stats)
}

// ===== 辅助函数 =====

func setCORSHeaders(w http.ResponseWriter, methods string) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", methods)
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

func setJSONHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
}

func writeSuccess(w http.ResponseWriter, data interface{}) {
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    data,
	})
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": false,
		"message": msg,
	})
}
