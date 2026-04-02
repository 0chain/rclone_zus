# Default build (current platform, CGO required for BLS crypto)
build:
	CGO_ENABLED=1 go build -v -tags bn256 -o rclone rclone.go

# Linux builds: fully static linking for portability.
# Produces a single binary that works on any Linux distro (no glibc/libc dependency).
# The BLS static library (herumi) is linked in along with glibc statically.
build-linux:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=1 \
	go build -v -tags bn256 \
		-ldflags="-s -w -linkmode external -extldflags '-static'" \
		-o rclone rclone.go

build-linux-arm64:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=1 \
	CC=aarch64-linux-gnu-gcc \
	go build -v -tags bn256 \
		-ldflags="-s -w -linkmode external -extldflags '-static'" \
		-o rclone rclone.go

# Windows builds: CGO with static linking (Windows has ABI stability)
build-windows:
	GOOS=windows GOARCH=amd64 CGO_ENABLED=1 \
	CC=x86_64-w64-mingw32-gcc \
	CXX=x86_64-w64-mingw32-g++ \
	CGO_LDFLAGS="-static -static-libgcc -static-libstdc++ -lstdc++ -lwinpthread -lgcc_eh -lgcc -lmsvcrt -lkernel32 -luser32 -lgdi32 -lwinspool -lshell32 -lole32 -loleaut32 -luuid -lcomdlg32 -ladvapi32 -lws2_32 -liphlpapi -lpsapi -lversion -lwinmm -lwininet -lurlmon -loleacc -lcomctl32" \
	go build -v -tags bn256 \
		-ldflags="-extldflags=-static -s -w" \
		-o rclone.exe rclone.go

# build on windows os natively
build-windows-native:
	CGO_ENABLED=1 go build -v -tags bn256 -o rclone.exe rclone.go

# macOS builds: CGO with system libc (macOS has ABI stability)
build-mac-amd:
	CGO_ENABLED=1 go build -v -tags bn256 -o rclone rclone.go

build-mac-arm:
	CGO_ENABLED=1 go build -v -tags bn256 -o rclone rclone.go

.PHONY: build build-linux build-linux-arm64 build-windows build-windows-native build-mac-amd build-mac-arm
