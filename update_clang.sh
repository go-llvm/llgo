#!/bin/sh -e

# Build a version of Clang that matches the version we use in gollvm.
# This will ensure that the bitcode files that we produce are compatible with
# those that Clang produces.

llgodir=$(dirname "$0")
llgodir=$(cd "$llgodir" && pwd)
gollvmdir=$(go list -f '{{.Dir}}' github.com/go-llvm/llvm)

llvmrev=$(grep run_update_llvm_sh_to_get_revision_ $gollvmdir/llvm_dep.go | \
          sed -E -e 's/.*run_update_llvm_sh_to_get_revision_([0-9]+.*)/\1/g')

workdir=$llgodir/workdir
clangdir=$workdir/clang
clang_builddir=$workdir/clang_build

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

touch $workdir/.update-clang-stamp
