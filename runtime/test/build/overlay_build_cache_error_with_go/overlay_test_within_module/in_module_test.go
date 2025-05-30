package overlay_build_cache_error

import (
	"strings"
	"testing"

	"github.com/xhd2015/xgo/runtime/test/build/overlay_build_cache_error_with_go/pkg"
)

func TestOverlay(t *testing.T) {
	msg := pkg.Greet()
	if msg != "hello" && !strings.HasPrefix(msg, "hellogo") {
		t.Fatalf("msg=%s", msg)
	}
}
