@echo off
REM env GOOS=windows GOARCH=amd64 go build -o windows-amd64.exe .\v1
REM env GOOS=windows GOARCH=386 go build -o windows-386.exe .\v1
env GOOS=android GOARCH=arm go build -o android-arm .\v1
REM env GOOS=darwin GOARCH=386 go build -o darwin-386 .\v1
REM env GOOS=darwin GOARCH=arm go build -o darwin-arm .\v1
REM env GOOS=darwin GOARCH=arm64 go build -o darwin-arm64 .\v1