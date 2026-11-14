@echo off
setlocal
cd /d "%~dp0"
where go >nul 2>nul
if errorlevel 1 (
  echo [提示] 未找到 go 命令。请先把 Go 安装目录下的 bin 加入 PATH，再双击本脚本。
  pause
  exit /b 1
)
if exist bot.exe goto run
echo [首次运行] 正在编译压测客户端...
go build -o bot.exe ./cmd/bot
if not exist bot.exe (
  echo 编译失败，请保留窗口中的报错信息以便排查。
  pause
  exit /b 1
)
:run
echo.
echo 提醒：要先开 "1_启动服务端.bat"，否则这里会显示连接失败。
echo 现在让 100 个假玩家连上去，跑 15 秒...
echo.
bot.exe -addr 127.0.0.1:9000 -n 100 -d 15s
echo.
pause
