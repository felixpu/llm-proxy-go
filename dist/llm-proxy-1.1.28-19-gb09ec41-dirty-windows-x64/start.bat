@echo off
REM LLM Proxy Go - Windows 启动脚本

echo Starting LLM Proxy...

REM 检查 .env 文件
if not exist .env (
    echo Warning: .env file not found. Using default configuration.
    echo Please copy .env.example to .env and configure it.
)

REM 创建必要的目录
if not exist data mkdir data
if not exist logs mkdir logs

REM 启动服务
llm-proxy.exe

pause
