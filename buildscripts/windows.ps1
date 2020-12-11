$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
cd ..
go build -o built/Zinigo_Windows_x64.exe Grabazine.go
cd buildscripts
$env:GOOS = 'linux'
$env:GOARCH = 'amd64'
cd ..
go build -o built/Zinigo_Linux_AMD64 Grabazine.go
cd buildscripts