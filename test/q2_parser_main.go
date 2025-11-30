// +build ignore

package main

import (
	"fmt"
	"log"
	"crawler-platform/Store"
	"crawler-platform/GoogleEarth"
)

func main() {
	// 从 SQLite 读取 q2 原始数据
	data, err := Store.GetTileSQLite("./data", "q2", "0")
	if err != nil {
		log.Fatalf("从 SQLite 读取 q2 数据失败: %v", err)
	}
	
	fmt.Printf("原始数据大小: %d 字节\n", len(data))
	
	// 尝试使用不同的参数解析 q2 数据
	q2Response, err := GoogleEarth.NewQ2Parser().Parse(data, "0", true)
	if err != nil {
		log.Printf("使用 rootNode=true 解析 q2 数据失败: %v", err)
		// 尝试另一种方式
		q2Response, err = GoogleEarth.NewQ2Parser().Parse(data, "0", false)
		if err != nil {
			log.Fatalf("使用 rootNode=false 解析 q2 数据也失败: %v", err)
		} else {
			log.Printf("使用 rootNode=false 解析成功")
		}
	}
	
	// 输出解析结果
	fmt.Printf("解析结果:\n")
	fmt.Printf("  Tilekey: %s\n", q2Response.Tilekey)
	fmt.Printf("  Success: %v\n", q2Response.Success)
	if !q2Response.Success {
		fmt.Printf("  Error: %s\n", q2Response.Error)
	}
	fmt.Printf("  ImageryList: %d 项\n", len(q2Response.ImageryList))
	fmt.Printf("  TerrainList: %d 项\n", len(q2Response.TerrainList))
	fmt.Printf("  VectorList: %d 项\n", len(q2Response.VectorList))
	fmt.Printf("  Q2List: %d 项\n", len(q2Response.Q2List))
	
	// 显示前几项数据作为示例
	if len(q2Response.ImageryList) > 0 {
		fmt.Printf("  前3个Imagery项:\n")
		for i, item := range q2Response.ImageryList {
			if i >= 3 {
				break
			}
			fmt.Printf("    %v\n", item)
		}
	}
	
	if len(q2Response.Q2List) > 0 {
		fmt.Printf("  前3个Q2项:\n")
		for i, item := range q2Response.Q2List {
			if i >= 3 {
				break
			}
			fmt.Printf("    %v\n", item)
		}
	}
}