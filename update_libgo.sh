#!/bin/sh -e

# Fetch libgo and its dependencies, and build the dependencies.
# We build libgo itself while bootstrapping.

llgodir=$(dirname "$0")
llgodir=$(cd "$llgodir" && pwd)

gofrontendrepo=https://code.google.com/p/gofrontend/
gofrontendrev=93286dc73be0

gccrepo=svn://gcc.gnu.org/svn/gcc/trunk
gccrev=209880

workdir=$llgodir/workdir
gofrontenddir=$workdir/gofrontend
gofrontend_builddir=$workdir/gofrontend_build

mkdir -p $workdir
if [ -d $gofrontenddir/.hg ] ; then
  (cd $gofrontenddir && hg pull)
else
  hg clone $gofrontendrepo $gofrontenddir
fi
(cd $gofrontenddir && hg update -r $gofrontendrev)

# Some dependencies are stored in the gcc repository.
# TODO(pcc): Ask iant about mirroring these dependencies into gofrontend.

mkdir -p $gofrontenddir/include
mkdir -p $gofrontenddir/libgcc
for f in config.guess config-ml.in config.sub depcomp \
  install-sh ltmain.sh missing move-if-change \
  include/dwarf2.{def,h} libgcc/unwind-pe.h ; do
  svn cat -r $gccrev $gccrepo/$f > $gofrontenddir/$f
done

# Avoid pulling in a bunch of unneeded gcc headers.
echo "#define IS_ABSOLUTE_PATH(path) ((path)[0] == '/')" > $gofrontenddir/include/filenames.h

for d in libatomic libbacktrace libffi ; do
  svn co -r $gccrev $gccrepo/$d $gofrontenddir/$d
  mkdir -p $gofrontend_builddir/$d
  (cd $gofrontend_builddir/$d && $gofrontenddir/$d/configure CFLAGS=-fPIC)
  make -C $gofrontend_builddir/$d -j4
done

touch $workdir/.update-stamp
