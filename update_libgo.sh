#!/bin/sh -e

# Fetch libgo and its dependencies.

llgodir=$(dirname "$0")
llgodir=$(cd "$llgodir" && pwd)

gofrontendrepo=https://code.google.com/p/gofrontend/
gofrontendrev=c63d4fe75597

gccrepo=svn://gcc.gnu.org/svn/gcc/trunk
gccrev=209880

workdir=$llgodir/workdir
gofrontenddir=$workdir/gofrontend

mkdir -p $workdir
if [ -d $gofrontenddir/.hg ] ; then
  (cd $gofrontenddir && hg pull)
else
  hg clone $gofrontendrepo $gofrontenddir
fi

# Revert the previous version of the noext diff (see below).
if [ -e $workdir/libgo-noext.diff ] ; then
  (cd $gofrontenddir && patch -R -p1 < $workdir/libgo-noext.diff)
  rm $workdir/libgo-noext.diff
fi

(cd $gofrontenddir && hg update -r $gofrontendrev)

# Apply a diff that eliminates use of the unnamed struct extension beyond what
# -fms-extensions supports. We keep a copy of the diff in the work directory so
# we know what to revert when we update. This is a temporary measure until a
# similar change can be made upstream.
(cd $gofrontenddir && patch -p1 < $llgodir/libgo-noext.diff)
cp $llgodir/libgo-noext.diff $workdir/

# Some dependencies are stored in the gcc repository.
# TODO(pcc): Ask iant about mirroring these dependencies into gofrontend.

mkdir -p $gofrontenddir/include
mkdir -p $gofrontenddir/libgcc
for f in config.guess config-ml.in config.sub depcomp \
  install-sh ltmain.sh missing move-if-change \
  include/dwarf2.def include/dwarf2.h libgcc/unwind-pe.h ; do
  svn cat -r $gccrev $gccrepo/$f > $gofrontenddir/$f
done

# Avoid pulling in a bunch of unneeded gcc headers.
echo "#define IS_ABSOLUTE_PATH(path) ((path)[0] == '/')" > $gofrontenddir/include/filenames.h

for d in libbacktrace libffi ; do
  svn co -r $gccrev $gccrepo/$d $gofrontenddir/$d
done

touch $workdir/.update-libgo-stamp
