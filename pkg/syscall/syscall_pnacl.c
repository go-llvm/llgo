extern int open(char const *pathname, int flags, ...);

int syscall_open(char const *pathname, long mode, unsigned int perm) {
    return open(pathname, mode, perm);
}

