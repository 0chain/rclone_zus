build:
	CGO_ENABLED=1 go build -x -v -tags bn256 -o rclone rclone.go

build-linux:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -x -v -tags bn256 -o rclone rclone.go

build-windows:
	GOOS=windows GOARCH=amd64 CGO_ENABLED=1 \
	CC=x86_64-w64-mingw32-gcc \
	CGO_CFLAGS="-O2 -static" \
	CGO_LDFLAGS="-static -lstdc++ -lm -lwinpthread" \
	go build -x -v -tags bn256 -ldflags="-extldflags '-static'" -o rclone.exe rclone.go

# for Intel Mac binary
build-mac-amd:
	CGO_ENABLED=1 go build -x -v -tags bn256 -o rclone rclone.go 

# for Apple Silicon
build-mac-arm:
	CGO_ENABLED=1 go build -x -v -tags bn256 -o rclone rclone.go

# build on windows os (with MSYS2 gcc)
build-windows-native:
	set CGO_ENABLED=1 && set CC=gcc && go build -x -v -tags bn256 -o rclone.exe rclone.go