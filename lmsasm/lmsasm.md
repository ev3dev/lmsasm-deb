% LMSASM(1) | User's Manual
% David Lechner
% October 2016

# NAME

lmsasm - LEGO MINDSTORMS bytecode assembler

# SYNOPSIS

lmsasm [--support *vm*] [--output *out-file*] *in-file*

# DESCRIPTION

Compiles an lms bytecode file to lms2012 VM machine language.

# OPTIONS

--support *vm*
: The target VM bytecode definitions. This can be "official" for targeting the
official LEGO firmware/VM, "xtended" for the RoboMatter/National Instruments
extended firmware/VM or "comapt" for the ev3dev `lms2012-comapt` VM. The default
is "official".

--output *out-file*
: The name of the output file. The default is "out.rbf".

*in-file*
: The name of the input lms bytecode file.
