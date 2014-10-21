#!/bin/sh -e

# Build a version of Clang that matches the version we use in gollvm.
# This will ensure that the bitcode files that we produce are compatible with
# those that Clang produces.

llgodir=$(dirname "$0")
llgodir=$(cd "$llgodir" && pwd)
gollvmdir=$(go list -f '{{.Dir}}' llvm.org/llvm/bindings/go/llvm)

llvmrev=$(cd $gollvmdir && (svn info || git svn info) | grep '^Revision:' | cut -d' ' -f2)

workdir=$llgodir/workdir
clangdir=$workdir/clang
clang_builddir=$workdir/clang_build

if [ "$llvmrev" = "$(cat $workdir/.update-clang-stamp 2>/dev/null)" ] ; then
  exit 0
fi

mkdir -p $workdir
svn co -r $llvmrev https://llvm.org/svn/llvm-project/cfe/trunk $clangdir
mkdir -p $clang_builddir

cmake_flags="../clang -DCMAKE_PREFIX_PATH=$gollvmdir/workdir/llvm_build/bin -DLLVM_INCLUDE_TESTS=NO $@"

if test -n "`which ninja`" ; then
  (cd $clang_builddir && cmake -G Ninja $cmake_flags)
  ninja -C $clang_builddir clang
else
  (cd $clang_builddir && cmake $cmake_flags)
  make -C $clang_builddir -j4 clang
fi

echo $llvmrev > $workdir/.update-clang-stamp
