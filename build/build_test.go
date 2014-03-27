package build_test

import (
	"bytes"
	"io/ioutil"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/go-llvm/llgo/build"
)

func TestLLVMIRReadCloser(t *testing.T) {
	const input = `;abc\n;def\n;xyz`

	var buf bytes.Buffer
	buf.WriteString(input)
	r := build.NewLLVMIRReader(ioutil.NopCloser(&buf))
	b, err := ioutil.ReadAll(iotest.OneByteReader(r))
	if err != nil {
		t.Errorf("unexpected error: %s", err)
	}

	str := string(b)
	expected := strings.Replace(str, ";", "//", -1)
	if str != expected {
		t.Errorf("%q != %q", str, expected)
	}
}
