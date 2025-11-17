package main

import (
	"crawler-platform/utlsclient"
	"fmt"
)

func main() {
	// 模拟为1631个IP选择指纹
	fingerprintCounts := make(map[string]int)

	for i := 0; i < 1631; i++ {
		fp := utlsclient.GetRandomFingerprint()
		fingerprintCounts[fp.Name]++
	}

	fmt.Printf("总共测试: 1631次\n")
	fmt.Printf("使用的不同指纹种类: %d\n\n", len(fingerprintCounts))

	fmt.Println("各指纹使用次数:")
	for name, count := range fingerprintCounts {
		fmt.Printf("  %s: %d次 (%.1f%%)\n", name, count, float64(count)*100/1631)
	}
}
