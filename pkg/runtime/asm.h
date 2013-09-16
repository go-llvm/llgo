#ifndef _LLGO_ASM_H
#define _LLGO_ASM_H

#ifdef __APPLE__
	#define LLGO_ASM_EXPORT(a) __asm__("_"  a)
#else
	#define LLGO_ASM_EXPORT(a) __asm__(a)
#endif

#endif
