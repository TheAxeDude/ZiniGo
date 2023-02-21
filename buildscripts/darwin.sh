set GOOS=windows
set GOARCH=amd64
cd ..
go build -v -o built/Zinigo_Windows_x64.exe ZiniGo/main.go
cd buildscripts
set GOOS=linux
set GOARCH=amd64
cd ..
go build -o built/Zinigo_Linux_AMD64 ZiniGo/main.go
cd buildscripts
set GOOS=darwin
set GOARCH=amd64
cd ..
go build -o built/Zinigo_Macos_AMD64 ZiniGo/main.go
cd buildscripts
set GOOS=windows
set GOARCH=386
cd ..
go build -o built/Zinigo_Windows.exe ZiniGo/main.go
cd buildscripts
