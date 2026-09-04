@echo off
cd /d "%~dp0"
go build -o maze.exe .
if errorlevel 1 (
  echo Build failed; the existing maze.exe was not run.
  pause
  exit /b 1
)
maze.exe -bench
echo.
echo Opening viewer.html to explore the full results...
start "" "viewer.html"
pause
