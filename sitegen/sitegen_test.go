package sitegen

import (
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

// assert fails the test if the condition is false.
func assert(tb testing.TB, condition bool, msg string, v ...interface{}) {
	if !condition {
		_, file, line, _ := runtime.Caller(1)
		fmt.Printf("\033[31m%s:%d: "+msg+"\033[39m\n\n", append([]interface{}{filepath.Base(file), line}, v...)...)
		tb.FailNow()
	}
}

// ok fails the test if an err is not nil.
func ok(tb testing.TB, err error) {
	if err != nil {
		_, file, line, _ := runtime.Caller(1)
		fmt.Printf("\033[31m%s:%d: unexpected error: %s\033[39m\n\n", filepath.Base(file), line, err.Error())
		tb.FailNow()
	}
}

// equals fails the test if exp is not equal to act.
func equals(tb testing.TB, act, exp interface{}) {
	if !reflect.DeepEqual(exp, act) {
		_, file, line, _ := runtime.Caller(1)
		fmt.Printf("\033[31m%s:%d:\n\tExpected: %#v\n\tGot: %#v\033[39m\n\n", filepath.Base(file), line, exp, act)
		tb.FailNow()
	}
}

func TestSplit(t *testing.T) {
	in := []byte(`---
title: Testing a new page generator!
---

# Yup

This **works**!
`)

	frontMatter, body, err := splitContent(in)
	ok(t, err)
	equals(t, string(frontMatter), "title: Testing a new page generator!")
	equals(t, string(body), "# Yup\n\nThis **works**!\n")
}

func TestSplit2(t *testing.T) {
	frontMatter, body, err := splitContent([]byte("Just some text"))
	ok(t, err)
	equals(t, string(frontMatter), "")
	equals(t, string(body), "Just some text")
}

func TestHighlight(t *testing.T) {
	in := `console.log("Test");`
	out := ""

	fmt.Println(in)
	fmt.Println(out)
}

func TestParseAttrs(t *testing.T) {
	attrs := parseAttributes("")
	assert(t, len(attrs) == 0, "Unexpected length")

	attrs = parseAttributes(`language="php"`)
	assert(t, len(attrs) == 1, "Unexpected length")
	equals(t, attrs["language"], "php")

	attrs = parseAttributes(`language="php" title="test"`)
	assert(t, len(attrs) == 2, "Unexpected length")
	equals(t, attrs["language"], "php")
	equals(t, attrs["title"], "test")

	attrs = parseAttributes(`language='php'`)
	assert(t, len(attrs) == 1, "Unexpected length")
	equals(t, attrs["language"], "php")

	attrs = parseAttributes(`title="A \" quote"`)
	assert(t, len(attrs) == 1, "Unexpected length")
	equals(t, attrs["title"], "A \" quote")

	attrs = parseAttributes(`title="A \' different quote"`)
	assert(t, len(attrs) == 1, "Unexpected length")
	equals(t, attrs["title"], "A ' different quote")

	attrs = parseAttributes("bad")
	assert(t, len(attrs) == 0, "Unexpected length")

	attrs = parseAttributes("bad='mismatch")
	assert(t, len(attrs) == 0, "Unexpected length")

}
