#!/bin/sh -e

llgodir=$(dirname "$0")
llgodir=$(cd "$llgodir" && pwd)

workdir=$llgodir/workdir
gofrontenddir=$workdir/gofrontend
gofrontend_builddir=$workdir/gofrontend_build

bootstrap_type="$1"
shift

case "$bootstrap_type" in
quick | full)
  ;;

*)
  echo "Bootstrap type must be 'quick' or 'full'"
  exit 1
  ;;
esac

# Clean up any previous libgo stages.
rm -rf $gofrontend_builddir/libgo*

# Build a stage1 compiler with gc.
(cd $llgodir/cmd/gllgo && go build -o $workdir/gllgo-stage1)

# Build libgo with the stage1 compiler.
mkdir -p $gofrontend_builddir/libgo-stage1
(cd $gofrontend_builddir/libgo-stage1 && $gofrontenddir/libgo/configure GOC="$workdir/gllgo-stage1 -no-prefix")
make -C $gofrontend_builddir/libgo-stage1 "$@"

# Set up a directory which when added to $PATH causes "gccgo" to resolve
# to our stage1 compiler. This is necessary because the logic in "go build"
# for locating the compiler is fixed.
mkdir -p $gofrontend_builddir/stage1-path
ln -sf $workdir/gllgo-stage1 $gofrontend_builddir/stage1-path/gccgo

# Build a stage2 compiler using the stage1 compiler and libgo.
gllgoflags="-no-prefix -L$gofrontend_builddir/libgo-stage1 -L$gofrontend_builddir/libgo-stage1/.libs -static-libgo"
(cd $llgodir/cmd/gllgo && PATH=$gofrontend_builddir/stage1-path:$PATH go build -compiler gccgo -gccgoflags "$gllgoflags" -o $workdir/gllgo-stage2)

# If this is a quick bootstrap, do not rebuild libgo with the stage2 compiler.
# Instead, use the stage1 libgo.

if [ "$bootstrap_type" == "full" ] ; then
  # Build libgo with the stage2 compiler.
  mkdir -p $gofrontend_builddir/libgo-stage2
  (cd $gofrontend_builddir/libgo-stage2 && $gofrontenddir/libgo/configure GOC="$workdir/gllgo-stage2 -no-prefix")
  make -C $gofrontend_builddir/libgo-stage2 "$@"

  # Set up $gllgoflags to use the stage2 libgo.
  gllgoflags="-no-prefix -L$gofrontend_builddir/libgo-stage2 -L$gofrontend_builddir/libgo-stage2/.libs -static-libgo"
fi

# Set up a directory which when added to $PATH causes "gccgo" to resolve
# to our stage2 compiler.
mkdir -p $gofrontend_builddir/stage2-path
ln -sf $workdir/gllgo-stage2 $gofrontend_builddir/stage2-path/gccgo

# Build the stage3 compiler.
(cd $llgodir/cmd/gllgo && PATH=$gofrontend_builddir/stage2-path:$PATH go build -compiler gccgo -gccgoflags "$gllgoflags" -o $workdir/gllgo-stage3)

# Strip the compiler binaries. The binaries are currently only
# expected to compare equal modulo debug info.
strip -R .note.gnu.build-id -o $workdir/gllgo-stage2.stripped $workdir/gllgo-stage2
strip -R .note.gnu.build-id -o $workdir/gllgo-stage3.stripped $workdir/gllgo-stage3

cmp $workdir/gllgo-stage2.stripped $workdir/gllgo-stage3.stripped && \
echo "Bootstrap completed successfully" && touch $workdir/.bootstrap-stamp && exit 0 || \
echo "Bootstrap failed, binaries differ" && exit 1
