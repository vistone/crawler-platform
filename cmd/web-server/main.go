package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Task 任务结构
type Task struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Type          string    `json:"type"`
	Status        string    `json:"status"`
	Priority      int       `json:"priority"`
	Schedule      string    `json:"schedule"`
	URL           string    `json:"url"`
	Timeout       int       `json:"timeout"`
	CronExpr      string    `json:"cronExpression,omitempty"`
	LastExecution time.Time `json:"lastExecution"`
	SuccessRate   float64   `json:"successRate"`
	CreatedAt     time.Time `json:"createdAt"`
}

// Stats 统计信息
type Stats struct {
	TotalTasks  int     `json:"totalTasks"`
	Running     int     `json:"running"`
	Failed      int     `json:"failed"`
	SuccessRate float64 `json:"successRate"`
}

var (
	tasks []Task
	stats Stats
)

func main() {
	// 初始化模拟数据
	initMockData()

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

	// 启动服务器
	port := ":8080"
	fmt.Printf("\n🚀 爬虫任务管理系统已启动\n")
	fmt.Printf("📊 访问地址: http://localhost%s\n", port)
	fmt.Printf("📡 API地址: http://localhost%s/api\n", port)
	fmt.Printf("⏰ 启动时间: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))

	log.Fatal(http.ListenAndServe(port, nil))
}

// 初始化模拟数据
func initMockData() {
	now := time.Now()
	tasks = []Task{
		{
			ID:            "task-1",
			Name:          "Google Earth 数据采集",
			Type:          "google_earth",
			Status:        "running",
			Priority:      10,
			Schedule:      "cron",
			URL:           "https://kh.google.com/rt/earth/PlanetoidMetadata",
			Timeout:       30,
			CronExpr:      "0 */10 * * * *",
			LastExecution: now.Add(-2 * time.Minute),
			SuccessRate:   99.5,
			CreatedAt:     now.Add(-24 * time.Hour),
		},
		{
			ID:            "task-2",
			Name:          "地形数据解析任务",
			Type:          "custom",
			Status:        "completed",
			Priority:      8,
			Schedule:      "interval",
			URL:           "https://example.com/terrain",
			Timeout:       30,
			LastExecution: now.Add(-5 * time.Minute),
			SuccessRate:   98.2,
			CreatedAt:     now.Add(-48 * time.Hour),
		},
		{
			ID:            "task-3",
			Name:          "四叉树数据爬取",
			Type:          "google_earth",
			Status:        "completed",
			Priority:      7,
			Schedule:      "cron",
			URL:           "https://kh.google.com/rt/earth",
			Timeout:       30,
			CronExpr:      "0 */15 * * * *",
			LastExecution: now.Add(-10 * time.Minute),
			SuccessRate:   97.8,
			CreatedAt:     now.Add(-72 * time.Hour),
		},
		{
			ID:            "task-4",
			Name:          "批量URL采集",
			Type:          "http",
			Status:        "failed",
			Priority:      5,
			Schedule:      "once",
			URL:           "https://example.com/api",
			Timeout:       30,
			LastExecution: now.Add(-15 * time.Minute),
			SuccessRate:   85.3,
			CreatedAt:     now.Add(-96 * time.Hour),
		},
		{
			ID:            "task-5",
			Name:          "定时数据同步",
			Type:          "http",
			Status:        "completed",
			Priority:      6,
			Schedule:      "cron",
			URL:           "https://example.com/sync",
			Timeout:       30,
			CronExpr:      "0 0 */6 * * *",
			LastExecution: now.Add(-20 * time.Minute),
			SuccessRate:   99.1,
			CreatedAt:     now.Add(-120 * time.Hour),
		},
	}

	stats = Stats{
		TotalTasks:  1247,
		Running:     156,
		Failed:      23,
		SuccessRate: 98.2,
	}
}

// 处理任务列表请求
func handleTasks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if r.Method == http.MethodGet {
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
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if r.Method == http.MethodPost {
		var newTask Task
		if err := json.NewDecoder(r.Body).Decode(&newTask); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// 设置默认值
		newTask.ID = fmt.Sprintf("task-%d", time.Now().Unix())
		newTask.Status = "pending"
		newTask.CreatedAt = time.Now()
		newTask.LastExecution = time.Now()
		newTask.SuccessRate = 100.0

		tasks = append([]Task{newTask}, tasks...)

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
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    stats,
	})
}

// 健康检查
func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "healthy",
		"time":    time.Now().Format(time.RFC3339),
		"version": "v0.0.15",
	})
}
