@echo off
set arg1=%1
if "%arg1%"=="" (
set /p password=Password:
)
if "%arg1%"=="-p" ( 
set password=%2
)
bin\datastack-sentinel.exe -p %password% -service stop
bin\datastack-sentinel.exe -p %password% -service uninstall
bin\datastack-sentinel.exe -p %password% -service install
bin\datastack-sentinel.exe -p %password% -service start
bin\datastack-agent.exe -p %password% -c .\conf\agent.conf -service stop
bin\datastack-agent.exe -p %password% -c .\conf\agent.conf -service uninstall
bin\datastack-agent.exe -p %password% -c .\conf\agent.conf -service install
bin\datastack-agent.exe -p %password% -c .\conf\agent.conf -service start