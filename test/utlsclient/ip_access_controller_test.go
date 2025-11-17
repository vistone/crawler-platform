package utlsclient_test

import (
	"fmt"
	"testing"

	"crawler-platform/utlsclient"
)

func TestNewIPAccessController(t *testing.T) {
	ctrl := utlsclient.NewIPAccessController()
	if ctrl == nil {
		t.Fatal("NewIPAccessController should not return nil")
	}
}

func TestIPAccessControllerAddIP(t *testing.T) {
	ctrl := utlsclient.NewIPAccessController()

	// 添加到白名单
	ctrl.AddIP("1.2.3.4", true)
	if !ctrl.IsIPAllowed("1.2.3.4") {
		t.Error("IP should be allowed after adding to whitelist")
	}

	// 添加到黑名单
	ctrl.AddIP("1.2.3.5", false)
	if ctrl.IsIPAllowed("1.2.3.5") {
		t.Error("IP should not be allowed after adding to blacklist")
	}
}

func TestIPAccessControllerIsIPAllowed(t *testing.T) {
	ctrl := utlsclient.NewIPAccessController()

	// 默认情况下，IP应该被允许（实现中默认允许）
	if !ctrl.IsIPAllowed("1.2.3.4") {
		t.Error("IP should be allowed by default (implementation allows by default)")
	}

	// 添加到白名单后应该允许
	ctrl.AddIP("1.2.3.4", true)
	if !ctrl.IsIPAllowed("1.2.3.4") {
		t.Error("IP should be allowed after adding to whitelist")
	}

	// 添加到黑名单后应该拒绝（即使也在白名单中）
	ctrl.AddIP("1.2.3.4", false)
	if ctrl.IsIPAllowed("1.2.3.4") {
		t.Error("IP should be denied if in blacklist, even if also in whitelist")
	}
}

func TestIPAccessControllerGetAllowedIPs(t *testing.T) {
	ctrl := utlsclient.NewIPAccessController()

	// 初始应该为空
	allowed := ctrl.GetAllowedIPs()
	if len(allowed) != 0 {
		t.Error("Should have no allowed IPs initially")
	}

	// 添加白名单IP
	ctrl.AddIP("1.2.3.4", true)
	ctrl.AddIP("1.2.3.5", true)

	allowed = ctrl.GetAllowedIPs()
	if len(allowed) != 2 {
		t.Errorf("Expected 2 allowed IPs, got %d", len(allowed))
	}
}

func TestIPAccessControllerGetBlockedIPs(t *testing.T) {
	ctrl := utlsclient.NewIPAccessController()

	// 初始应该为空
	blocked := ctrl.GetBlockedIPs()
	if len(blocked) != 0 {
		t.Error("Should have no blocked IPs initially")
	}

	// 添加黑名单IP
	ctrl.AddIP("1.2.3.4", false)
	ctrl.AddIP("1.2.3.5", false)

	blocked = ctrl.GetBlockedIPs()
	if len(blocked) != 2 {
		t.Errorf("Expected 2 blocked IPs, got %d", len(blocked))
	}
}

func TestIPAccessControllerRemoveFromBlacklist(t *testing.T) {
	ctrl := utlsclient.NewIPAccessController()

	// 添加到黑名单
	ctrl.AddIP("1.2.3.4", false)
	if ctrl.IsIPAllowed("1.2.3.4") {
		t.Error("IP should be blocked")
	}

	// 从黑名单移除
	ctrl.RemoveFromBlacklist("1.2.3.4")

	// 验证已从黑名单移除
	blocked := ctrl.GetBlockedIPs()
	for _, ip := range blocked {
		if ip == "1.2.3.4" {
			t.Error("IP should be removed from blacklist")
		}
	}

	// IP应该被允许（实现中默认允许，且已从黑名单移除）
	if !ctrl.IsIPAllowed("1.2.3.4") {
		t.Error("IP should be allowed after removing from blacklist (implementation allows by default)")
	}
}

func TestIPAccessControllerAddToWhitelist(t *testing.T) {
	ctrl := utlsclient.NewIPAccessController()

	// 添加到白名单
	ctrl.AddToWhitelist("1.2.3.4")
	if !ctrl.IsIPAllowed("1.2.3.4") {
		t.Error("IP should be allowed after adding to whitelist")
	}

	// 验证在白名单中
	allowed := ctrl.GetAllowedIPs()
	found := false
	for _, ip := range allowed {
		if ip == "1.2.3.4" {
			found = true
			break
		}
	}
	if !found {
		t.Error("IP should be in whitelist")
	}
}

func TestIPAccessControllerBlacklistPriority(t *testing.T) {
	ctrl := utlsclient.NewIPAccessController()

	// 先添加到白名单
	ctrl.AddIP("1.2.3.4", true)
	if !ctrl.IsIPAllowed("1.2.3.4") {
		t.Error("IP should be allowed after adding to whitelist")
	}

	// 再添加到黑名单（黑名单优先）
	ctrl.AddIP("1.2.3.4", false)
	if ctrl.IsIPAllowed("1.2.3.4") {
		t.Error("IP should be denied if in blacklist, even if also in whitelist")
	}

	// 从黑名单移除后，由于AddIP会同时从白名单移除，所以需要重新添加到白名单
	ctrl.RemoveFromBlacklist("1.2.3.4")
	// 由于AddIP(false)会从白名单移除，所以现在不在任何列表中，默认允许
	if !ctrl.IsIPAllowed("1.2.3.4") {
		t.Error("IP should be allowed after removing from blacklist (default allow)")
	}
	
	// 重新添加到白名单
	ctrl.AddIP("1.2.3.4", true)
	if !ctrl.IsIPAllowed("1.2.3.4") {
		t.Error("IP should be allowed after adding to whitelist again")
	}
}

func TestIPAccessControllerConcurrentAccess(t *testing.T) {
	ctrl := utlsclient.NewIPAccessController()

	// 并发添加IP
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func(id int) {
			ip := fmt.Sprintf("1.2.3.%d", id)
			ctrl.AddIP(ip, id%2 == 0) // 交替添加到白名单和黑名单
			ctrl.IsIPAllowed(ip)
			done <- true
		}(i)
	}

	// 等待所有goroutine完成
	for i := 0; i < 100; i++ {
		<-done
	}

	// 验证没有panic，并且状态一致
	allowed := ctrl.GetAllowedIPs()
	blocked := ctrl.GetBlockedIPs()

	if len(allowed)+len(blocked) != 100 {
		t.Errorf("Expected 100 total IPs, got %d", len(allowed)+len(blocked))
	}
}

