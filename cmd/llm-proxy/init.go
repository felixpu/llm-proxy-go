package main

import (
	_ "embed"
	"fmt"
	"os"
)

//go:embed .env.example
var envExampleContent string

// runInit generates .env.example in the current directory.
func runInit() error {
	const filename = ".env.example"

	// Always overwrite .env.example (it's a template, safe to update)
	if err := os.WriteFile(filename, []byte(envExampleContent), 0644); err != nil {
		return fmt.Errorf("write %s: %w", filename, err)
	}

	fmt.Printf("✓ 已生成 %s\n", filename)
	fmt.Println("  下一步：")
	fmt.Println("  1. 复制配置文件：cp .env.example .env  (Windows: copy .env.example .env)")
	fmt.Println("  2. 编辑 .env，修改 LLM_PROXY_SECRET_KEY 和管理员密码")
	fmt.Println("  3. 启动服务：./llm-proxy  (Windows: llm-proxy.exe)")
	fmt.Println("  4. 访问管理界面：http://localhost:8000")

	return nil
}
