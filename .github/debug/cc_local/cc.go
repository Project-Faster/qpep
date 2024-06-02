// Copyright 2021 The CC Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:generate rm -f ast.go
//go:generate yy -o /dev/null -position -astImport "\"fmt\"\n\n\"modernc.org/token\"" -prettyString PrettyString -kind Case -noListKind -noPrivateHelpers -forceOptPos parser.yy
//go:generate sed -i "s/\\*.*Expression$/ExpressionNode/" ast.go
//go:generate sed -i "s/\\*ExpressionList$/ExpressionNode/" ast.go
//go:generate patch ast.go ast.patch
//go:generate stringer -output stringer.go -linecomment -type=tokCh,Kind,Linkage,StorageDuration
//go:generate sh -c "go test -run ^Example |fe"

// Package cc is a C99 compiler front end.
//
// Online documentation
//
// See https://godoc.org/modernc.org/cc/v4.
//
// Links
//
// Referenced from elsewhere:
//
//  [0]: http://www.open-std.org/jtc1/sc22/wg14/www/docs/n1256.pdf
//  [1]: https://www.spinellis.gr/blog/20060626/cpp.algo.pdf
//  [2]: https://jhjourdan.mketjh.fr/pdf/jourdan2017simple.pdf
package cc // import "modernc.org/cc/v4"

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"modernc.org/opt"
)

const (
	DmesgsFile = "/tmp/cc.log"

	// Builtin definitions used by package tests. Possibly usable also by consumers
	// of this package.
	Builtin = `
#define __extension__
#define __restrict_arr restrict

#ifndef __builtin_va_list
#define __builtin_va_list __builtin_va_list
typedef void *__builtin_va_list;
#endif

#ifndef __builtin_va_arg
#define __builtin_va_arg __builtin_va_arg
#define __builtin_va_arg(va, type) (*(type*)__builtin_va_arg_impl(va))
#endif

#define __builtin_offsetof(type, member) ((__SIZE_TYPE__)&(((type*)0)->member))
#define __builtin_types_compatible_p(t1, t2) __builtin_types_compatible_p_impl((t1)0, (t2)0)

int __predefined_declarator;

#ifdef __SIZE_TYPE__
typedef __SIZE_TYPE__ __predefined_size_t;
#endif

#ifdef __WCHAR_TYPE__
typedef __WCHAR_TYPE__ __predefined_wchar_t;
#endif

#ifdef __PTRDIFF_TYPE__
typedef __PTRDIFF_TYPE__ __predefined_ptrdiff_t;
#endif

#define __FUNCTION__ __func__
#define __PRETTY_FUNCTION__ __func__

#ifdef __clang__
#define __builtin_convertvector(src, type) ((type)(src))
#endif

#if sizeof(double) == sizeof(long double)
#define __builtin_huge_vall(x) __builtin_huge_val(x)
#endif

__SIZE_TYPE__ __builtin_strcspn(const char *s, const char *reject);
__SIZE_TYPE__ __builtin_strlen(const char *s);
__SIZE_TYPE__ __builtin_strspn(const char *s, const char *accept);
__UINT16_TYPE__ __builtin_bswap16 (__UINT16_TYPE__ x);
__UINT32_TYPE__ __builtin_bswap32 (__UINT32_TYPE__ x);
__UINT64_TYPE__ __builtin_bswap64 (__UINT64_TYPE__ x);
char *__builtin_strcasestr(const char *haystack, const char *needle);
char *__builtin_strcat(char *dest, const char *src);
char *__builtin_strchr(const char *s, int c);
char *__builtin_strcpy(char *dest, const char *src);
char *__builtin_strncat(char *dest, const char *src, __SIZE_TYPE__ n);
char *__builtin_strncpy(char *dest, const char *src, __SIZE_TYPE__ n);
char *__builtin_strpbrk(const char *s, const char *accept);
char *__builtin_strrchr(const char *s, int c);
char *__builtin_strstr(const char *haystack, const char *needle);
double __builtin_acos(double x);
double __builtin_asin(double x);
double __builtin_atan(double x);
double __builtin_atan2(double y, double x);
double __builtin_ceil(double x);
double __builtin_copysign(double x, double y);
double __builtin_cos(double x);
double __builtin_cosh(double x);
double __builtin_exp(double x);
double __builtin_fabs(double x);
double __builtin_floor(double x);
double __builtin_fmod(double x, double y);
double __builtin_frexp(double x, int *exp);
double __builtin_huge_val();
double __builtin_inf();
double __builtin_ldexp(double x, int exp);
double __builtin_log(double x);
double __builtin_log10(double x);
double __builtin_modf(double x, double *iptr);
double __builtin_nan(char*);
double __builtin_pow(double x, double y);
double __builtin_sin(double x);
double __builtin_sinh(double x);
double __builtin_sqrt(double x);
double __builtin_tan(double x);
double __builtin_tanh(double x);
float __builtin_acosf(float x);
float __builtin_asinf(float x);
float __builtin_atan2f(float y, float x);
float __builtin_atanf(float x);
float __builtin_ceilf(float x);
float __builtin_copysignf(float x, float y);
float __builtin_cosf(float x);
float __builtin_coshf(float x);
float __builtin_expf(float x);
float __builtin_fabsf(float x);
float __builtin_floorf(float x);
float __builtin_fmodf(float x, float y);
float __builtin_frexpf(float x, int *exp);
float __builtin_huge_valf();
float __builtin_inff();
float __builtin_ldexpf(float x, int exp);
float __builtin_log10f(float x);
float __builtin_logf(float x);
float __builtin_modff(float x, float *iptr);
float __builtin_nanf(char*);
float __builtin_powf(float x, float y);
float __builtin_sinf(float x);
float __builtin_sinhf(float x);
float __builtin_sqrtf(float x);
float __builtin_tanf(float x);
float __builtin_tanhf(float x);
int __builtin___sprintf_chk(char * str, int flag, __SIZE_TYPE__ strlen, char * format, ...);
int __builtin_abs(int j);
int __builtin_dprintf(int fd, const char *format, ...);
int __builtin_fprintf(void *stream, const char *format, ...);
int __builtin_fputc(int c, void *stream);
int __builtin_fputs(const char *s, void *stream);
int __builtin_fscanf(void *stream, const char *format, ...);
int __builtin_isalnum(int c);
int __builtin_isalpha(int c);
int __builtin_isascii(int c);
int __builtin_isblank(int c);
int __builtin_iscntrl(int c);
int __builtin_isdigit(int c);
int __builtin_isgraph(int c);
int __builtin_islower(int c);
int __builtin_isprint(int c);
int __builtin_ispunct(int c);
int __builtin_isspace(int c);
int __builtin_isunordered(double, double);
int __builtin_isupper(int c);
int __builtin_isxdigit(int c);
int __builtin_memcmp(const void *s1, const void *s2, __SIZE_TYPE__ n);
int __builtin_printf(const char *format, ...);
int __builtin_putc(int c, void *stream);
int __builtin_putchar(int c);
int __builtin_puts(const char *s);
int __builtin_scanf(const char *format, ...);
int __builtin_snprintf(char *str, __SIZE_TYPE__ size, const char *format, ...);
int __builtin_sprintf(char *str, const char *format, ...);
int __builtin_sscanf(const char *str, const char *format, ...);
int __builtin_strcmp(const char *s1, const char *s2);
int __builtin_strncmp(const char *s1, const char *s2, __SIZE_TYPE__ n);
int __builtin_tolower(int c);
int __builtin_toupper(int c);
int __builtin_vdprintf(int fd, const char *format, __builtin_va_list ap);
int __builtin_vfprintf(void *stream, const char *format, __builtin_va_list ap);
int __builtin_vfscanf(void *stream, const char *format, __builtin_va_list ap);
int __builtin_vprintf(const char *format, __builtin_va_list ap);
int __builtin_vscanf(const char *format, __builtin_va_list ap);
int __builtin_vsnprintf(char *str, __SIZE_TYPE__ size, const char *format, __builtin_va_list ap);
int __builtin_vsprintf(char *str, const char *format, __builtin_va_list ap);
int __builtin_vsscanf(const char *str, const char *format, __builtin_va_list ap);
long __builtin_expect(long, long);
long __builtin_labs(long j);
long double __builtin_acosl(long double x);
long double __builtin_asinl(long double x);
long double __builtin_atan2l(long double y, long double x);
long double __builtin_atanl( long double x);
long double __builtin_atanl(long double x);
long double __builtin_ceill(long double x);
long double __builtin_copysignl(long double x, long double y);
long double __builtin_coshl(long double x);
long double __builtin_cosl(long double x);
long double __builtin_expl(long double x);
long double __builtin_fabsl(long double x);
long double __builtin_floorl(long double x);
long double __builtin_fmodl(long double x, long double y);
long double __builtin_frexpl(long double x, int *exp);
long double __builtin_infl();
long double __builtin_ldexpl(long double x, int exp);
long double __builtin_log10l(long double x);
long double __builtin_logl(long double x);
long double __builtin_modfl(long double x, long double *iptr);
long double __builtin_nanl(char*);
long double __builtin_powl(long double x, long double y);
long double __builtin_sinhl(long double x);
long double __builtin_sinl(long double x);
long double __builtin_sqrtl(long double x);
long double __builtin_tanhl(long double x);
long double __builtin_tanl(long double x);
long long __builtin_llabs(long long j);
void *__builtin___memcpy_chk(void * dest, void * src, __SIZE_TYPE__ len, __SIZE_TYPE__ destlen);
void *__builtin___memset_chk(void * dest, int c, __SIZE_TYPE__ len, __SIZE_TYPE__ destlen);
void *__builtin___strncpy_chk (char *dest, char *src, __SIZE_TYPE__ len, __SIZE_TYPE__ dstlen);
void *__builtin__strncpy_chk (char *dest, char *src, __SIZE_TYPE__ len, __SIZE_TYPE__ dstlen);
void *__builtin_calloc(__SIZE_TYPE__ nmemb, __SIZE_TYPE__ size);
void *__builtin_malloc(__SIZE_TYPE__ size);
void *__builtin_memchr(const void *s, int c, __SIZE_TYPE__ n);
void *__builtin_memcpy(void *dest, const void *src, __SIZE_TYPE__ n);
void *__builtin_memrchr(const void *s, int c, __SIZE_TYPE__ n);
void *__builtin_memset(void *s, int c, __SIZE_TYPE__ n);
void *__builtin_rawmemchr(const void *s, int c);
void *__builtin_realloc(void *ptr, __SIZE_TYPE__ size);
void *__builtin_reallocarray(void *ptr, __SIZE_TYPE__ nmemb, __SIZE_TYPE__ size);
void __builtin_abort(void);
void __builtin_exit(int status);
void __builtin_free(void *ptr);
void __builtin_prefetch (void*, ...);
void __builtin_unreachable();

#ifdef __APPLE__
int __darwin_check_fd_set_overflow(int, void *, int) {
	__builtin_abort();
}
#endif
`

//TODO
// int __builtin_isalnum_l(int c, locale_t locale);
// int __builtin_isalpha_l(int c, locale_t locale);
// int __builtin_isascii_l(int c, locale_t locale);
// int __builtin_isblank_l(int c, locale_t locale);
// int __builtin_iscntrl_l(int c, locale_t locale);
// int __builtin_isdigit_l(int c, locale_t locale);
// int __builtin_isgraph_l(int c, locale_t locale);
// int __builtin_islower_l(int c, locale_t locale);
// int __builtin_isprint_l(int c, locale_t locale);
// int __builtin_ispunct_l(int c, locale_t locale);
// int __builtin_isspace_l(int c, locale_t locale);
// int __builtin_isupper_l(int c, locale_t locale);
// int __builtin_isxdigit_l(int c, locale_t locale);
// int __builtin_tolower_l(int c, locale_t locale);
// int __builtin_toupper_l(int c, locale_t locale);
)

var (
	isTesting  bool
	traceFails bool
)

func init() { //TODO- DBG

}

// NewConfig returns the system C compiler configuration, or an error, if
// any. The function will look for the compiler first in the environment
// variable CC, then it'll try other options. Usually that means looking for
// the "cc" and "gcc" binary, in that order.
//
// Additional arguments (flags) in opts are passed to the system C compiler
// unchanged.  For example, the _REENTRANT preprocessor macro is defined when
// the -pthread flag is present.  The set of recognized keywords is adjusted to
// emulate gcc, see:
//
//	https://gcc.gnu.org/onlinedocs/gcc/Alternate-Keywords.html#Alternate-Keywords
//
// Execution of NewConfig is expensive, caching the results is recommended
// wherever possible.
func NewConfig(goos, goarch string, opts ...string) (r *Config, err error) {
	cc, predefined, includePaths, sysIncludePaths, keywords, err := newConfig(opts)
	if err != nil {
		return nil, fmt.Errorf("NewConfig: %v", err)
	}

	switch fmt.Sprintf("%s/%s", goos, goarch) {
	case "netbsd/amd64":
		sysIncludePaths = append(sysIncludePaths, "/usr/pkg/include")
	case "freebsd/386":
		sysIncludePaths = append(sysIncludePaths, "/usr/local/include")
	}
	abi, err := NewABI(goos, goarch)
	if err != nil {
		return nil, err
	}

	if err := adjustLongDouble(predefined, abi); err != nil {
		return nil, err
	}

	includePaths = includePaths[:len(includePaths):len(includePaths)]
	sysIncludePaths = sysIncludePaths[:len(sysIncludePaths):len(sysIncludePaths)]
	return &Config{
		ABI:                 abi,
		CC:                  cc,
		Predefined:          predefined,
		HostIncludePaths:    includePaths,
		HostSysIncludePaths: sysIncludePaths,
		IncludePaths:        append([]string{""}, append(includePaths, sysIncludePaths...)...),
		SysIncludePaths:     sysIncludePaths,
		keywords:            keywords,
	}, nil
}

func adjustLongDouble(predefined string, abi *ABI) error {
	const tag = "#define __SIZEOF_LONG_DOUBLE__ "
	x := strings.Index(predefined, tag)
	if x < 0 {
		return nil
	}

	y := x + len(tag)
	for ; y < len(predefined) && predefined[y] >= '0' && predefined[y] <= '9'; y++ {
	}
	n, err := strconv.ParseInt(predefined[x+len(tag):y], 10, 32)
	if err != nil {
		return fmt.Errorf("parsing %s: %v", tag, err)
	}
	if abi.Types[LongDouble].Size == n {
		return nil
	}

	if abi.Types[Double].Size != n {
		return nil
	}

	abi.Types[LongDouble] = abi.Types[Double]
	return nil
}

func newConfig(opts []string) (cc, predefined string, includePaths, sysIncludePaths []string, keywords map[string]rune, err error) {
	if Dmesgs {
		Dmesg("newConfig(%v)", opts)
		defer func() {
			var s string
			if err != nil {
				s = " (FAIL)"
			}
			Dmesg("newConfig: cc: %q includePaths: %v sysIncludePaths: %v err: %v%s", cc, includePaths, sysIncludePaths, err, s)
		}()
	}
	clone := func() {
		if keywords == nil {
			keywords = make(map[string]rune, len(defaultKeywords))
			for k, v := range defaultKeywords {
				keywords[k] = v
			}
		}
	}
	var args []string
	set := opt.NewSet()

	// https://gcc.gnu.org/onlinedocs/gcc/C-Dialect-Options.html

	set.Opt("ansi", func(opt string) error {
		args = append(args, opt)
		clone()
		delete(keywords, "asm")
		delete(keywords, "inline")
		delete(keywords, "typeof")
		return nil
	})
	set.Opt("fno-asm", func(opt string) error {
		args = append(args, opt)
		clone()
		delete(keywords, "asm")
		delete(keywords, "typeof")
		return nil
	})
	set.Arg("std", false, func(opt, val string) error {
		args = append(args, fmt.Sprintf("%s=%s", opt, val))
		if !strings.HasPrefix(val, "gnu") {
			clone()
			delete(keywords, "asm")
			delete(keywords, "typeof")
		}
		switch val {
		case "c89", "c90", "iso9899:1990", "iso9899:199409":
			clone()
			delete(keywords, "inline")
		}
		return nil
	})

	if err := set.Parse(opts, func(arg string) error {
		args = append(args, arg)
		return nil
	}); err != nil {
		return "", "", nil, nil, nil, errorf("parsing %v: %v", opts, err)
	}

	opts = args[:len(args):len(args)]
	for _, cc = range []string{os.Getenv("CC"), "cc", "gcc"} {
		if cc == "" {
			continue
		}

		cc, err = exec.LookPath(cc)
		if err != nil {
			continue
		}

		args := append(opts, "-dM", "-E", "-")
		pre, err := exec.Command(cc, args...).CombinedOutput()
		if err != nil {
			if Dmesgs {
				Dmesg("cc: %s %v ----\n%s\n----: %v", cc, args, pre, err)
			}
			continue
		}

		sep := "\n"
		if env("GOOS", runtime.GOOS) == "windows" {
			sep = "\r\n"
		}
		a := strings.Split(string(pre), sep)
		w := 0
		for _, v := range a {
			if strings.HasPrefix(v, "#") {
				a[w] = v
				w++
			}
		}
		predefined = strings.Join(a[:w], "\n")
		args = append(opts, "-v", "-E", "-")
		out, err := exec.Command(cc, args...).CombinedOutput()
		if err != nil {
			if Dmesgs {
				Dmesg("cc: %s %v ----\n%s\n----: %v", cc, args, pre, err)
			}
			continue
		}

		a = strings.Split(string(out), sep)
		for i := 0; i < len(a); {
			switch a[i] {
			case "#include \"...\" search starts here:":
			loop:
				for i = i + 1; i < len(a); {
					switch v := a[i]; {
					case strings.HasPrefix(v, "#") || v == "End of search list.":
						break loop
					default:
						includePaths = append(includePaths, strings.TrimSpace(v))
						i++
					}
				}
			case "#include <...> search starts here:":
				for i = i + 1; i < len(a); {
					switch v := a[i]; {
					case strings.HasPrefix(v, "#") || v == "End of search list.":
						return cc, predefined, includePaths, sysIncludePaths, keywords, nil
					default:
						sysIncludePaths = append(sysIncludePaths, strings.TrimSpace(v))
						i++
					}
				}
			default:
				i++
			}
		}
	}
	return "", "", nil, nil, nil, errorf("cannot determine C compiler configuration")
}

// Source is a named part of a translation unit. The name argument is used for
// reporting Token positions.  The value argument can be a string, []byte,
// fs.File, io.ReadCloser, io.Reader or nil. If the value argument is nil an
// attempt is made to open a file using the name argument.
//
// When the value argument is an *os.File, io.ReadCloser or fs.File,
// Value.Close() is called before returning from Preprocess, Parse or
// Translate.
//
// If FS is not nil it overrides the FS from Config.
type Source struct {
	Name  string
	Value interface{}
	FS    fs.FS
}

// Config configures the preprocessor, parser and type checker.
//
// Search paths listed in IncludePaths and SysIncludePaths are used to resolve
// #include "foo.h" and #include <foo.h> preprocessing directives respectively.
// A special search path "@" is interpreted as 'the same directory as where the
// file with the #include directive is'.
//
// If FS is nil, os.Open is used to open named files.
type Config struct {
	ABI                 *ABI
	CC                  string // The configured C compiler, filled by NewConfig.
	FS                  fs.FS
	HostIncludePaths    []string
	HostSysIncludePaths []string
	IncludePaths        []string
	PragmaHandler       func([]Token) error
	Predefined          string // The predefined macros from CC, filled by NewConfig.
	SysIncludePaths     []string
	keywords            map[string]rune

	doNotInjectFunc        bool // testing
	fakeIncludes           bool // testing
	noPredefinedDeclarator bool // testing
}

type errors []string

// Error implements error.
func (e errors) Error() string { return strings.Join(e, "\n") }

func (e *errors) add(err error) { *e = append(*e, err.Error()) }

func (e errors) err() error {
	w := 0
	for i, v := range e {
		if i != 0 {
			if prev, ok := extractPos(e[i-1]); ok {
				if cur, ok := extractPos(v); ok && prev.Filename == cur.Filename && prev.Line == cur.Line {
					continue
				}
			}
		}
		e[w] = v
		w++
	}
	e = e[:w]
	if len(e) == 0 {
		return nil
	}

	return e
}

// Preprocess preprocesses a translation unit, consisting of inputs in sources,
// and writes the result to w.
func Preprocess(cfg *Config, sources []Source, w io.Writer) (err error) {
	cpp, err := newCPP(cfg, newFset(), sources, nil)
	if err != nil {
		return err
	}

	return preprocess(cpp, w)
}

func preprocess(cpp *cpp, w io.Writer) (err error) {
	var errors errors
	cpp.eh = func(msg string, args ...interface{}) { errors = append(errors, fmt.Sprintf(msg, args...)) }
	var prev rune
	for {
		if cpp.rune() == eof {
			return errors.err()
		}

		tok := cpp.shift()
		switch c := tok.Ch; {
		case
			// Prevent the textual form of certain adjacent tokens to form a "false" token,
			// not present in the source.
			c == '#' && prev == '#',
			c == '&' && prev == '&',
			c == '+' && prev == '+',
			c == '+' && prev == rune(PPNUMBER),
			c == '-' && prev == '-',
			c == '-' && prev == rune(PPNUMBER),
			c == '.' && prev == '.',
			c == '.' && prev == rune(PPNUMBER),
			c == '<' && prev == '<',
			c == '=' && prev == '!',
			c == '=' && prev == '%',
			c == '=' && prev == '&',
			c == '=' && prev == '*',
			c == '=' && prev == '+',
			c == '=' && prev == '/',
			c == '=' && prev == '<',
			c == '=' && prev == '=',
			c == '=' && prev == '^',
			c == '=' && prev == '|',
			c == '>' && prev == '-',
			c == '>' && prev == '>',
			c == '|' && prev == '|',
			c == rune(DEC) && prev == '-',
			c == rune(IDENTIFIER) && prev == rune(IDENTIFIER),
			c == rune(INC) && prev == '+':

			if _, err := w.Write(sp); err != nil {
				return err
			}
		}
		if prev == ' ' && tok.Ch == ' ' {
			continue
		}

		prev = tok.Ch
		if _, err = w.Write(tok.Src()); err != nil {
			return err
		}
	}
}

// Parse preprocesses and parses a translation unit, consisting of inputs in
// sources.
func Parse(cfg *Config, sources []Source) (*AST, error) {
	p, err := newParser(cfg, newFset(), sources)
	if err != nil {
		return nil, err
	}

	return p.parse()
}

// Translate preprocesses, parses and type checks a translation unit,
// consisting of inputs in sources.
func Translate(cfg *Config, sources []Source) (*AST, error) {
	ast, err := Parse(cfg, sources)
	if err != nil {
		return nil, err
	}

	if err := ast.check(); err != nil {
		return nil, err
	}

	return ast, nil
}

// NodeTokens returns the source tokens n consists of.
func NodeTokens(n Node) (r []Token) {
	nodeSource(n, &r)
	sort.Slice(r, func(i, j int) bool { return r[i].seq < r[j].seq })
	return r
}

// NodeSource returns the source form of s. Non-empty separators between tokens
// are replaced by a single ' '.
func NodeSource(s ...Node) string {
	var a []Token
	for _, n := range s {
		nodeSource(n, &a)
	}
	sort.Slice(a, func(i, j int) bool { return a[i].seq < a[j].seq })
	var b strings.Builder
	for i, t := range a {
		if i != 0 && len(t.Sep()) != 0 {
			b.WriteByte(' ')
		}
		b.Write(t.Src())
	}
	return b.String()
}

func nodeSource2(s ...Node) string {
	var a []Token
	for _, n := range s {
		nodeSource(n, &a)
	}
	sort.Slice(a, func(i, j int) bool { return a[i].seq < a[j].seq })
	var b strings.Builder
	for _, t := range a {
		b.Write(t.Sep())
		b.Write(t.Src())
	}
	return b.String()
}

func nodeSource(n Node, a *[]Token) {
	if n == nil {
		return
	}

	t := reflect.TypeOf(n)
	v := reflect.ValueOf(n)
	var zero reflect.Value
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
		v = v.Elem()
		if v == zero {
			return
		}
	}

	if t.Kind() != reflect.Struct {
		return
	}

	if x, ok := n.(Token); ok && x.seq != 0 {
		*a = append(*a, x)
		return
	}

	nf := t.NumField()
	for i := 0; i < nf; i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}

		if strings.HasPrefix(f.Name, "Token") {
			if x, ok := v.Field(i).Interface().(Token); ok && x.seq != 0 {
				*a = append(*a, x)
			}
			continue
		}

		if v == zero || v.IsZero() {
			continue
		}

		if m, ok := v.Field(i).Interface().(Node); ok {
			nodeSource(m, a)
		}
	}
}
