@echo off
set arg1=%1
if "%arg1%"=="" (
set /p password=Password:
)
if "%arg1%"=="-p" ( 
set password=%2
)
bin\datastack-agent.exe -p %password% -c .\conf\agent.conf