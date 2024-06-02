// Copyright 2021 The CC Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cc // import "modernc.org/cc/v4"

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/dustin/go-humanize"
	"github.com/pbnjay/memory"
	"github.com/pmezard/go-difflib/difflib"
	"modernc.org/ccorpus2"
	"modernc.org/mathutil"
)

var (
	cfs         = ccorpus2.FS
	corpusIndex []string
	defaultCfg0 *Config
	re          *regexp.Regexp
	totalRam    = memory.TotalMemory()

	oTrace = flag.Bool("trc", false, "Print tested paths.")
)

func init() {
	flag.BoolVar(&traceFails, "trcfails", false, "")
	isTesting = true
	extendedErrors = true
	var err error
	if defaultCfg0, err = NewConfig(runtime.GOOS, runtime.GOARCH, []string{LongDouble64Flag(runtime.GOOS, runtime.GOARCH)}...); err != nil {
		panic(errorf("NewConfig: %v", err))
	}

	if err := walk("assets", func(pth string, fi os.FileInfo) error {
		corpusIndex = append(corpusIndex, pth)
		return nil
	}); err != nil {
		panic(err)
	}

}

func walk(dir string, f func(pth string, fi os.FileInfo) error) error {
	fis, err := cfs.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, v := range fis {
		switch {
		case v.IsDir():
			if err = walk(dir+"/"+v.Name(), f); err != nil {
				return err
			}
		default:
			fi, err := v.Info()
			if err != nil {
				return err
			}

			if err = f(dir+"/"+v.Name(), fi); err != nil {
				return err
			}
		}
	}
	return nil
}

// Produce the AST used in examples documentation.
func exampleAST(rule int, src string) (r interface{}) {
	defer func() {
		if err := recover(); err != nil {
			r = fmt.Sprintf("%v (%v:)", err, origin(5))
			trc("%v\n%s", r, debug.Stack())
		}
	}()

	src = strings.Replace(src, "\\n", "\n", -1)
	cfg := &Config{}
	ast, _ := Parse(cfg, []Source{{Name: "example.c", Value: src}})
	if ast == nil {
		return "FAIL"
	}

	pc, _, _, _ := runtime.Caller(1)
	typ := runtime.FuncForPC(pc - 1).Name()
	i := strings.LastIndexByte(typ, '.')
	typ = typ[i+1+len("Example"):]
	i = strings.LastIndexByte(typ, '_')
	typ = typ[:i]
	var node Node
	depth := mathutil.MaxInt
	findNode(typ, ast.TranslationUnit, 0, &node, &depth)
	return node
}

func TestMain(m *testing.M) {
	oRE := flag.String("re", "", "")
	flag.Parse()
	if *oRE != "" {
		re = regexp.MustCompile(*oRE)
	}
	os.Exit(m.Run())
}

func TestScannerSource(t *testing.T) {
	const fn = "all_test.go"
	exp, err := os.ReadFile(fn)
	if err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(fn)
	if err != nil {
		t.Fatal(err)
	}

	testScannerSource(t, fn, f, exp, false)
	testScannerSource(t, fn, exp, exp, false)
	testScannerSource(t, fn, string(exp), exp, false)
	testScannerSource(t, fn, bytes.NewReader(exp), exp, false)
	testScannerSource(t, fn, nil, exp, false)
	testScannerSource(t, fn, 42, nil, true)
}

func testScannerSource(t *testing.T, name string, value interface{}, exp []byte, mustFail bool) {
	ss, err := newScannerSource(newFset(), Source{name, value, nil})
	if err != nil != mustFail {
		t.Fatalf("(%q, %T): %v", name, value, err)
	}

	if err != nil {
		return
	}

	if !bytes.Equal(ss.buf, exp) {
		t.Fatal("buf does not match")
	}
}

func TestToken(t *testing.T) {
	s, err := newScannerSource(newFset(), Source{"test", `abc
def
 ghi
`, nil})
	// abc\ndef\n ghi\n
	//             1
	// 0123 4567 89012
	if err != nil {
		t.Fatal(err)
	}

	s.file.AddLine(4)
	s.file.AddLine(8)
	s.file.AddLine(13)
	for itest, test := range []struct {
		Token
		line int
		col  int
		ch   rune
		sep  string
		src  string
	}{
		{newToken(s, 0, 0, 0, 3), 1, 1, 0, "", "abc"},
		{newToken(s, 1, 3, 4, 3), 2, 1, 1, "\n", "def"},
		{newToken(s, 2, 7, 9, 3), 3, 2, 2, "\n ", "ghi"},
		{newToken(s, eof, 13, 13, 0), 3, 6, eof, "", ""},
	} {
		tok := test.Token
		if g, e := tok.Position().Line, test.line; g != e {
			t.Fatal(itest, g, e)
		}
		if g, e := tok.Position().Column, test.col; g != e {
			t.Fatal(itest, g, e)
		}
		if g, e := tok.Ch, test.ch; g != e {
			t.Fatal(itest, g, e)
		}
		if g, e := string(tok.Sep()), test.sep; g != e {
			t.Fatalf("%v %q %q", itest, g, e)
		}
		if g, e := tok.SrcStr(), test.src; g != e {
			t.Fatalf("%v %q %q", itest, g, e)
		}

		tok2 := tok
		tok2.Set([]byte("xyz0123"), []byte("456789"))
		if g, e := string(tok.Sep()), test.sep; g != e {
			t.Fatalf("%v %q %q", itest, g, e)
		}
		if g, e := tok.SrcStr(), test.src; g != e {
			t.Fatalf("%v %q %q", itest, g, e)
		}
		if g, e := string(tok2.Sep()), "xyz0123"; g != e {
			t.Fatalf("%v %q %q", itest, g, e)
		}
		if g, e := tok2.SrcStr(), "456789"; g != e {
			t.Fatalf("%v %q %q", itest, g, e)
		}
	}
}

type parallel struct {
	limit chan struct{}
	sync.Mutex
	wg sync.WaitGroup
}

func newParallel() *parallel {
	limit := runtime.GOMAXPROCS(0)
	switch runtime.GOARCH {
	case "386", "arm": // 32 bit targets
		limit = 1
	default:
		if totalRam <= 1<<32 {
			limit = 1
		}
	}
	return &parallel{
		limit: make(chan struct{}, limit),
	}
}

func (p *parallel) exec(run func()) {
	p.limit <- struct{}{}
	p.wg.Add(1)

	go func() {
		defer func() {
			p.wg.Done()
			<-p.limit
		}()

		run()
	}()
}

var tokSink []Token

func TestScanner(t *testing.T) {
	defer func() { tokSink = nil }()

	var files, tokens, chars int64
	var m0, m runtime.MemStats
	p := newParallel()
	debug.FreeOSMemory()
	runtime.ReadMemStats(&m0)
	for _, path := range corpusIndex {
		path := path
		switch filepath.Ext(path) {
		case ".c", ".h":
			files++
			p.exec(func() {
				var err error
				var chars0, tokens0 int64
				var toks []Token

				defer func() {
					p.Lock()
					chars += chars0
					tokens += tokens0
					tokSink = append(tokSink, toks...)
					if err != nil {
						t.Error(err)
					}
					p.Unlock()
				}()

				var buf []byte
				if buf, err = getCorpusFile(path); err != nil {
					return
				}

				chars0 += int64(len(buf))
				var s *scanner
				if s, err = newScanner(newFset(), Source{path, buf, nil}, func(msg string, args ...interface{}) {
					s.close()
					err = fmt.Errorf(msg, args...)
				}); err != nil {
					err = fmt.Errorf("%v: %v", path, err)
					return
				}

				for {
					tok := s.cppScan()
					if tok.Ch == eof {
						return
					}

					toks = append(toks, tok)
					tokens0++
				}
			})
		}
	}
	p.wg.Wait()
	debug.FreeOSMemory()
	runtime.ReadMemStats(&m)
	t.Logf("files %v; tokens %v; bytes %v; heap %v; alloc %v", h(files), h(tokens), h(chars), h(m.HeapAlloc-m0.HeapAlloc), h(m.TotalAlloc-m0.TotalAlloc))
}

func getCorpusFile(path string) ([]byte, error) {
	f, err := cfs.Open(path)
	if err != nil {
		return nil, err
	}

	return ioutil.ReadAll(f)
}

func h(v interface{}) string {
	switch x := v.(type) {
	case int64:
		return humanize.Comma(x)
	case uint64:
		if x <= math.MaxInt64 {
			return humanize.Comma(int64(x))
		}
	}
	return fmt.Sprint(v)
}

func BenchmarkScanner(b *testing.B) {
	debug.FreeOSMemory()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var chars int64
		for _, path := range corpusIndex {
			switch filepath.Ext(path) {
			case ".c", ".h":
				buf, err := getCorpusFile(path)
				if err != nil {
					b.Fatal(err)
				}

				chars += int64(len(buf))
				var s *scanner
				if s, err = newScanner(newFset(), Source{path, buf, nil}, func(msg string, args ...interface{}) {
					s.close()
					b.Fatalf(msg, args...)
				}); err != nil {
					b.Fatal(path, err)
				}
				for {
					tok := s.cppScan()
					if tok.Ch == eof {
						break
					}
				}
			}
		}
		b.SetBytes(chars)
	}
}

var (
	cppParseBlacklist = map[string]struct{}{
		"assets/github.com/vnmakarov/mir/c-tests/new/endif.c": {}, // 1:1: unexpected #endif
	}
	astSink []group
)

func TestCPPParse0(t *testing.T) {
	defer func() { astSink = nil }()

	var files, lines, chars int64
	var m0, m runtime.MemStats
	debug.FreeOSMemory()
	runtime.ReadMemStats(&m0)
	for _, path := range corpusIndex {
		if _, ok := cppParseBlacklist[path]; ok {
			continue
		}

		switch filepath.Ext(path) {
		case ".c", ".h":
			buf, err := getCorpusFile(path)
			if err != nil {
				t.Fatal(path, err)
			}

			chars += int64(len(buf))
			var p *cppParser
			if p, err = newCppParser(newFset(), Source{path, buf, nil}, func(msg string, args ...interface{}) {
				p.close()
				t.Fatalf(msg, args...)
			}); err != nil {
				t.Fatal(path, err)
			}

			files++
			ast := p.preprocessingFile()
			if len(ast) == 0 {
				t.Fatalf("%v: empty AST", path)
			}

			eol := ast[len(ast)-1]
			x, ok := eol.(eofLine)
			if !ok {
				t.Fatalf("%v: AST not terminated: %T", p.pos(), eol)
			}

			eof := Token(x)
			lines += int64(eof.Position().Line)
			astSink = append(astSink, ast)
		}
	}
	debug.FreeOSMemory()
	runtime.ReadMemStats(&m)
	astSink = nil
	t.Logf("files %v; lines %v bytes %v; heap %v; alloc %v", h(files), h(lines), h(chars), h(m.HeapAlloc-m0.HeapAlloc), h(m.TotalAlloc-m0.TotalAlloc))
}

func TestCPPParse(t *testing.T) {
	defer func() { astSink = nil }()

	var files, lines, chars int64
	var m0, m runtime.MemStats
	p := newParallel()
	debug.FreeOSMemory()
	runtime.ReadMemStats(&m0)
	for _, path := range corpusIndex {
		path := path
		if _, ok := cppParseBlacklist[path]; ok {
			continue
		}

		switch filepath.Ext(path) {
		case ".c", ".h":
			files++
			p.exec(func() {
				buf, err := getCorpusFile(path)
				if err != nil {
					t.Fatal(path, err)
				}

				var ast group
				var eof Token

				defer func() {
					p.Lock()
					chars += int64(len(buf))
					lines += int64(eof.Position().Line)
					astSink = append(astSink, ast)
					if err != nil {
						t.Error(err)
					}
					p.Unlock()
				}()

				var p *cppParser
				if p, err = newCppParser(newFset(), Source{path, buf, nil}, func(msg string, args ...interface{}) {
					p.close()
					err = fmt.Errorf(msg, args...)
				}); err != nil {
					t.Fatal(path, err)
				}

				if ast = p.preprocessingFile(); len(ast) == 0 {
					t.Fatalf("%v: empty AST", path)
				}

				eol := ast[len(ast)-1]
				x, ok := eol.(eofLine)
				if !ok {
					err = fmt.Errorf("%v: AST not terminated: %T", p.pos(), eol)
					return
				}

				eof = Token(x)
			})
		}
	}
	p.wg.Wait()
	debug.FreeOSMemory()
	runtime.ReadMemStats(&m)
	astSink = nil
	t.Logf("files %v; lines %v bytes %v; heap %v; alloc %v", h(files), h(lines), h(chars), h(m.HeapAlloc-m0.HeapAlloc), h(m.TotalAlloc-m0.TotalAlloc))
}

func BenchmarkCPPParse(b *testing.B) {
	debug.FreeOSMemory()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var chars int64
		for _, path := range corpusIndex {
			if _, ok := cppParseBlacklist[path]; ok {
				continue
			}

			switch filepath.Ext(path) {
			case ".c", ".h":
				buf, err := getCorpusFile(path)
				if err != nil {
					b.Fatal(err)
				}

				chars += int64(len(buf))
				var p *cppParser
				if p, err = newCppParser(newFset(), Source{path, buf, nil}, func(msg string, args ...interface{}) {
					p.close()
					b.Fatalf(msg, args...)
				}); err != nil {
					b.Fatal(path, err)
				}

				ast := p.preprocessingFile()
				if len(ast) == 0 {
					b.Fatalf("%v: empty AST", path)
				}

				eol := ast[len(ast)-1]
				if _, ok := eol.(eofLine); !ok {
					b.Fatalf("%v: AST not terminated: %T", p.pos(), eol)
				}
			}
		}
		b.SetBytes(chars)
	}
}

func defaultCfg() *Config {
	c := *defaultCfg0
	return &c
}

func TestCPPExpand(t *testing.T) {
	if runtime.GOOS == "windows" && runtime.GOARCH == "arm64" {
		t.Skip("TODO")
	}

	testCPPExpand(t, "testdata/cpp-expand/", nil, true)
}

func testCPPExpand(t *testing.T, dir string, blacklist map[string]struct{}, fakeIncludes bool) {
	var fails []string
	var files, ok, skip int
	var c *cpp
	cfg := defaultCfg()
	cfg.fakeIncludes = fakeIncludes
	cfg.PragmaHandler = func(s []Token) error {
		pragmaTestTok := Token{s: s[0].s, Ch: rune(IDENTIFIER)}
		pragmaTestTok.Set(nil, []byte("__pragma"))
		a := textLine{pragmaTestTok}
		for i, v := range s {
			if i == 0 {
				v.Set(sp, v.Src())
			}
			a = append(a, v)
		}
		nlTok := Token{s: s[0].s, Ch: '\n'}
		nlTok.Set(nil, nl)
		c.push(append(a, nlTok))
		return nil
	}
	err := filepath.Walk(filepath.FromSlash(dir), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() || (!strings.HasSuffix(path, ".c") && !strings.HasSuffix(path, ".h")) {
			return nil
		}

		files++
		switch {
		case re != nil:
			if !re.MatchString(path) {
				skip++
				return nil
			}
		default:
			if _, ok := blacklist[filepath.Base(path)]; ok {
				skip++
				return nil
			}
		}

		if *oTrace {
			fmt.Fprintln(os.Stderr, path)
		}
		var b strings.Builder
		if c, err = newCPP(cfg, newFset(), []Source{{path, nil, nil}}, nil); err != nil {
			t.Fatalf("%v: %v", path, err)
		}

		if err := preprocess(c, &b); err != nil {
			fails = append(fails, path)
			t.Fatalf("%v: %v", path, err)
		}

		if strings.Contains(filepath.ToSlash(path), "/mustfail/") {
			if err != nil {
				return nil
			}

			fails = append(fails, path)
			return fmt.Errorf("%v: unexpected success", path)
		}

		if err != nil {
			fails = append(fails, path)
			return err
		}

		expFn := path + ".expect"
		exp, err := os.ReadFile(expFn)
		if err != nil {
			fails = append(fails, path)
			t.Error(err)
		}

		g := strings.ReplaceAll(b.String(), "\r", "")
		g = strings.TrimSpace(g)
		e := strings.ReplaceAll(string(exp), "\r", "")
		e = strings.TrimSpace(e)
		if g != e {
			fails = append(fails, path)
			diff := difflib.UnifiedDiff{
				A:        difflib.SplitLines(e),
				B:        difflib.SplitLines(g),
				FromFile: expFn,
				ToFile:   path,
				Context:  0,
			}
			s, err := difflib.GetUnifiedDiffString(diff)
			if err != nil {
				t.Fatalf("%v: %v", path, err)
			}

			t.Errorf("%v\ngot\n%s\nexp\n%s\ngot\n%s\nexp\n%s", s, g, e, hex.Dump([]byte(g)), hex.Dump([]byte(e)))
			return nil
		}
		ok++
		return nil
	})
	for _, v := range fails {
		t.Log(v)
	}
	t.Logf("files %v, skip %v, ok %v, fails %v", files, skip, ok, len(fails))
	if err != nil {
		t.Fatal(err)
	}
}

func TestPreprocess(t *testing.T) {
	if runtime.GOOS == "windows" && runtime.GOARCH == "arm64" {
		t.Skip("TODO")
	}

	testCPPExpand(t, "testdata/preprocess/", nil, true)
}

func TestTCCExpand(t *testing.T) {
	if runtime.GOOS == "windows" && runtime.GOARCH == "arm64" {
		t.Skip("TODO")
	}

	testCPPExpand(t, "testdata/tcc-0.9.27/tests/pp/", map[string]struct{}{
		"11.c": {}, // https://gcc.gnu.org/onlinedocs/gcc/Variadic-Macros.html#Variadic-Macros
		"16.c": {}, // We don't produce warnings on macro redefinition.
	}, true)
}

func TestInclude(t *testing.T) {
	if runtime.GOOS == "windows" && runtime.GOARCH == "arm64" {
		t.Skip("TODO")
	}

	testCPPExpand(t, "testdata/include/", nil, false)
}

func TestTranslationPhase4(t *testing.T) {
	cfg := defaultCfg()
	cfg.SysIncludePaths = append(cfg.SysIncludePaths, "Include") // benchmarksgame
	cfg.FS = cfs
	blacklistCompCert := map[string]struct{}{}
	blacklistGCC := map[string]struct{}{
		// assertions are deprecated, not supported.
		"950919-1.c": {},
	}
	blacklictTCC := map[string]struct{}{
		// https://gcc.gnu.org/onlinedocs/gcc/Variadic-Macros.html#Variadic-Macros, not supported.
		"11.c": {},
	}
	switch fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH) {
	case "linux/s390x":
		blacklistCompCert["aes.c"] = struct{}{} // Unsupported endianness.
	}
	var files, ok, skip, fails int32
	for _, v := range []struct {
		cfg       *Config
		dir       string
		blacklist map[string]struct{}
	}{
		{cfg, "CompCert-3.6/test/c", blacklistCompCert},
		{cfg, "ccgo", nil},
		{cfg, "gcc-9.1.0/gcc/testsuite/gcc.c-torture", blacklistGCC},
		{cfg, "github.com/AbsInt/CompCert/test/c", blacklistCompCert},
		{cfg, "github.com/cxgo", nil},
		{cfg, "github.com/gcc-mirror/gcc/gcc/testsuite", blacklistGCC},
		{cfg, "github.com/vnmakarov", nil},
		{cfg, "sqlite-amalgamation", nil},
		{cfg, "tcc-0.9.27/tests", blacklictTCC},
		{cfg, "benchmarksgame-team.pages.debian.net", nil},
	} {
		t.Run(v.dir, func(t *testing.T) {
			f, o, s, n := testTranslationPhase4(t, v.cfg, "assets/"+v.dir, v.blacklist)
			files += f
			ok += o
			skip += s
			fails += n
		})
	}
	t.Logf("TOTAL: files %v, skip %v, ok %v, fails %v", files, skip, ok, fails)
}

func testTranslationPhase4(t *testing.T, cfg *Config, dir string, blacklist map[string]struct{}) (files, ok, skip, nfails int32) {
	tmp := t.TempDir()
	var fails []string
	p := newParallel()
	err := walk(dir, func(pth string, fi os.FileInfo) error {
		if fi.IsDir() {
			return nil
		}

		if filepath.Ext(pth) != ".c" {
			return nil
		}

		switch {
		case re != nil:
			if !re.MatchString(pth) {
				atomic.AddInt32(&skip, 1)
				return nil
			}
		default:
			if _, ok := blacklist[filepath.Base(pth)]; ok {
				atomic.AddInt32(&skip, 1)
				return nil
			}
		}

		files++
		apth := pth
		p.exec(func() {
			if *oTrace {
				fmt.Fprintln(os.Stderr, apth)
			}
			var err error

			defer func() {
				p.Lock()

				defer p.Unlock()

				if err != nil {
					fails = append(fails, apth)
					t.Errorf("%v: %v", apth, err)
				}
			}()

			if err = Preprocess(
				cfg,
				[]Source{
					{Name: "<predefined>", Value: cfg.Predefined},
					{Name: "<builtin>", Value: Builtin},
					{Name: apth, FS: cfs},
				},
				io.Discard,
			); err == nil {
				atomic.AddInt32(&ok, 1)
				return
			}

			f, err2 := cfs.Open(apth)
			if err2 != nil {
				err = errorf("", err2)
				return
			}

			defer f.Close()

			b := make([]byte, fi.Size())
			if n, _ := f.Read(b); int64(n) != fi.Size() {
				err = errorf("%v: short read", apth)
				return
			}

			fn := filepath.Join(tmp, filepath.Base(apth))
			if err2 := os.WriteFile(fn, b, 0660); err2 != nil {
				err = errorf("", err2)
				return
			}

			defer os.Remove(fn)

			cmd := exec.Command(cfg.CC, "-E", fn)
			var buf bytes.Buffer
			cmd.Stderr = &buf
			if err2 = cmd.Run(); err2 != nil {
				t.Logf("%v: skip: %v: %s %v", apth, cfg.CC, buf.Bytes(), err2)
				atomic.AddInt32(&skip, 1)
				err = nil
			}
		})
		return nil
	})
	if err != nil {
		t.Error(err)
	}

	p.wg.Wait()
	for _, v := range fails {
		t.Log(v)
	}
	t.Logf("files %v, skip %v, ok %v, fails %v", files, skip, ok, len(fails))
	return files, ok, skip, int32(len(fails))
}

// https://gitlab.com/cznic/cc/-/issues/127
func TestIssue127(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatal(err)
		}
	}()

	if err := os.Chdir(filepath.FromSlash("testdata/issue127/")); err != nil {
		t.Error(err)
		return
	}

	cd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("working directory: %s", cd)
	cfg := defaultCfg()
	cfg.IncludePaths = append(cfg.IncludePaths, "include")
	if err := Preprocess(
		cfg,
		[]Source{{Name: "main.c"}},
		io.Discard,
	); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestBOM(t *testing.T) {
	for i, v := range []struct {
		src string
		err string
	}{
		{"int main() {}", ""},
		{"\xEF\xBB\xBFint main() {}", ""},
	} {
		cfg := *defaultCfg()
		cfg.noPredefinedDeclarator = true
		switch _, err := Parse(&cfg, []Source{{Value: v.src}}); {
		case v.err == "" && err != nil:
			t.Errorf("%v: unexpected error %v", i, err)
		case v.err != "" && err == nil:
			t.Errorf("%v: unexpected success, expected error matching %v", i, v.err)
		case v.err != "":
			if !regexp.MustCompile(v.err).MatchString(err.Error()) {
				t.Errorf("%v: error %v does not match %v", i, err, v.err)
			}
		}
	}
}

func TestStrCatSep(t *testing.T) {
	for i, v := range []struct {
		src         string
		lit         string
		sep         string
		trailingSep string
	}{
		{`int f() {  "a";}`, `"a"`, "  ", "\n"},
		{`int f() {  L"a";}`, `L"a"`, "  ", "\n"},
		{`int f() { "a" "b";}`, `"ab"`, "  ", "\n"},
		{`int f() { "a""b";}`, `"ab"`, " ", "\n"},
		{`int f() { "a";}`, `"a"`, " ", "\n"},

		{`int f() { "a"` + "\n\t" + `"b"; }`, `"ab"`, " \n\t", "\n"},
		{`int f() { /*x*/ /*y*/ "a";}`, `"a"`, " /*x*/ /*y*/ ", "\n"},
		{`int f() { /*x*/` + "\n" + `/*y*/ "a";}`, `"a"`, " /*x*/\n/*y*/ ", "\n"},
		{`int f() { //x` + "\n" + ` "a";}`, `"a"`, " //x\n ", "\n"},
		{`int f() { //x` + "\n" + `"a";}`, `"a"`, " //x\n", "\n"},

		{`int f() { L"a" L"b";}`, `L"ab"`, "  ", "\n"},
		{`int f() { ` + "\n" + ` "a";}`, `"a"`, " \n ", "\n"},
		{`int f() { ` + "\n" + `"a";}`, `"a"`, " \n", "\n"},
		{`int f() {"a" "b";}`, `"ab"`, " ", "\n"},
		{`int f() {"a"/*y*/"b";}`, `"ab"`, "/*y*/", "\n"},

		{`int f() {"a";} /*x*/ `, `"a"`, "", " /*x*/ \n"},
		{`int f() {"a";} /*x*/`, `"a"`, "", " /*x*/\n"},
		{`int f() {"a";} /*x` + "\n" + `*/ `, `"a"`, "", " /*x\n*/ \n"},
		{`int f() {"a";} `, `"a"`, "", " \n"},
		{`int f() {"a";}/*x*/`, `"a"`, "", "/*x*/\n"},

		{`int f() {"a";}` + "\n", `"a"`, "", "\n"},
		{`int f() {"a";}`, `"a"`, "", "\n"},
		{`int f() {/*x*/ /*y*/ "a";}`, `"a"`, "/*x*/ /*y*/ ", "\n"},
		{`int f() {/*x*/"a""b";}`, `"ab"`, "/*x*/", "\n"},
		{`int f() {/*x*/"a"/*y*/"b";}`, `"ab"`, "/*x*//*y*/", "\n"},

		{`int f() {/*x*/"a";}`, `"a"`, "/*x*/", "\n"},
		{`int f() {/*x*//*y*/ "a";}`, `"a"`, "/*x*//*y*/ ", "\n"},
		{`int f() {/*x*//*y*/"a";}`, `"a"`, "/*x*//*y*/", "\n"},
		{`int f() {//` + "\n" + `"a";}`, `"a"`, "//\n", "\n"},
		{`int f() {//x` + "\n" + `"a";}`, `"a"`, "//x\n", "\n"},

		{`int f() {` + "/*A*/\n/*B*/\n" + `"a";}`, `"a"`, "/*A*/\n/*B*/\n", "\n"},
		{`int f() {` + "\n" + ` "a";}`, `"a"`, "\n ", "\n"},
		{`int f() {` + "\n" + `"a";}`, `"a"`, "\n", "\n"},
	} {
		ast, err := Parse(
			&Config{
				doNotInjectFunc:        true,
				noPredefinedDeclarator: true,
			},
			[]Source{{Name: "test", Value: v.src}},
		)
		if err != nil {
			t.Errorf("%v: %v", i, err)
			continue
		}

		// trc("%q -> %q", v.src, nodeSource2(ast.TranslationUnit))
		var n Node
		depth := mathutil.MaxInt
		findNode("PrimaryExpression", ast.TranslationUnit, 0, &n, &depth)
		tok := n.(*PrimaryExpression).Token
		if g, e := tok.SrcStr(), v.lit; g != e {
			t.Errorf("%v: %q %q", i, g, e)
		}
		if g, e := string(tok.Sep()), v.sep; g != e {
			t.Errorf("%v: %q %q", i, g, e)
		}
		if g, e := string(ast.EOF.Sep()), v.trailingSep; g != e {
			t.Errorf("%v: %q %q", i, g, e)
		}
	}
}

func TestParserBug(t *testing.T) {
	blacklistJourdan := map[string]struct{}{
		// Type checking has to detect the fail.
		"bitfield_declaration_ambiguity.fail.c": {},
	}
	t.Run("bug", func(t *testing.T) { testParserBug(t, "testdata/bug", nil) })
	t.Run("jhjourdan", func(t *testing.T) { testParserBug(t, "testdata/jhjourdan", blacklistJourdan) })
}

func testParserBug(t *testing.T, dir string, blacklist map[string]struct{}) {
	tmp := t.TempDir()
	cfg := defaultCfg()
	var fails []string
	var files, ok, skip int
	err := filepath.Walk(filepath.FromSlash(dir), func(pth string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if fi.IsDir() {
			return nil
		}

		if filepath.Ext(pth) != ".c" {
			return nil
		}

		switch {
		case re != nil:
			if !re.MatchString(pth) {
				skip++
				return nil
			}
		}

		files++
		switch {
		case re != nil:
			if !re.MatchString(pth) {
				skip++
				return nil
			}
		default:
			if _, ok := blacklist[filepath.Base(pth)]; ok {
				skip++
				return nil
			}
		}

		if *oTrace {
			fmt.Fprintln(os.Stderr, pth)
		}

		sources := []Source{
			{Name: "<predefined>", Value: cfg.Predefined},
			{Name: "<builtin>", Value: Builtin},
			{Name: pth},
		}
		_, err = Parse(cfg, sources)
		switch {
		case strings.Contains(pth, ".fail."):
			if err == nil {
				fails = append(fails, pth)
				t.Errorf("%v: missing error", pth)
			} else {
				if *oTrace {
					t.Log(err)
				}
				ok++
			}
		case err == nil:
			ok++
		default:
			cmd := exec.Command(cfg.CC, "-c", "-o", filepath.Join(tmp, "test.o"), pth)
			var buf bytes.Buffer
			cmd.Stderr = &buf
			if err2 := cmd.Run(); err2 != nil {
				t.Logf("%v: skip: %v: %s %v", pth, cfg.CC, buf.Bytes(), err2)
				skip++
				break
			}

			fails = append(fails, pth)
			t.Errorf("%v: %v", pth, err)
		}
		return nil
	})
	if err != nil {
		t.Error(err)
	}

	for _, v := range fails {
		t.Log(v)
	}
	t.Logf("files %v, skip %v, ok %v, fails %v", files, skip, ok, len(fails))
}

func TestParse(t *testing.T) {
	cfg := defaultCfg()
	cfg.SysIncludePaths = append(cfg.SysIncludePaths, "Include") // benchmarksgame
	cfg.FS = cfs
	blacklistCompCert := map[string]struct{}{}
	blacklistGCC := map[string]struct{}{
		// Assertions are deprecated, not supported.
		"950919-1.c": {},
	}
	switch fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH) {
	case "linux/s390x":
		blacklistCompCert["aes.c"] = struct{}{} // Unsupported endianness.
	}
	var files, ok, skip, fails int32
	for _, v := range []struct {
		cfg       *Config
		dir       string
		blacklist map[string]struct{}
	}{
		{cfg, "CompCert-3.6/test/c", blacklistCompCert},
		{cfg, "ccgo", nil},
		{cfg, "gcc-9.1.0/gcc/testsuite/gcc.c-torture", blacklistGCC},
		{cfg, "github.com/AbsInt/CompCert/test/c", blacklistCompCert},
		{cfg, "github.com/cxgo", nil},
		{cfg, "github.com/gcc-mirror/gcc/gcc/testsuite", blacklistGCC},
		{cfg, "github.com/vnmakarov", nil},
		{cfg, "sqlite-amalgamation", nil},
		{cfg, "tcc-0.9.27/tests/tests2", nil},
		{cfg, "benchmarksgame-team.pages.debian.net", nil},
	} {
		t.Run(v.dir, func(t *testing.T) {
			f, o, s, n := testParse(t, v.cfg, "assets/"+v.dir, v.blacklist)
			files += f
			ok += o
			skip += s
			fails += n
		})
	}
	t.Logf("TOTAL: files %v, skip %v, ok %v, fails %v", files, skip, ok, fails)
}

func testParse(t *testing.T, cfg *Config, dir string, blacklist map[string]struct{}) (files, ok, skip, nfails int32) {
	tmp := t.TempDir()
	var fails []string
	p := newParallel()
	err := walk(dir, func(pth string, fi os.FileInfo) error {
		if fi.IsDir() {
			return nil
		}

		if filepath.Ext(pth) != ".c" {
			return nil
		}

		files++
		switch {
		case re != nil:
			if !re.MatchString(pth) {
				atomic.AddInt32(&skip, 1)
				return nil
			}
		default:
			if _, ok := blacklist[filepath.Base(pth)]; ok {
				atomic.AddInt32(&skip, 1)
				return nil
			}
		}
		apth := pth
		p.exec(func() {
			if *oTrace {
				fmt.Fprintln(os.Stderr, apth)
			}

			var err error

			defer func() {
				p.Lock()

				defer p.Unlock()

				if err != nil {
					fails = append(fails, apth)
					t.Errorf("%v: %v", apth, err)
				}

			}()

			func() {
				defer func() {
					if e := recover(); e != nil && err == nil {
						err = fmt.Errorf("%v: PANIC: %v", apth, e)
						// trc("\n%s", debug.Stack())
					}
				}()

				if _, err = Parse(
					cfg,
					[]Source{
						{Name: "<predefined>", Value: cfg.Predefined},
						{Name: "<builtin>", Value: Builtin},
						{Name: apth, FS: cfs},
					},
				); err == nil {
					atomic.AddInt32(&ok, 1)
					return
				}
			}()

			if err == nil {
				return
			}

			f, err2 := cfs.Open(apth)
			if err2 != nil {
				err = errorf("", err2)
				return
			}

			defer f.Close()

			b := make([]byte, fi.Size())
			if n, _ := f.Read(b); int64(n) != fi.Size() {
				err = errorf("%v: short read", apth)
				return
			}

			fn := filepath.Join(tmp, filepath.Base(apth))
			if err2 := os.WriteFile(fn, b, 0660); err2 != nil {
				err = errorf("", err2)
				return
			}

			defer os.Remove(fn)

			cmd := exec.Command(cfg.CC, "-c", "-o", filepath.Join(tmp, "test.o"), fn)
			var buf bytes.Buffer
			cmd.Stderr = &buf
			if err2 = cmd.Run(); err2 != nil {
				t.Logf("%v: skip: %v: %s %v", apth, cfg.CC, buf.Bytes(), err2)
				atomic.AddInt32(&skip, 1)
				err = nil
				return
			}
		})
		return nil
	})
	if err != nil {
		t.Error(err)
	}

	p.wg.Wait()
	for _, v := range fails {
		t.Log(v)
	}
	// fmt.Fprintf(os.Stderr, "%v: files %v, skip %v, ok %v, fails %v\n", dir, files, skip, ok, len(fails))
	t.Logf("files %v, skip %v, ok %v, fails %v", files, skip, ok, len(fails))
	return files, ok, skip, int32(len(fails))
}

func TestTranslateBug(t *testing.T) {
	t.Run("bug", func(t *testing.T) { testTranslateBug(t, "testdata/bug", nil) })
}

func testTranslateBug(t *testing.T, dir string, blacklist map[string]struct{}) {
	tmp := t.TempDir()
	cfg := defaultCfg()
	var fails []string
	var files, ok, skip int
	err := filepath.Walk(filepath.FromSlash(dir), func(pth string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if fi.IsDir() {
			return nil
		}

		if filepath.Ext(pth) != ".c" {
			return nil
		}

		switch {
		case re != nil:
			if !re.MatchString(pth) {
				skip++
				return nil
			}
		}

		files++
		switch {
		case re != nil:
			if !re.MatchString(pth) {
				skip++
				return nil
			}
		default:
			if _, ok := blacklist[filepath.Base(pth)]; ok {
				skip++
				return nil
			}
		}

		if *oTrace {
			fmt.Fprintln(os.Stderr, pth)
		}

		sources := []Source{
			{Name: "<predefined>", Value: cfg.Predefined},
			{Name: "<builtin>", Value: Builtin},
			{Name: pth},
		}
		_, err = Translate(cfg, sources)
		switch {
		case strings.Contains(pth, ".fail."):
			if err == nil {
				fails = append(fails, pth)
				t.Errorf("%v: missing error", pth)
			} else {
				if *oTrace {
					t.Log(err)
				}
				ok++
			}
		case err == nil:
			ok++
		default:
			cmd := exec.Command(cfg.CC, "-c", "-o", filepath.Join(tmp, "test.o"), pth)
			var buf bytes.Buffer
			cmd.Stderr = &buf
			if err2 := cmd.Run(); err2 != nil {
				t.Logf("%v: skip: %v: %s %v", pth, cfg.CC, buf.Bytes(), err2)
				skip++
				break
			}

			fails = append(fails, pth)
			t.Errorf("%v: %v", pth, err)
		}
		return nil
	})
	if err != nil {
		t.Error(err)
	}

	for _, v := range fails {
		t.Log(v)
	}
	t.Logf("files %v, skip %v, ok %v, fails %v", files, skip, ok, len(fails))
}

func TestTranslate(t *testing.T) {
	cfg := defaultCfg()
	cfg.SysIncludePaths = append(cfg.SysIncludePaths, "Include") // benchmarksgame
	cfg.FS = cfs
	blacklistCompCert := map[string]struct{}{}
	blacklistMakarov := map[string]struct{}{}
	blacklistGCC := map[string]struct{}{
		// Assertions are deprecated, not supported.
		"950919-1.c": {},
	}
	switch fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH) {
	case "linux/s390x":
		blacklistCompCert["aes.c"] = struct{}{} // Unsupported endianness.
	case "windows/arm64":
		blacklistGCC["pr49218.c"] = struct{}{}     //TODO
		blacklistGCC["pr54471.c"] = struct{}{}     //TODO
		blacklistGCC["pr61375.c"] = struct{}{}     //TODO
		blacklistGCC["pr63302.c"] = struct{}{}     //TODO
		blacklistGCC["pr65170.c"] = struct{}{}     //TODO
		blacklistGCC["pr70355.c"] = struct{}{}     //TODO
		blacklistGCC["pr84169.c"] = struct{}{}     //TODO
		blacklistGCC["pr84748.c"] = struct{}{}     //TODO
		blacklistGCC["pr85582-2.c"] = struct{}{}   //TODO
		blacklistGCC["pr85582-3.c"] = struct{}{}   //TODO
		blacklistGCC["pr92904.c"] = struct{}{}     //TODO
		blacklistGCC["pr93213.c"] = struct{}{}     //TODO
		blacklistGCC["pr98474.c"] = struct{}{}     //TODO
		blacklistMakarov["setjmp2.c"] = struct{}{} //TODO
	}
	var files, ok, skip, fails int32
	for _, v := range []struct {
		cfg       *Config
		dir       string
		blacklist map[string]struct{}
	}{
		{cfg, "CompCert-3.6/test/c", blacklistCompCert},
		{cfg, "ccgo", nil},
		{cfg, "gcc-9.1.0/gcc/testsuite/gcc.c-torture", blacklistGCC},
		{cfg, "github.com/AbsInt/CompCert/test/c", blacklistCompCert},
		{cfg, "github.com/cxgo", nil},
		{cfg, "github.com/gcc-mirror/gcc/gcc/testsuite", blacklistGCC},
		{cfg, "github.com/vnmakarov", blacklistMakarov},
		{cfg, "sqlite-amalgamation", nil},
		{cfg, "tcc-0.9.27/tests/tests2", nil},
		{cfg, "benchmarksgame-team.pages.debian.net", nil},
	} {
		t.Run(v.dir, func(t *testing.T) {
			f, o, s, n := testTranslate(t, v.cfg, "assets/"+v.dir, v.blacklist)
			files += f
			ok += o
			skip += s
			fails += n
		})
	}
	t.Logf("TOTAL: files %v, skip %v, ok %v, fails %v", files, skip, ok, fails)
}

func testTranslate(t *testing.T, cfg *Config, dir string, blacklist map[string]struct{}) (files, ok, skip, nfails int32) {
	tmp := t.TempDir()
	var fails []string
	p := newParallel()
	err := walk(dir, func(pth string, fi os.FileInfo) error {
		if fi.IsDir() {
			return nil
		}

		if filepath.Ext(pth) != ".c" {
			return nil
		}

		files++
		switch {
		case re != nil:
			if !re.MatchString(pth) {
				atomic.AddInt32(&skip, 1)
				return nil
			}
		default:
			if _, ok := blacklist[filepath.Base(pth)]; ok {
				atomic.AddInt32(&skip, 1)
				return nil
			}
		}
		apth := pth
		afi := fi
		p.exec(func() {
			if *oTrace {
				fmt.Fprintln(os.Stderr, apth)
			}

			var err error

			defer func() {
				p.Lock()

				defer p.Unlock()

				if err != nil {
					fails = append(fails, apth)
					t.Errorf("%v:\n%v", apth, err)
				}

			}()

			func() {
				defer func() {
					if e := recover(); e != nil && err == nil {
						err = fmt.Errorf("%v: PANIC: %v", apth, e)
						trc("%v: PANIC: %v\n%s", apth, e, debug.Stack())
						os.Exit(1)
					}
				}()

				if _, err = Translate(
					cfg,
					[]Source{
						{Name: "<predefined>", Value: cfg.Predefined},
						{Name: "<builtin>", Value: Builtin},
						{Name: apth, FS: cfs},
					},
				); err == nil {
					atomic.AddInt32(&ok, 1)
					return
				}
			}()

			if err == nil {
				return
			}

			f, err2 := cfs.Open(apth)
			if err2 != nil {
				err = errorf("", err2)
				return
			}

			defer f.Close()

			b := make([]byte, afi.Size())
			if n, _ := f.Read(b); int64(n) != afi.Size() {
				err = errorf("%v: short read", apth)
				return
			}

			fn := filepath.Join(tmp, filepath.Base(apth))
			if err2 := os.WriteFile(fn, b, 0660); err2 != nil {
				err = errorf("", err2)
				return
			}

			defer os.Remove(fn)

			cmd := exec.Command(cfg.CC, "-c", "-o", filepath.Join(tmp, "test.o"), fn)
			var buf bytes.Buffer
			cmd.Stderr = &buf
			if err2 = cmd.Run(); err2 != nil {
				t.Logf("%v: skip: %v: %s %v", apth, cfg.CC, buf.Bytes(), err2)
				atomic.AddInt32(&skip, 1)
				err = nil
				return
			}
		})
		return nil
	})
	if err != nil {
		t.Error(err)
	}

	p.wg.Wait()
	for _, v := range fails {
		t.Log(v)
	}
	// fmt.Fprintf(os.Stderr, "%v: files %v, skip %v, ok %v, fails %v\n", dir, files, skip, ok, len(fails))
	t.Logf("files %v, skip %v, ok %v, fails %v", files, skip, ok, len(fails))
	return files, ok, skip, int32(len(fails))
}

func shell(cmd string, args ...string) ([]byte, error) {
	wd, err := absCwd()
	if err != nil {
		return nil, err
	}

	fmt.Printf("execute %s %q in %s\n", cmd, args, wd)
	var b echoWriter
	b.echo = testing.Verbose()
	c := exec.Command(cmd, args...)
	c.Stdout = &b
	c.Stderr = &b
	err = c.Run()
	return b.w.Bytes(), err
}

func absCwd() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	if wd, err = filepath.Abs(wd); err != nil {
		return "", err
	}

	return wd, nil
}

type echoWriter struct {
	w    bytes.Buffer
	echo bool
}

func (w *echoWriter) Write(b []byte) (int, error) {
	if w.echo {
		os.Stdout.Write(b)
	}
	return w.w.Write(b)
}

func mustShell(t *testing.T, cmd string, args ...string) []byte {
	b, err := shell(cmd, args...)
	if err != nil {
		t.Fatalf("%v %s\noutput: %s\nerr: %s", cmd, args, b, err)
	}

	return b
}

func mustUntarFile(t *testing.T, dst, src string, canOverwrite func(fn string, fi os.FileInfo) bool) {
	if err := untarFile(dst, src, canOverwrite); err != nil {
		t.Fatal(err)
	}
}

func untarFile(dst, src string, canOverwrite func(fn string, fi os.FileInfo) bool) error {
	f, err := cfs.Open(src)
	if err != nil {
		return err
	}

	defer f.Close()

	return untar(dst, bufio.NewReader(f), canOverwrite)
}

func untar(dst string, r io.Reader, canOverwrite func(fn string, fi os.FileInfo) bool) error {
	dst = filepath.FromSlash(dst)
	gr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err != nil {
			if err != io.EOF {
				return err
			}

			return nil
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			dir := filepath.Join(dst, hdr.Name)
			if err = os.MkdirAll(dir, 0770); err != nil {
				return err
			}
		case tar.TypeSymlink, tar.TypeXGlobalHeader:
			// skip
		case tar.TypeReg, tar.TypeRegA:
			dir := filepath.Dir(filepath.Join(dst, hdr.Name))
			if _, err := os.Stat(dir); err != nil {
				if !os.IsNotExist(err) {
					return err
				}

				if err = os.MkdirAll(dir, 0770); err != nil {
					return err
				}
			}

			fn := filepath.Join(dst, hdr.Name)
			f, err := os.OpenFile(fn, os.O_CREATE|os.O_WRONLY, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}

			w := bufio.NewWriter(f)
			if _, err = io.Copy(w, tr); err != nil {
				return err
			}

			if err := w.Flush(); err != nil {
				return err
			}

			if err := f.Close(); err != nil {
				return err
			}

			if err := os.Chtimes(fn, hdr.AccessTime, hdr.ModTime); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unexpected tar header typeflag %#02x", hdr.Typeflag)
		}
	}

}

func TestMake(t *testing.T) {
	if testing.Short() {
		t.Skip("SKIP: -short")
	}

	if goos == "windows" {
		t.Skip("SKIP: not supported on Windows")
	}

	tmp := t.TempDir()
	CC := defaultCfg().CC
	if !filepath.IsAbs(CC) {
		var err error
		if CC, err = filepath.Abs(CC); err != nil {
			t.Fatal(err)
		}
	}
	os.Setenv("FAKE_CC_CC", CC)
	base := filepath.Base(CC)
	oldCC := os.Getenv("CC")

	defer os.Setenv("CC", oldCC)

	os.Setenv("CC", base)
	cc := filepath.Join(tmp, base)
	args := []string{"build", "-o", cc}
	if Dmesgs {
		args = append(args, "-tags=cc.dmesg")
	}
	mustShell(t, "go", append(args, "fakecc.go")...)
	if !strings.HasPrefix(base, "gcc") {
		mustShell(t, "cp", cc, filepath.Join(tmp, "gcc"))
	}
	if !strings.HasPrefix(base, "cc") {
		mustShell(t, "cp", cc, filepath.Join(tmp, "cc"))
	}
	if !strings.HasPrefix(base, "clang") {
		mustShell(t, "cp", cc, filepath.Join(tmp, "clang"))
	}
	oldPath := os.Getenv("PATH")

	defer os.Setenv("PATH", oldPath)

	os.Setenv("PATH", tmp+string(os.PathListSeparator)+oldPath)
	var files, ok, skip, fails int32
	all := []string{
		"darwin/amd64",
		"darwin/arm64",
		"freebsd/386",
		"freebsd/amd64",
		"linux/386",
		"linux/amd64",
		"linux/arm",
		"linux/arm64",
		"linux/riscv64",
		"linux/s390x",
		"netbsd/amd64",
		"openbsd/amd64",
	}
	hdf5 := []string{
		"freebsd/386",
		"freebsd/amd64",
		"linux/386",
		"linux/amd64",
		"linux/arm",
		"linux/arm64",
		"linux/riscv64",
		"linux/s390x",
		"netbsd/amd64",
		"openbsd/amd64",
		// "darwin/amd64", //TODO PATH_MAX undefined
		// "darwin/arm64", //TODO PATH_MAX undefined
	}
	mpfr := []string{
		"darwin/amd64",
		"darwin/arm64",
		"freebsd/386",
		"freebsd/amd64",
		"linux/386",
		"linux/amd64",
		"linux/arm",
		"linux/arm64",
		"linux/s390x",
		"netbsd/amd64",
		"openbsd/amd64",
		// "linux/riscv64", //TODO
	}
	qbe := []string{
		"amd64",
		"arm64",
		"riscv64",
	}
	qjs := []string{
		"freebsd/386",
		"freebsd/amd64",
		"linux/386",
		"linux/amd64",
		"linux/arm",
		"linux/arm64",
		"linux/riscv64",
		"linux/s390x",
		"netbsd/amd64",
		"openbsd/amd64",
		// "darwin/amd64", //TODO PATH_MAX undefined
		// "darwin/arm64", //TODO PATH_MAX undefined
	}
	redis := []string{
		"freebsd/386",
		"freebsd/amd64",
		"linux/386",
		"linux/amd64",
		"linux/arm64",
		"linux/riscv64",
		"linux/s390x",
		"openbsd/amd64",
		// "darwin/amd64", //TODO SSIZE_MAX undefined
		// "darwin/arm64", //TODO SSIZE_MAX undefined
		// "linux/arm",    //TODO <sys/event.h> not found
		// "netbsd/amd64", //TODO <sys/epoll.h> not found
	}
	cfg := &makeCfg{cc: cc}
	switch goos {
	case "darwin":
		cfg.cflags = "-I/opt/homebrew/include"
	case "freebsd", "openbsd;":
		cfg.cflags = "-I/usr/local/include"
	case "netbsd":
		cfg.cflags = "-I/usr/pkg/include"
	}
	cfg.cflags = strings.TrimSpace(cfg.cflags + LongDouble64Flag(goos, goarch))
	for _, v := range []struct {
		archive string
		dir     string
		cfg     *makeCfg
		filter  []string
	}{
		{"ftp.pcre.org/pub/pcre.tar.gz", "pcre", cfg.add("--disable-cpp"), all},
		{"ftp.pcre.org/pub/pcre2.tar.gz", "pcre2", cfg, all},
		{"github.com/madler/zlib.tar.gz", "zlib", cfg, all},
		{"sourceforge.net/projects/tcl/files/Tcl/tcl.tar.gz", "tcl/unix", cfg.add("--enable-corefoundation=no"), all},
		{"gmplib.org/download/gmp/gmp-6.2.1.tar.gz", "gmp-6.2.1", cfg, all},
		{"www.mpfr.org/mpfr-current/mpfr-4.1.0.tar.gz", "mpfr-4.1.0", cfg, mpfr},
		{"ftp.gnu.org/gnu/mpc/mpc-1.2.1.tar.gz", "mpc-1.2.1", cfg, all},
		{"www.hdfgroup.org/downloads/hdf5/source-code/hdf5-1.12.1.tar.gz", "hdf5-1.12.1", cfg, hdf5},
		{"musl.libc.org/releases/musl-1.2.2.tar.gz", "musl-1.2.2", cfg, []string{"linux"}},
		{"github.com/git/git/archive/refs/tags/v2.35.1.tar.gz", "git-2.35.1", cfg.noConfigure(), all},
		{"github.com/bellard/quickjs/archive/refs/heads/quickjs-master/quickjs-master.tar.gz", "quickjs-master", cfg.noConfigure(), qjs},
		{"download.redis.io/releases/redis-6.2.6.tar.gz", "redis-6.2.6", cfg.noConfigure(), redis},
		{"c9x.me/git/qbe.tar.gz", "qbe", cfg.noConfigure(), qbe},
		{"git.postgresql.org/git/postgresql.tar.gz", "postgresql", cfg, all},
		{"www.lua.org/ftp/lua-5.4.4.tar.gz", "lua-5.4.4", cfg.noConfigure(), all},
	} {
		if !filter(v.filter) {
			continue
		}

		t.Run(v.dir, func(t *testing.T) {
			f, o, s, n := testMake(t, "assets/"+v.archive, v.dir, v.cfg)
			files += f
			ok += o
			skip += s
			fails += n
		})
	}
	t.Logf("TOTAL: files %v, skip %v, ok %v, fails %v", files, skip, ok, fails)
}

var (
	goos   = runtime.GOOS
	goarch = runtime.GOARCH
	osarch = fmt.Sprintf("%s/%s", goos, goarch)
)

func filter(f []string) bool {
	for _, v := range f {
		if v == goos || v == goarch || v == osarch {
			return true
		}
	}

	return false
}

type makeCfg struct {
	cc        string
	cflags    string
	configure []string

	disableConfigure bool
}

func (n *makeCfg) add(a ...string) *makeCfg {
	m := *n
	m.configure = append(m.configure[:len(m.configure):len(m.configure)], a...)
	return &m
}

func (n *makeCfg) noConfigure() *makeCfg {
	m := *n
	m.disableConfigure = true
	return &m
}

func testMake(t *testing.T, archive, dir string, mcfg *makeCfg) (files, ok, skip, nfails int32) {
	tmp := t.TempDir()
	mustUntarFile(t, tmp, archive, nil)
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	newWd := filepath.Join(tmp, dir)
	if err := os.Chdir(newWd); err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatal(err)
		}
	}()

	fn := filepath.Join(newWd, "_fake_cc.log")
	os.Setenv("FAKE_CC_LOG", fn)
	var args []string
	if mcfg != nil {
		if mcfg.cflags != "" {
			os.Setenv("CFLAGS", mcfg.cflags)
		}
		args = mcfg.configure
	}
	if !mcfg.disableConfigure {
		if _, err := shell("./configure", args...); err != nil {
			t.Skipf("SKIP: ./configure: %v", err)
		}
	}
	switch goos {
	case "darwin", "freebsd", "netbsd", "openbsd":
		if _, err := shell("gmake"); err != nil {
			t.Skipf("SKIP: gmake: %v", err)
		}
	default:
		if _, err := shell("make"); err != nil {
			t.Skipf("SKIP: make: %v", err)
		}
	}
	logf, err := os.ReadFile(fn)
	if err != nil {
		t.Fatal(err)
	}

	lines := bytes.Split(logf, []byte{'\n'})
	for _, b := range lines {
		if b = bytes.TrimSpace(b); len(b) == 0 {
			continue
		}

		var a []string
		if err := json.NewDecoder(bytes.NewReader(b)).Decode(&a); err != nil {
			trc("", a)
			t.Fatal(err)
		}

		if len(a) == 0 {
			t.Fatal("unexpected empty line")
		}

		if *oTrace {
			fmt.Printf("%s\n", strings.Join(a, " "))
		}
		files++
		switch a[0] {
		case "FAIL":
			t.Errorf("%s", strings.Join(a, " "))
			nfails++
		case "PASS":
			ok++
		case "SKIP":
			skip++
		default:
			t.Fatalf("unexpected report tag %q", a[0])
		}
	}
	t.Logf("files %v, skip %v, ok %v, fails %v", files, skip, ok, nfails)
	return files, ok, skip, nfails
}

func TestSep(t *testing.T) {
	for i, v := range []struct {
		src         string
		sep         string
		trailingSep string
	}{
		{"int f() {}", "", "\n"},
		{" int f() {}\n", " ", "\n"},
		{"\nint f() {}\n", "\n", "\n"},
		{"/*A*//*B*/int f() {}\n", "/*A*//*B*/", "\n"},
		{"/*A*//*B*/ int f() {}\n", "/*A*//*B*/ ", "\n"},

		{"/*A*//*B*/\nint f() {}\n", "/*A*//*B*/\n", "\n"},
		{"/*A*/ /*B*/int f() {}\n", "/*A*/ /*B*/", "\n"},
		{"/*A*/ /*B*/ int f() {}\n", "/*A*/ /*B*/ ", "\n"},
		{"/*A*/ /*B*/\nint f() {}\n", "/*A*/ /*B*/\n", "\n"},
		{"/*A*/\n/*B*/int f() {}\n", "/*A*/\n/*B*/", "\n"},

		{"/*A*/\n/*B*/ int f() {}\n", "/*A*/\n/*B*/ ", "\n"},
		{"/*A*/\n/*B*/\nint f() {}\n", "/*A*/\n/*B*/\n", "\n"},
		{" /*A*/ /*B*/int f() {}\n", " /*A*/ /*B*/", "\n"},
		{" /*A*/ /*B*/ int f() {}\n", " /*A*/ /*B*/ ", "\n"},
		{" /*A*/ /*B*/\nint f() {}\n", " /*A*/ /*B*/\n", "\n"},

		{" /*A*/\n/*B*/int f() {}\n", " /*A*/\n/*B*/", "\n"},
		{" /*A*/\n/*B*/ int f() {}\n", " /*A*/\n/*B*/ ", "\n"},
		{" /*A*/\n/*B*/\nint f() {}\n", " /*A*/\n/*B*/\n", "\n"},
		{"\n/*A*/ /*B*/int f() {}\n", "\n/*A*/ /*B*/", "\n"},
		{"\n/*A*/ /*B*/ int f() {}\n", "\n/*A*/ /*B*/ ", "\n"},

		{"\n/*A*/ /*B*/\nint f() {}\n", "\n/*A*/ /*B*/\n", "\n"},
		{"\n/*A*/\n/*B*/int f() {}\n", "\n/*A*/\n/*B*/", "\n"},
		{"\n/*A*/\n/*B*/ int f() {}\n", "\n/*A*/\n/*B*/ ", "\n"},
		{"\n/*A*/\n/*B*/\nint f() {}\n", "\n/*A*/\n/*B*/\n", "\n"},
	} {
		ast, err := Parse(
			&Config{},
			[]Source{{Name: "test", Value: v.src + "int __predefined_declarator;"}},
		)
		if err != nil {
			t.Errorf("%v: %v", i, err)
			continue
		}

		t.Logf("%q -> %q", v.src, NodeSource(ast.TranslationUnit))
		var tok Token
		firstToken(ast.TranslationUnit, &tok)
		if g, e := string(tok.Sep()), v.sep; g != e {
			t.Errorf("%v: %q %q", i, g, e)
		}
		if g, e := string(ast.EOF.Sep()), v.trailingSep; g != e {
			t.Errorf("%v: %q %q", i, g, e)
		}
	}
}

func firstToken(n Node, r *Token) {
	if n == nil {
		return
	}

	if x, ok := n.(*LabeledStatement); ok && x != nil {
		t := x.Token
		if r.Seq() == 0 || t.Seq() < r.Seq() {
			*r = t
		}
		return
	}

	if x, ok := n.(Token); ok && x.Seq() != 0 {
		if r.Seq() == 0 || x.Seq() < r.Seq() {
			*r = x
		}
		return
	}

	t := reflect.TypeOf(n)
	v := reflect.ValueOf(n)
	var zero reflect.Value
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
		v = v.Elem()
	}
	if v == zero || v.IsZero() || t.Kind() != reflect.Struct {
		return
	}

	nf := t.NumField()
	for i := 0; i < nf; i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}

		if m, ok := v.Field(i).Interface().(Node); ok {
			firstToken(m, r)
		}
	}
}

// https://gitlab.com/cznic/cc/-/issues/141
func TestIssue141(t *testing.T) {
	cfg := defaultCfg()
	ast, err := Translate(cfg, []Source{
		{Name: "<predefined>", Value: cfg.Predefined},
		{Name: "<builtin>", Value: Builtin},
		{Name: "141.c", Value: `
typedef struct {
        int i;
        char s[];
} t;

t x = {1, "a"};
t y = {2, "bc"};
`},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, nm := range []string{"t", "x", "y"} {
		s := ast.Scope.Nodes[nm]
		if len(s) == 0 {
			t.Fatal(nm)
		}

		x, ok := s[0].(*Declarator)
		if !ok {
			t.Fatal(nm)
		}

		structType, ok := x.Type().(*StructType)
		if !ok {
			t.Fatal(nm)
		}

		if g, e := structType.IsIncomplete(), false; g != e {
			t.Errorf("%s: declarator: %s, IsIncomplete got %v, exp %v", nm, structType, g, e)
		}

		if g, e := structType.Size(), ast.Int.Size(); g != e {
			t.Errorf("%s: declarator: %s, size got %v, exp %v", nm, structType, g, e)
		}

		if g, e := structType.Padding(), 0; g != e {
			t.Errorf("%s: declarator: %s, padding got %v, exp %v", nm, structType, g, e)
		}

		field := structType.FieldByName("s")
		if field == nil {
			t.Fatal(nm)
		}

		fieldType := field.Type()
		if g, e := fieldType.IsIncomplete(), true; g != e {
			t.Errorf("%s.s: declarator: %s, IsIncomplete got %v, exp %v", nm, fieldType, g, e)
		}

		if g, e := fieldType.Size(), int64(-1); g != e {
			t.Errorf("%s.s: declarator: %s, size got %v, exp %v", nm, fieldType, g, e)
		}

	}
	for l := ast.TranslationUnit; l != nil; l = l.TranslationUnit {
		ed := l.ExternalDeclaration
		if ed.Case != ExternalDeclarationDecl {
			continue
		}

		d := ed.Declaration
		if d == nil {
			continue
		}

		idl := d.InitDeclaratorList
		if idl == nil {
			continue
		}

		id := idl.InitDeclarator
		sz2 := 3
		switch nm := id.Declarator.Name(); nm {
		case "x":
			sz2 = 2
			fallthrough
		case "y":
			structType, ok := id.Initializer.Type().(*StructType)
			if !ok {
				t.Fatal(nm)
			}

			if g, e := structType.IsIncomplete(), false; g != e {
				t.Errorf("%s: initializer: %s, IsIncomplete got %v, exp %v", nm, structType, g, e)
			}

			if g, e := structType.Size(), 2*ast.Int.Size(); g != e {
				t.Errorf("%s: initializer: %s, size got %v, exp %v", nm, structType, g, e)
			}

			if g, e := structType.Padding(), int(ast.Int.Size())-sz2; g != e {
				t.Errorf("%s: initializer: %s, padding got %v, exp %v", nm, structType, g, e)
			}

			field := structType.FieldByName("s")
			if field == nil {
				t.Fatal(nm)
			}

			fieldType := field.Type()
			if g, e := fieldType.IsIncomplete(), false; g != e {
				t.Errorf("%s.s: initializer: %s, IsIncomplete got %v, exp %v", nm, fieldType, g, e)
			}

			if g, e := fieldType.Size(), int64(sz2); g != e {
				t.Errorf("%s.s: initializer: %s, size got %v, exp %v", nm, fieldType, g, e)
			}
		}
	}
}

func TestABI(t *testing.T) {
	for _, osarch := range []string{
		"darwin/amd64",
		"darwin/arm64",
		"freebsd/386",
		"freebsd/amd64",
		"freebsd/arm",
		"freebsd/arm64",
		"linux/386",
		"linux/amd64",
		"linux/arm",
		"linux/arm64",
		"linux/ppc64le",
		"linux/riscv64",
		"linux/s390x",
		"netbsd/amd64",
		"netbsd/arm",
		"openbsd/amd64",
		"openbsd/arm64",
		"windows/386",
		"windows/amd64",
		"windows/arm64",
	} {
		a := strings.Split(osarch, "/")
		os := a[0]
		arch := a[1]
		if _, err := NewABI(os, arch); err != nil {
			t.Errorf("%s: %v", osarch, err)
		}
	}
}
