package constants

const RUNTIME_MODULE = "github.com/xhd2015/xgo/runtime"

const (
	RUNTIME_INTERNAL_TRAP_PKG    = "github.com/xhd2015/xgo/runtime/internal/trap"
	RUNTIME_INTERNAL_RUNTIME_PKG = "github.com/xhd2015/xgo/runtime/internal/runtime"
	RUNTIME_TRAP_FLAGS_PKG       = "github.com/xhd2015/xgo/runtime/internal/flags"
	RUNTIME_FUNC_INFO_PKG        = "github.com/xhd2015/xgo/runtime/core/info"
	RUNTIME_CORE_PKG             = "github.com/xhd2015/xgo/runtime/core"
	RUNTIME_MOCK_PKG             = "github.com/xhd2015/xgo/runtime/mock"
	RUNTIME_TRACE_PKG            = "github.com/xhd2015/xgo/runtime/trace"
	RUNTIME_TRAP_PKG             = "github.com/xhd2015/xgo/runtime/trap"
)

const (
	RUNTIME_PKG_NAME_FUNC = "__xgo_func_runtime"
	UNSAFE_PKG_NAME_FUNC  = "__xgo_func_unsafe"
	RUNTIME_PKG_NAME_VAR  = "__xgo_var_runtime"
	UNSAFE_PKG_NAME_VAR   = "__xgo_var_unsafe"

	RUNTIME_PKG_FUNC_INFO_REF = "__xgo_func_info"
	RUNTIME_REGISTER_FUNC     = "Register"
	RUNTIME_FUNC_TYPE         = "Func"
)

const (
	RUNTIME_LINK_FILE = "runtime_link.go"
	VERSION_FILE      = "version.go"
	FLAG_FILE         = "flags.go"
	TRACE_FILE        = "trace.go"
)

const (
	FUNC_INFO = "__xgo_func_info"
	VAR_INFO  = "__xgo_var_info"
	INTF_INFO = "__xgo_intf_info"
)

const (
	XGO_REAL_NOW   = "XgoRealNow"
	XGO_REAL_SLEEP = "XgoRealSleep"
)

const (
	// see https://github.com/xhd2015/xgo/blob/branch-xgo-v1.0/runtime/core/version.go
	// the corresponding commit is 4123ef9cd711daea863cd3cf319989a581debaad
	LATEST_LEGACY_RUNTIME_NUMBER = 324
)
