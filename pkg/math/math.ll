; Copyright 2012 Andrew Wilkins.
; Use of this source code is governed by an MIT-style
; license that can be found in the LICENSE file.

declare double @llvm.fabs.f64(double)

declare double @math.ldexp(double, i32)
@math.Ldexp = alias double (double, i32)* @math.ldexp

declare {double, i32} @math.frexp(double)
@math.Frexp = alias {double, i32} (double)* @math.frexp

declare {double, double} @math.modf(double)
@math.Modf = alias {double, double} (double)* @math.modf

declare double @math.log1p(double)
@math.Log1p = alias double (double)* @math.log1p

declare double @math.mod(double, double)
@math.Mod = alias double (double, double)* @math.mod

declare double @math.log(double)
@math.Log = alias double (double)* @math.log

declare double @math.sin(double)
@math.Sin = alias double (double)* @math.sin

declare double @math.asin(double)
@math.Asin = alias double (double)* @math.asin

declare double @math.acos(double)
@math.Acos = alias double (double)* @math.acos

declare double @math.sincos(double)
@math.Sincos = alias double (double)* @math.sincos

declare double @math.cos(double)
@math.Cos = alias double (double)* @math.cos

declare double @math.atan(double)
@math.Atan = alias double (double)* @math.atan

declare double @math.exp(double)
@math.Exp = alias double (double)* @math.exp

declare double @math.exp2(double)
@math.Exp2 = alias double (double)* @math.exp2

declare double @math.expm1(double)
@math.Expm1 = alias double (double)* @math.expm1

declare double @math.floor(double)
@math.Floor = alias double (double)* @math.floor

declare double @math.sqrt(double)
@math.Sqrt = alias double (double)* @math.sqrt

declare double @math.abs(double)
@math.Abs = alias double (double)* @math.abs

declare double @math.ceil(double)
@math.Ceil = alias double (double)* @math.ceil

declare double @math.trunc(double)
@math.Trunc = alias double (double)* @math.trunc

declare double @math.dim(double, double)
@math.Dim = alias double (double, double)* @math.dim

declare double @math.max(double, double)
@math.Max = alias double (double, double)* @math.max

declare double @math.min(double, double)
@math.Min = alias double (double, double)* @math.min

declare double @math.log2(double)
@math.Log2 = alias double (double)* @math.log2

declare double @math.log10(double)
@math.Log10 = alias double (double)* @math.log10

declare double @math.remainder(double, double)
@math.Remainder = alias double (double, double)* @math.remainder

declare double @math.tan(double)
@math.Tan = alias double (double)* @math.tan

declare double @math.atan2(double, double)
@math.Atan2 = alias double (double,double)* @math.atan2

declare double @math.hypot(double, double)
@math.Hypot = alias double (double,double)* @math.hypot
