@echo off
setlocal
cd /d "%~dp0"
set "GO=C:\Users\Samanda\GoToolchain\go\bin\go.exe"
if exist gs.exe goto run
echo [首次启动] 正在编译服务端，大约 20 秒，请稍等...
"%GO%" build -o gs.exe .
if not exist gs.exe (
  echo.
  echo 编译失败。把这里的报错截图发给豆包就行。
  pause
  exit /b 1
)
:run
echo.
echo ================================================================
echo   服务端已启动。这个黑窗口不要关，关了就等于停服。
echo   游戏端口 9000   统计端口 9001
echo   看效果：另开窗口双击 "2_跑压测.bat" 或 "4_试玩客户端.bat"
echo ================================================================
echo.
gs.exe -addr :9000 -http :9001 -data players.json
echo.
echo 服务端已停止。
pause