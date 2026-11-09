#!/usr/bin/env bash
# build-tools.sh <target> <out>
#
# Builds the ffmpeg and whisper-cli Kando downloads for target (darwin-arm64, darwin-amd64, linux-amd64 or linux-arm64) from the sources .github/tools.env pins, and writes them to out as ffmpeg-<version>-<target>.tar.gz and whisper-<version>-<target>.tar.gz. Each archive holds its programs at the top, with the licenses beside them, as Kando unpacks it into a folder of its own. It runs on a machine of the target's own system and processor.
set -euo pipefail

target=$1
out=$(mkdir -p "$2" && cd "$2" && pwd)
here=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=../tools.env
. "$here/../tools.env"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
prefix=$work/prefix
jobs=$(getconf _NPROCESSORS_ONLN)

# fetch downloads url to the work folder as name and stops unless its SHA-256 is sum.
fetch() {
  local url=$1 name=$2 sum=$3
  curl -fsSL --retry 3 -o "$work/$name" "$url"
  echo "$sum  $work/$name" | shasum -a 256 -c - >/dev/null
}

# pack writes the files of dir, at its top, into out/name.
pack() {
  local dir=$1 name=$2
  tar -C "$dir" -czf "$out/$name" .
  echo "$name: $(cd "$dir" && ls | tr '\n' ' ')"
}

# check_macho stops when a program links a library outside the system, which a user's Mac would not have.
check_macho() {
  local foreign
  foreign=$(otool -L "$1" | tail -n +2 | awk '{print $1}' | grep -vE '^(/usr/lib/|/System/Library/)' || true)
  if [ -n "$foreign" ]; then
    echo "::error::$1 links libraries outside the system: $foreign"
    exit 1
  fi
}

# smoke transcribes a second of silence with the smallest model, so a program that starts but cannot load its backends fails here rather than on a user's computer.
smoke() {
  fetch "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-tiny.bin" ggml-tiny.bin be07e048e1e599ad46341c8d2a135645097a538221678b7acdd1b1919c6e1b21
  "$work/ffmpeg-out/ffmpeg" -nostdin -loglevel error -f lavfi -i anullsrc=r=16000:cl=mono -t 1 -c:a pcm_s16le "$work/silence.wav"
  if ! "$1" -m "$work/ggml-tiny.bin" -f "$work/silence.wav" -np -oj -of "$work/silence" >"$work/smoke.log" 2>&1 || [ ! -s "$work/silence.json" ]; then
    cat "$work/smoke.log"
    echo "::error::whisper-cli could not transcribe"
    exit 1
  fi
}

ffmpeg_darwin() {
  export MACOSX_DEPLOYMENT_TARGET=12.0
  fetch "https://github.com/libsdl-org/SDL/releases/download/release-$SDL2_VERSION/SDL2-$SDL2_VERSION.tar.gz" sdl2.tar.gz "$SDL2_SHA256"
  fetch "https://downloads.videolan.org/pub/videolan/dav1d/$DAV1D_VERSION/dav1d-$DAV1D_VERSION.tar.xz" dav1d.tar.xz "$DAV1D_SHA256"
  fetch "https://ffmpeg.org/releases/ffmpeg-$FFMPEG_VERSION.tar.xz" ffmpeg.tar.xz "$FFMPEG_SHA256"
  tar -C "$work" -xzf "$work/sdl2.tar.gz"
  tar -C "$work" -xJf "$work/dav1d.tar.xz"
  tar -C "$work" -xJf "$work/ffmpeg.tar.xz"

  cmake -S "$work/SDL2-$SDL2_VERSION" -B "$work/sdl2-build" -DCMAKE_BUILD_TYPE=Release -DCMAKE_INSTALL_PREFIX="$prefix" -DSDL_SHARED=OFF -DSDL_STATIC=ON -DSDL_TEST=OFF
  cmake --build "$work/sdl2-build" --parallel "$jobs"
  cmake --install "$work/sdl2-build"

  meson setup "$work/dav1d-build" "$work/dav1d-$DAV1D_VERSION" --prefix="$prefix" --libdir=lib --buildtype=release --default-library=static -Denable_tools=false -Denable_tests=false
  ninja -C "$work/dav1d-build" install

  # Only what is asked for is linked: the runner's Homebrew holds libraries a user's Mac does not.
  (
    cd "$work/ffmpeg-$FFMPEG_VERSION"
    PKG_CONFIG_LIBDIR="$prefix/lib/pkgconfig" ./configure --prefix="$prefix" --pkg-config-flags=--static \
      --extra-cflags="-mmacosx-version-min=$MACOSX_DEPLOYMENT_TARGET" --extra-ldflags="-mmacosx-version-min=$MACOSX_DEPLOYMENT_TARGET" \
      --disable-autodetect --enable-pthreads --enable-videotoolbox --enable-audiotoolbox --enable-zlib --enable-bzlib --enable-iconv \
      --enable-sdl2 --enable-libdav1d --disable-ffprobe --disable-doc --disable-debug --extra-libs=-liconv
    make -j "$jobs"
  )
  local dir=$work/ffmpeg-out
  mkdir -p "$dir"
  for program in ffmpeg ffplay; do
    cp "$work/ffmpeg-$FFMPEG_VERSION/$program" "$dir/"
    check_macho "$dir/$program"
    codesign --force --sign - "$dir/$program"
  done
  "$dir/ffmpeg" -hide_banner -decoders | grep -q libdav1d
  cp "$work/ffmpeg-$FFMPEG_VERSION/COPYING.LGPLv2.1" "$dir/LICENSE-ffmpeg.txt"
  cp "$work/SDL2-$SDL2_VERSION/LICENSE.txt" "$dir/LICENSE-SDL2.txt"
  cp "$work/dav1d-$DAV1D_VERSION/COPYING" "$dir/LICENSE-dav1d.txt"
  pack "$dir" "ffmpeg-$FFMPEG_VERSION-$target.tar.gz"
}

ffmpeg_linux() {
  local arch=linux64 sum=$BTBN_LINUX_AMD64_SHA256
  if [ "$target" = linux-arm64 ]; then
    arch=linuxarm64 sum=$BTBN_LINUX_ARM64_SHA256
  fi
  local name="$BTBN_BUILD-$arch-lgpl-${FFMPEG_VERSION%.*}"
  fetch "https://github.com/BtbN/FFmpeg-Builds/releases/download/$BTBN_RELEASE/$name.tar.xz" ffmpeg.tar.xz "$sum"
  tar -C "$work" -xJf "$work/ffmpeg.tar.xz"
  local dir=$work/ffmpeg-out
  mkdir -p "$dir"
  cp "$work/$name/bin/ffmpeg" "$dir/"
  cp "$work/$name/LICENSE.txt" "$dir/LICENSE-ffmpeg.txt"
  "$dir/ffmpeg" -hide_banner -version | head -1
  pack "$dir" "ffmpeg-$FFMPEG_VERSION-$target.tar.gz"
}

whisper() {
  fetch "https://github.com/ggml-org/whisper.cpp/archive/refs/tags/v$WHISPER_VERSION.tar.gz" whisper.tar.gz "$WHISPER_SHA256"
  tar -C "$work" -xzf "$work/whisper.tar.gz"
  local src=$work/whisper.cpp-$WHISPER_VERSION build=$work/whisper-build dir=$work/whisper-out
  local flags=(-DCMAKE_BUILD_TYPE=Release -DWHISPER_BUILD_TESTS=OFF -DWHISPER_BUILD_EXAMPLES=ON -DGGML_NATIVE=OFF)
  case $target in
    darwin-*)
      flags+=(-DBUILD_SHARED_LIBS=OFF -DGGML_METAL=ON -DGGML_METAL_EMBED_LIBRARY=ON -DCMAKE_OSX_DEPLOYMENT_TARGET=12.0)
      # Every Intel Mac that runs macOS 12 has AVX2.
      if [ "$target" = darwin-amd64 ]; then
        flags+=(-DGGML_AVX=ON -DGGML_AVX2=ON -DGGML_FMA=ON -DGGML_F16C=ON)
      fi
      ;;
    linux-amd64)
      # A backend per processor generation, loaded from beside the program, so that it runs on any x86-64 and uses what the processor has.
      flags+=(-DBUILD_SHARED_LIBS=ON -DGGML_BACKEND_DL=ON -DGGML_CPU_ALL_VARIANTS=ON '-DCMAKE_INSTALL_RPATH=$ORIGIN' -DCMAKE_BUILD_WITH_INSTALL_RPATH=ON)
      ;;
    linux-arm64)
      # The ARM64 variants need a newer compiler than the older Ubuntu has, so it is one program for every ARMv8 processor.
      flags+=(-DBUILD_SHARED_LIBS=OFF)
      ;;
  esac
  cmake -S "$src" -B "$build" "${flags[@]}"
  cmake --build "$build" --parallel "$jobs" --target whisper-cli
  mkdir -p "$dir"
  cp "$build/bin/whisper-cli" "$dir/"
  case $target in
    darwin-*)
      check_macho "$dir/whisper-cli"
      codesign --force --sign - "$dir/whisper-cli"
      ;;
    linux-amd64)
      # The CPU backends are built as targets of their own, which whisper-cli does not depend on.
      cmake --build "$build" --parallel "$jobs"
      # The libraries go under the names the program asks for (their sonames) and the backends under their own, as files rather than links.
      while IFS= read -r lib; do
        soname=$(readelf -d "$lib" | sed -n 's/.*(SONAME).*\[\(.*\)\]/\1/p')
        cp -L "$lib" "$dir/${soname:-$(basename "$lib")}"
      done < <(find "$build" -name 'lib*.so*' -type f)
      for backend in "$build"/bin/libggml-cpu-*.so; do
        [ -e "$backend" ] && cp -L "$backend" "$dir/"
      done
      ;;
  esac
  if [[ $target == linux-* ]]; then
    echo "glibc: $(objdump -T "$dir"/* 2>/dev/null | grep -o 'GLIBC_[0-9.]*' | sort -uV | tail -1)"
  fi
  smoke "$dir/whisper-cli"
  cp "$src/LICENSE" "$dir/LICENSE-whisper.cpp.txt"
  pack "$dir" "whisper-$WHISPER_VERSION-$target.tar.gz"
}

case $target in
  darwin-arm64 | darwin-amd64) ffmpeg_darwin ;;
  linux-amd64 | linux-arm64) ffmpeg_linux ;;
  *)
    echo "unknown target $target" >&2
    exit 2
    ;;
esac
whisper
