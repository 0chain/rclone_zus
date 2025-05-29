build:
	CGO_ENABLED=1 go build -x -v -tags bn256 -o rclone rclone.go

build-linux:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -x -v -tags bn256 -o rclone rclone.go

build-windows:
	GOOS=windows GOARCH=amd64 CGO_ENABLED=1 \
	CC=x86_64-w64-mingw32-gcc \
	CGO_CFLAGS="-static -O2" \
	CGO_LDFLAGS="-static" \
	go build -x -v -tags bn256 -o rclone.exe rclone.go
