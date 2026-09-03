@echo off
cd /d "%~dp0"
maze.exe -bench
echo.
echo Opening viewer.html to explore the full results...
start "" "viewer.html"
pause
