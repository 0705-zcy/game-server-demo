@echo off
cd /d "%~dp0"
echo 服务端统计（online_players 就是当前在线人数）：
echo.
curl.exe -s http://127.0.0.1:9001/stats
echo.
echo.
echo 如果这里一直是空的，说明服务端没开，先双击 "1_启动服务端.bat"。
pause