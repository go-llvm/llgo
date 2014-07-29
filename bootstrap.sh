#!/usr/bin/env bash

set -e

llgodir=$(dirname "$0")
llgodir=$(cd "$llgodir" && pwd)

workdir=$llgodir/workdir
gofrontenddir=$workdir/gofrontend
gofrontend_builddir=$workdir/gofrontend_build

case "$1" in
libgodeps | quick | full)
  bootstrap_type="$1"
  shift
  ;;

*)
  ;;
esac

llgo_cc="${LIBGO_CC:-$workdir/clang_build/bin/clang} $LIBGO_CFLAGS"
llgo_cxx="${LIBGO_CXX:-$workdir/clang_build/bin/clang++} $LIBGO_CFLAGS"

build_libgodeps() {
  local cflags="$1"
  local destdir="$2"
  local variantname="$3"
  local makeflags="$4"

  for d in libbacktrace libffi ; do
    rm -rf $destdir/$d
    mkdir -p $destdir/$d
    case $d in
    libbacktrace)
      config_flags="--enable-host-shared"
      ;;
    *)
      config_flags=""
      ;;
    esac

    echo "# Configuring $variantname $d."
    (cd $destdir/$d && CC="$llgo_cc $cflags" $gofrontenddir/$d/configure --disable-multilib $config_flags > $workdir/${d}-config.log 2>&1 || (echo "# Configure failed, see $workdir/${d}-config.log" && exit 1))
    echo "# Building $variantname $d."
    make -C $destdir/$d $makeflags > $workdir/${d}-make.log 2>&1 || (echo "# Build failed, see $workdir/${d}-make.log" && exit 1)
  done
}

build_libgo_stage() {
  local goc="$1"
  local cflags="$2"
  local gocflags="$3"
  local destdir="$4"
  local variantname="$5"
  local makeflags="$6"

  # Wrap Clang in a program that understands -fgo-dump-spec and -fplan9-extensions.
  libgo_cc="$llgo_cc $cflags"
  libgo_wrapped_cc="env REAL_CC=$(echo $libgo_cc | sed -e 's/ /@SPACE@/g') $workdir/cc-wrapper"

  mkdir -p $destdir
  echo "# Configuring $variantname libgo."
  (cd $destdir && $gofrontenddir/libgo/configure --disable-multilib --without-libatomic CC="$libgo_wrapped_cc" GOC="$goc -no-prefix $GOCFLAGS $gocflags" > $workdir/${variantname}-config.log 2>&1 || (echo "# Configure failed, see $workdir/${variantname}-config.log" && exit 1))
  echo "# Building $variantname libgo."
  make -C $destdir $makeflags 2>&1 | tee $workdir/${variantname}-make.log | $workdir/makefilter
  test "${PIPESTATUS[0]}" = "0" || (echo "# Build failed, see $workdir/${variantname}-make.log" && exit 1)
  echo
}

build_libgo_variant() {
  local goc="$1"
  local cflags="$2"
  local gocflags="$3"
  local variantname="$4"
  local makeflags="$5"

  local destdir="$workdir/gofrontend_build_$variantname"
  rm -rf $destdir
  build_libgodeps "$cflags" "$destdir" "$variantname" "$makeflags"
  build_libgo_stage "$goc" "$cflags" "$gocflags" "$destdir/libgo" "$variantname" "$makeflags"
}

if [ "$bootstrap_type" = "libgodeps" ]; then
  build_libgodeps "$LIBGO_CFLAGS" "$gofrontend_builddir" "normal" "$*"
  touch $workdir/.build-libgodeps-stamp
  exit 0
fi

if [ "$bootstrap_type" != "" ]; then
  # Clean up any previous libgo stages.
  rm -rf $gofrontend_builddir/libgo* $workdir/gofrontend_build_*

  echo "# Building helper programs."
  (cd $llgodir/cmd/cc-wrapper && go build -o $workdir/cc-wrapper)
  (cd $llgodir/cmd/makefilter && go build -o $workdir/makefilter)

  # Build a stage1 compiler with gc.
  echo "# Building stage1 compiler."
  (cd $llgodir/cmd/gllgo && go build -o $workdir/gllgo-stage1)

  # Build libgo with the stage1 compiler.
  build_libgo_stage $workdir/gllgo-stage1 "" "" $gofrontend_builddir/libgo-stage1 stage1 "$*"

  # Set up a directory which when added to $PATH causes "gccgo" to resolve
  # to our stage1 compiler. This is necessary because the logic in "go build"
  # for locating the compiler is fixed.
  mkdir -p $gofrontend_builddir/stage1-path
  ln -sf $workdir/gllgo-stage1 $gofrontend_builddir/stage1-path/gccgo

  # Build a stage2 compiler using the stage1 compiler and libgo.
  echo "# Building stage2 compiler."
  gllgoflags="-no-prefix -L$gofrontend_builddir/libgo-stage1 -L$gofrontend_builddir/libgo-stage1/.libs -static-libgo $GOCFLAGS"
  (cd $llgodir/cmd/gllgo && PATH=$gofrontend_builddir/stage1-path:$PATH CC="$llgo_cc" CXX="$llgo_cxx" go build -compiler gccgo -gccgoflags "$gllgoflags" -o $workdir/gllgo-stage2)

  # If this is a quick bootstrap, do not rebuild libgo with the stage2 compiler.
  # Instead, use the stage1 libgo.

  if [ "$bootstrap_type" == "full" ] ; then
    # Build libgo with the stage2 compiler.
    build_libgo_stage $workdir/gllgo-stage2 "" "" $gofrontent_builddir/libgo-stage2 stage2 "$*"

    # Set up $gllgoflags to use the stage2 libgo.
    gllgoflags="-no-prefix -L$gofrontend_builddir/libgo-stage2 -L$gofrontend_builddir/libgo-stage2/.libs -static-libgo $GOCFLAGS"
  fi

  # Set up a directory which when added to $PATH causes "gccgo" to resolve
  # to our stage2 compiler.
  mkdir -p $gofrontend_builddir/stage2-path
  ln -sf $workdir/gllgo-stage2 $gofrontend_builddir/stage2-path/gccgo

  # Build the stage3 compiler.
  echo "# Building stage3 compiler."
  (cd $llgodir/cmd/gllgo && PATH=$gofrontend_builddir/stage2-path:$PATH CC="$llgo_cc" CXX="$llgo_cxx" go build -compiler gccgo -gccgoflags "$gllgoflags" -o $workdir/gllgo-stage3)

  # Strip the compiler binaries. The binaries are currently only
  # expected to compare equal modulo debug info.
  strip -R .note.gnu.build-id -o $workdir/gllgo-stage2.stripped $workdir/gllgo-stage2
  strip -R .note.gnu.build-id -o $workdir/gllgo-stage3.stripped $workdir/gllgo-stage3

  cmp $workdir/gllgo-stage2.stripped $workdir/gllgo-stage3.stripped && \
  echo "# Bootstrap completed successfully." && touch $workdir/.bootstrap-stamp || \
  (echo "# Bootstrap failed, binaries differ." && exit 1)
fi
