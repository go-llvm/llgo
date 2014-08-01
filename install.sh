#!/bin/sh -e

prefix="$1"
mkdir -p "$prefix"
prefix=$(cd "$prefix" && pwd)

llgodir=$(dirname "$0")

workdir=$llgodir/workdir
gofrontend_builddir=$workdir/gofrontend_build

# Install the compiler binary.
mkdir -p "$prefix/bin"
cp $workdir/gllgo-stage3 "$prefix/bin/llgo"

# Install llgo-go.
cp $llgodir/llgo-go.sh "$prefix/bin/llgo-go"
chmod +x "$prefix/bin/llgo-go"

# Install libgo. If we did a quick bootstrap, only the stage1 libgo will exist.
if [ -d "$gofrontend_builddir/libgo-stage2" ] ; then
  make -C $gofrontend_builddir/libgo-stage2 install "prefix=$prefix"
else
  make -C $gofrontend_builddir/libgo-stage1 install "prefix=$prefix"
fi

# Install the build variant libraries, but not the export data, which is shared
# between variants.
for i in $workdir/gofrontend_build_*/libgo ; do
  if [ -d $i ] ; then
    make -C $i install-toolexeclibLIBRARIES install-toolexeclibLTLIBRARIES \
      "prefix=$prefix"
  fi
done

# Set up the symlink required by llgo-go.
mkdir -p "$prefix/lib/llgo/go-path"
ln -sf ../../../bin/llgo "$prefix/lib/llgo/go-path/gccgo"
