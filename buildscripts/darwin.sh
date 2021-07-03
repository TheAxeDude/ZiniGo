set GOOS = 'windows'
set GOARCH = 'amd64'
cd ..
go build -o built/Zinigo_Windows_x64.exe Grabazine.go
cd buildscripts
set GOOS = 'linux'
set GOARCH = 'amd64'
cd ..
go build -o built/Zinigo_Linux_AMD64 Grabazine.go
cd buildscripts
set GOOS=darwin
set GOARCH=amd64
cd ..
go build -o built/Zinigo_Macos_AMD64 Grabazine.go
cd buildscripts
