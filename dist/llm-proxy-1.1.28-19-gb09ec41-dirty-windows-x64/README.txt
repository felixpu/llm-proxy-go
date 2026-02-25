LLM Proxy v1.1.28-19-gb09ec41-dirty (Go)
==================

智能 AI 模型代理服务，支持多端点负载均衡和基于内容的智能路由。

快速开始
--------
1. 复制环境变量模板：copy .env.example .env
2. 编辑 .env 文件，设置安全密钥和管理员密码
3. 启动服务：start.bat (或直接运行 llm-proxy.exe)
4. 访问管理界面：http://localhost:8000
5. 默认管理员：admin / admin123

环境变量
--------
- LLM_PROXY_HOST: 监听地址（默认 0.0.0.0）
- LLM_PROXY_PORT: 监听端口（默认 8000）
- LLM_PROXY_SECRET_KEY: 会话加密密钥
- LLM_PROXY_DB: 数据库文件路径
- LOG_LEVEL: 日志级别（DEBUG/INFO/WARN/ERROR）
