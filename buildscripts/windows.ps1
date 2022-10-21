$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
cd ..
go build -o built/Zinigo_Windows_x64.exe Zinigo/main.go
cd buildscripts
$env:GOOS = 'windows'
$env:GOARCH = '386'
cd ..
go build -o built/Zinigo_Windows_x86.exe Zinigo/main.go
cd buildscripts
$env:GOOS = 'linux'
$env:GOARCH = 'amd64'
cd ..
go build -o built/Zinigo_Linux_AMD64 Zinigo/main.go
cd buildscripts
$env:GOOS = 'darwin'
$env:GOARCH = 'amd64'
cd ..
go build -o built/Zinigo_Macos_Intel Zinigo/main.go
cd buildscripts
$env:GOOS = 'darwin'
$env:GOARCH = 'arm64'
cd ..
go build -o built/Zinigo_Macos_AppleSilicon Zinigo/main.go
cd buildscripts