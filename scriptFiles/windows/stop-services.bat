@echo off
net stop DATASTACKB2BAgentSentinel
FOR /F "tokens=3" %%A IN ('sc queryex DATASTACKB2BAgent ^| findstr PID') DO (SET pid=%%A)
 IF "%pid%" NEQ "0" (
  taskkill /F /PID %pid%
)
sc delete DATASTACKB2BAgent
sc delete DATASTACKB2BAgentSentinel