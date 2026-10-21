@echo off
setlocal
cd /d "%~dp0"
node build.mjs %*
set "BUILD_EXIT_CODE=%ERRORLEVEL%"
pause
exit /b %BUILD_EXIT_CODE%
