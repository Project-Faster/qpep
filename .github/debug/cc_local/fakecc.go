// Copyright 2022 The CC Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore
// +build ignore

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"modernc.org/cc/v4"
	"modernc.org/opt"
)

var (
	f        io.Writer
	failFast = os.Getenv("FAKE_CC_FAIL_FAST")
)

func fail(msg string, args ...interface{}) {
	if cc.Dmesgs {
		cc.Dmesg("fail: %s", fmt.Sprintf(msg, args...))
	}
	fmt.Fprintf(os.Stderr, msg, args...)
	os.Exit(1)
}

func report2(result, more string, rc int) {
	wd, err := os.Getwd()
	if err != nil {
		fail("%v\n", err)
	}

	a := append([]string{result, wd}, fmt.Sprint(os.Args[1:]))
	if more != "" {
		a = append(a, more)
	}
	if cc.Dmesgs {
		cc.Dmesg("==== report2 %s %v\n%s\n----", result, rc, strings.Join(a, "\n"))
	}
	var b bytes.Buffer
	if err := json.NewEncoder(&b).Encode(a); err != nil {
		fail("%v\n", err)
	}

	if _, err := f.Write(b.Bytes()); err != nil {
		fail("%v\n", err)
	}

	if rc >= 0 {
		os.Exit(rc)
	}

	if result == "FAIL" && failFast != "" {
		os.Exit(1)
	}
}

func report(result string, rc int) { report2(result, "", rc) }

func main() {
	if cc.Dmesgs {
		cc.Dmesg("start %q", os.Args)
		defer cc.Dmesg("exit: 0")
	}
	fn := os.Getenv("FAKE_CC_LOG")
	if fn == "" {
		fail("FAKE_CC_LOG not set\n")
	}

	CC := os.Getenv("FAKE_CC_CC")
	if CC == "" {
		fail("FAKE_CC_CC not set\n")
	}

	var err error
	if f, err = os.OpenFile(fn, os.O_APPEND|os.O_CREATE|os.O_RDWR|os.O_SYNC, 0660); err != nil {
		fail("%v\n", err)
	}

	cmd := exec.Command(CC, os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if cc.Dmesgs {
			cc.Dmesg("host C compiler returns %v", err.(*exec.ExitError).ExitCode())
		}
		report("SKIP", err.(*exec.ExitError).ExitCode())
	}

	set := opt.NewSet()
	var D, U, I []string
	var E bool
	set.Arg("D", true, func(opt, arg string) error {
		D = append(D, arg)
		return nil
	})
	set.Arg("I", true, func(opt, arg string) error { I = append(I, arg); return nil })
	set.Arg("U", true, func(opt, arg string) error { U = append(U, arg); return nil })
	set.Opt("E", func(opt string) error { E = true; return nil })
	var inputs []string
	if err := set.Parse(os.Args[1:], func(arg string) error {
		if strings.HasPrefix(arg, "-") {
			return nil
		}

		if strings.HasSuffix(arg, ".c") || strings.HasSuffix(arg, ".h") {
			inputs = append(inputs, arg)
		}

		return nil
	}); err != nil {
		fail("%v\n", err)
	}

	if len(inputs) == 0 {
		return
	}

	I = I[:len(I):len(I)]
	os.Setenv("CC", CC)
	cfg, err := cc.NewConfig(runtime.GOOS, runtime.GOARCH, sanitize(os.Args[1:])...)
	if err != nil {
		fail("%v\n", err)
	}

	cfg.IncludePaths = append([]string{""}, I...)
	cfg.IncludePaths = append(cfg.IncludePaths, cfg.HostIncludePaths...)
	cfg.IncludePaths = append(cfg.IncludePaths, cfg.HostSysIncludePaths...)
	cfg.SysIncludePaths = append(I, cfg.HostSysIncludePaths...)
	defs := buildDefs(D, U)
	for _, v := range inputs {
		if fi, err := os.Stat(v); err != nil || fi.Mode()&os.ModeIrregular != 0 {
			continue
		}

		sources := []cc.Source{
			{Name: "<predefined>", Value: cfg.Predefined},
			{Name: "<builtin>", Value: cc.Builtin},
		}
		if defs != "" {
			sources = append(sources, cc.Source{Name: "<command-line>", Value: defs})
		}
		sources = append(sources, cc.Source{Name: v})
		switch {
		case E:
			if err := cc.Preprocess(cfg, sources, io.Discard); err != nil {
				report("FAIL", -1)
				continue
			}
		default:
			if _, err := cc.Translate(cfg, sources); err != nil {
				src, _ := os.ReadFile(v)
				report2("FAIL", fmt.Sprintf("\nsrc\n----\n%s\n----\n%s", src, err), -1)
				continue
			}
		}

		report("PASS", -1)
	}
}

// Returns args suitable for passing to cc.NewConfig. Removes source file arguments.
// Fails on unknown/unsupported flags.
func sanitize(args []string) (r []string) {
	var fails []string
	set := opt.NewSet()

	// Pass
	set.Arg("D", true, func(opt, val string) error { r = append(r, fmt.Sprintf("%s%s", opt, val)); return nil })
	set.Arg("O", true, func(opt, val string) error { r = append(r, fmt.Sprintf("%s%s", opt, val)); return nil })
	set.Arg("U", true, func(opt, val string) error { r = append(r, fmt.Sprintf("%s%s", opt, val)); return nil })
	set.Arg("mlong-double", false, func(opt, val string) error { r = append(r, fmt.Sprintf("%s=%s", opt, val)); return nil })
	set.Arg("std", false, func(opt, val string) error { r = append(r, fmt.Sprintf("%s=%s", opt, val)); return nil })
	set.Opt("m128bit-long-double", func(opt string) error { r = append(r, opt); return nil })
	set.Opt("m96bit-long-double", func(opt string) error { r = append(r, opt); return nil })
	set.Opt("mlong-double-128", func(opt string) error { r = append(r, opt); return nil })
	set.Opt("mlong-double-64", func(opt string) error { r = append(r, opt); return nil })
	set.Opt("mlong-double-80", func(opt string) error { r = append(r, opt); return nil })
	set.Opt("pthread", func(opt string) error { r = append(r, opt); return nil })

	// Ignore
	set.Arg("I", true, func(opt, val string) error { return nil })
	set.Arg("MF", true, func(opt, val string) error { return nil })
	set.Arg("MQ", true, func(opt, val string) error { return nil })
	set.Arg("MT", true, func(opt, val string) error { return nil })
	set.Arg("l", true, func(opt, val string) error { return nil })
	set.Arg("o", true, func(opt, val string) error { return nil })
	set.Opt("E", func(opt string) error { return nil })
	set.Opt("M", func(opt string) error { return nil })
	set.Opt("MD", func(opt string) error { return nil })
	set.Opt("MM", func(opt string) error { return nil })
	set.Opt("MMD", func(opt string) error { return nil })
	set.Opt("MP", func(opt string) error { return nil })
	set.Opt("Qunused-arguments", func(opt string) error { return nil })
	set.Opt("S", func(opt string) error { return nil })
	set.Opt("c", func(opt string) error { return nil })
	set.Opt("dynamiclib", func(opt string) error { return nil })
	set.Opt("herror_on_warning", func(opt string) error { return nil })
	set.Opt("nostdinc", func(opt string) error { return nil })
	set.Opt("nostdlib", func(opt string) error { return nil })
	set.Opt("pedantic", func(opt string) error { return nil })
	set.Opt("pipe", func(opt string) error { return nil })
	set.Opt("s", func(opt string) error { return nil })
	set.Opt("shared", func(opt string) error { return nil })
	set.Opt("static", func(opt string) error { return nil })
	set.Opt("w", func(opt string) error { return nil })

	if err := set.Parse(os.Args[1:], func(opt string) error {
		if strings.HasPrefix(opt, "-W") {
			return nil
		}

		if strings.HasPrefix(opt, "-dump") {
			return nil
		}

		if strings.HasPrefix(opt, "-f") {
			return nil
		}

		if strings.HasPrefix(opt, "-g") {
			return nil
		}

		if strings.HasPrefix(opt, "-m") {
			return nil
		}

		if strings.HasPrefix(opt, "-") {
			fails = append(fails, opt)
			return nil
		}

		if !strings.HasSuffix(opt, ".c") && !strings.HasSuffix(opt, ".h") {
			r = append(r, opt)
		}
		return nil
	}); err != nil || len(fails) != 0 {
		report2("FAIL", fmt.Sprintf("sanitize: %v %v\n", fails, err), 1)
	}

	if cc.Dmesgs {
		cc.Dmesg("sanitize %v -> %v", args, r)
	}
	return r
}

func buildDefs(D, U []string) string {
	var a []string
	for _, v := range D {
		if i := strings.IndexByte(v, '='); i > 0 {
			a = append(a, fmt.Sprintf("#define %s %s", v[:i], v[i+1:]))
			continue
		}

		a = append(a, fmt.Sprintf("#define %s 1", v))
	}
	for _, v := range U {
		a = append(a, fmt.Sprintf("#undef %s", v))
	}
	return strings.Join(a, "\n")
}
