@echo off
cd /d "%~dp0"
echo 需要先开 "1_启动服务端.bat"。
echo 这个窗口每秒打印一次在线人数，按 Ctrl+C 退出。
echo.
python tools\manual_client.py
echo.
pause