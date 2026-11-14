@echo off
setlocal
cd /d "%~dp0"
where go >nul 2>nul
if errorlevel 1 (
  echo [提示] 未找到 go 命令。请先把 Go 安装目录下的 bin 加入 PATH，再双击本脚本。
  pause
  exit /b 1
)
if exist gs.exe goto run
echo [首次启动] 正在编译服务端，大约 20 秒，请稍等...
go build -o gs.exe .
if not exist gs.exe (
  echo.
  echo 编译失败，请保留窗口中的报错信息以便排查。
  pause
  exit /b 1
)
:run
echo.
echo ================================================================
echo   服务端已启动。这个窗口不要关，关了就等于停服。
echo   游戏端口 9000   统计端口 9001
echo   看效果：另开窗口双击 "2_跑压测.bat" 或 "4_试玩客户端.bat"
echo ================================================================
echo.
gs.exe -addr :9000 -http :9001 -data players.json
echo.
echo 服务端已停止。
pause
