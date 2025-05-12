

build: 
	CGO_ENABLED=1 go build -x -v -tags bn256 -o rclone rclone.go