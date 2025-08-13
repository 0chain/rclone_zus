build:
	CGO_ENABLED=1 go build -x -v -tags bn256 -o rclone rclone.go

build-linux:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -x -v -tags bn256 -o rclone rclone.go

build-windows:
	GOOS=windows GOARCH=amd64 CGO_ENABLED=1 \
	CC=x86_64-w64-mingw32-gcc \
	CXX=x86_64-w64-mingw32-g++ \
	CGO_LDFLAGS="-static -static-libgcc -static-libstdc++ -lstdc++ -lwinpthread -lgcc_eh -lgcc -lmsvcrt -lkernel32 -luser32 -lgdi32 -lwinspool -lshell32 -lole32 -loleaut32 -luuid -lcomdlg32 -ladvapi32 -lws2_32 -liphlpapi -lpsapi -lversion -lwinmm -lwininet -lurlmon -loleacc -lcomctl32" \
	go build -a -v -tags bn256 \
		-ldflags="-extldflags=-static -s -w" \
		-o rclone.exe rclone.go



# for Intel Mac binary
build-mac-amd:
	CGO_ENABLED=1 go build -x -v -tags bn256 -o rclone rclone.go 

# for Apple Silicon
build-mac-arm:
	CGO_ENABLED=1 go build -x -v -tags bn256 -o rclone rclone.go

# build on windows os
build-windows-native:
	CGO_ENABLED=1 go build -x -v -tags bn256 -o rclone.exe rclone.go
