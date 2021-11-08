#!/bin/sh

#From https://golang.org/doc/install/source#environment
platforms=(aix android darwin dragonfly freebsd illumos js linux netbsd openbsd plan9 solaris windows)
arches=(386 amd64 arm arm64 mips mipsle mips64 mips64le ppc64 ppc64le s390x wasm)

#.go file to build
test "$1" && target="$1"

if ! test "$target"; then
	target="main.go"
fi

binaryName="$2"

for platform in "${platforms[@]}"; do
	for arch in "${arches[@]}"; do
		goos=${platform}
		goarch=${arch}

		output="$binaryName"
		output="$(basename $target | sed 's/\.go//')"

		[[ "windows" == "$goos" ]] && output="$output.exe"

		destination="$(dirname $target)/builds/production/$goos/$goarch/$output"

		echo -e "\e[00;33mGOOS=$goos GOARCH=$goarch go build -ldflags \"-w\" -o $destination $target\e[00m"
		GOOS=$goos GOARCH=$goarch go build -ldflags "-w" -o "$destination" "$target"
	done
done