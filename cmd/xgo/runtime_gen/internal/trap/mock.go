package trap

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/xhd2015/xgo/runtime/core"
	"github.com/xhd2015/xgo/runtime/functab"
)

// ErrNotInstrumented is the error returned when a function or variable
// is not instrumented by xgo.
// when this error happened, user should check if `--trap` was correctly
// passed to the xgo compiler
// for variable, only variables within main module are available,
// and the variable should be declared with type specified:
//
//	var SomeVar int = 10 // good
//	var SomeVar = 10 // bad: xgo cannot generate trap function for untyped variables
//
// and when used as method receiver, should wrap the variable in type conversion:
//
//	MyInt(SomeVar).String() // good
//	SomeVar.String() // bad: xgo cannot determine if it is a pointer receiver
//
// check https://github.com/xhd2015/xgo/tree/master/doc/ERR_NOT_INSTRUMENTED.md for more details
var ErrNotInstrumented = errors.New("not instrumented by xgo, see https://github.com/xhd2015/xgo/tree/master/doc/ERR_NOT_INSTRUMENTED.md")

type mockHolder struct {
	wantRecvPtr interface{}
	mock        func(fnInfo *core.FuncInfo, recvPtr interface{}, args []interface{}, results []interface{}) bool
}

type varMockHolder struct {
	mock func(fnInfo *core.FuncInfo, res interface{})
}

func PushMockInterceptor(fn interface{}, interceptor Interceptor) func() {
	return pushMockInterceptor(fn, interceptor)
}

func PushMockReplacer(fn interface{}, replacer interface{}) func() {
	return pushMockReplacer(fn, replacer)
}

func PushMockReplacerByName(pkgPath string, funcName string, replacer interface{}) func() {
	return pushMockReplacerByName(pkgPath, funcName, replacer)
}

func PushMockReplacerMethodByName(instance interface{}, method string, replacer interface{}) func() {
	return pushMockReplacerMethodByName(instance, method, replacer)
}

func PushMockByName(pkgPath string, funcName string, interceptor Interceptor) func() {
	recvPtr, _, _, trappingPC := getFuncByName(pkgPath, funcName)
	handler := buildMockFromInterceptor(recvPtr, interceptor)
	return pushMockHandler(trappingPC, recvPtr, handler)
}

func PushMockMethodByName(instance interface{}, method string, interceptor Interceptor) func() {
	recvPtr, _, _, trappingPC := getMethodByName(instance, method)
	handler := buildMockFromInterceptor(recvPtr, interceptor)
	return pushMockHandler(trappingPC, recvPtr, handler)
}

func pushMockInterceptor(fn interface{}, interceptor Interceptor) func() {
	fnv := reflect.ValueOf(fn)
	if fnv.Kind() == reflect.Ptr {
		varPtr := fnv.Pointer()
		funcInfo := functab.InfoVarAddr(varPtr)
		if funcInfo == nil {
			panic(fmt.Errorf("variable %w: %v", ErrNotInstrumented, varPtr))
		}
		// variable
		handler := func(fnInfo *core.FuncInfo, res interface{}) {
			var argObj object
			resObject := object{
				{
					name:   fnInfo.Name,
					valPtr: res,
				},
			}
			interceptor(nil, fnInfo, argObj, resObject)
		}
		return pushVarMockHandler(varPtr, handler)
	} else if fnv.Kind() == reflect.Func {
		// func
	} else {
		panic(fmt.Errorf("fn should be func or pointer to variable, actual: %T", fn))
	}

	recvPtr, _, _, trappingPC := Inspect(fn)
	handler := buildMockFromInterceptor(recvPtr, interceptor)
	return pushMockHandler(trappingPC, recvPtr, handler)
}

func pushMockReplacer(fn interface{}, replacer interface{}) func() {
	fnv := reflect.ValueOf(fn)
	if fnv.Kind() == reflect.Ptr {
		varPtr := fnv.Pointer()
		funcInfo := functab.InfoVarAddr(varPtr)
		if funcInfo == nil {
			panic(fmt.Errorf("variable %w: %v", ErrNotInstrumented, varPtr))
		}
		// variable
		rv := reflect.ValueOf(replacer)
		isPtr := checkVarType(fnv.Type(), rv.Type(), true)
		handler := func(fnInfo *core.FuncInfo, res interface{}) {
			fnRes := rv.Call([]reflect.Value{})
			reflect.ValueOf(res).Elem().Set(fnRes[0])
		}
		if !isPtr {
			return pushVarMockHandler(varPtr, handler)
		}
		return pushVarPtrMockHandler(varPtr, handler)
	} else if fnv.Kind() == reflect.Func {
		// func
		replacerV := reflect.ValueOf(replacer)
		replacerType := replacerV.Type()
		if replacerType.Kind() != reflect.Func {
			panic(fmt.Errorf("requires func, given %T", replacer))
		}
		if replacerV.IsNil() {
			panic("replacer is nil")
		}
		if replacerType != fnv.Type() {
			panic(fmt.Errorf("replacer should have type: %T, actual: %T", fn, replacer))
		}
	} else {
		panic(fmt.Errorf("fn should be func or pointer to variable, actual: %T", fn))
	}

	recvPtr, funcInfo, _, trappingPC := Inspect(fn)
	handler := buildMockHandler(recvPtr, funcInfo, replacer)
	return pushMockHandler(trappingPC, recvPtr, handler)
}

// pushMockHandler pushes a mock handler to the stack.
// The returned function can be used to pop the mock.
// If the mock is not popped, it will affect even after
// the caller returned.
// `mock` returns `false` if the original function should be called.
func pushMockHandler(pc uintptr, recvPtr interface{}, handler func(fnInfo *core.FuncInfo, recvPtr interface{}, args []interface{}, results []interface{}) bool) func() {
	stackData := getOrAttachStackData()
	if stackData.mock == nil {
		stackData.mock = map[uintptr][]*mockHolder{}
	}
	h := &mockHolder{wantRecvPtr: recvPtr, mock: handler}
	stackData.mock[pc] = append(stackData.mock[pc], h)
	return func() {
		list := stackData.mock[pc]
		n := len(list)
		if list[n-1] == h {
			stackData.mock[pc] = list[:n-1]
			return
		}
		// remove at some index
		for i, m := range list {
			if m == h {
				stackData.mock[pc] = append(list[:i], list[i+1:]...)
				return
			}
		}
		panic(fmt.Errorf("pop mock not found, check if the mock is already popped earlier"))
	}
}

func pushVarMockHandler(varAddr uintptr, mock func(fnInfo *core.FuncInfo, res interface{})) func() {
	stack := getOrAttachStackData()
	if stack.varMock == nil {
		stack.varMock = map[uintptr][]*varMockHolder{}
	}
	h := &varMockHolder{mock: mock}
	stack.varMock[varAddr] = append(stack.varMock[varAddr], h)
	return func() {
		list := stack.varMock[varAddr]
		n := len(list)
		if list[n-1] == h {
			stack.varMock[varAddr] = list[:n-1]
			return
		}
		// remove at some index
		for i, m := range list {
			if m == h {
				stack.varMock[varAddr] = append(list[:i], list[i+1:]...)
				return
			}
		}
		panic(fmt.Errorf("pop mock not found, check if the mock is already popped earlier"))
	}
}

func pushVarPtrMockHandler(varAddr uintptr, mock func(fnInfo *core.FuncInfo, res interface{})) func() {
	stack := getOrAttachStackData()
	if stack.varPtrMock == nil {
		stack.varPtrMock = map[uintptr][]*varMockHolder{}
	}
	h := &varMockHolder{mock: mock}
	stack.varPtrMock[varAddr] = append(stack.varPtrMock[varAddr], h)
	return func() {
		list := stack.varPtrMock[varAddr]
		n := len(list)
		if list[n-1] == h {
			stack.varPtrMock[varAddr] = list[:n-1]
			return
		}
		// remove at some index
		for i, m := range list {
			if m == h {
				stack.varPtrMock[varAddr] = append(list[:i], list[i+1:]...)
				return
			}
		}
		panic(fmt.Errorf("pop mock not found, check if the mock is already popped earlier"))
	}
}

func pushMockReplacerByName(pkgPath string, funcName string, replacer interface{}) func() {
	if replacer == nil {
		panic("replacer cannot be nil")
	}
	t := reflect.TypeOf(replacer)
	if t.Kind() != reflect.Func {
		panic(fmt.Errorf("replacer should be func, actual: %T", replacer))
	}

	// check type
	recvPtr, funcInfo, _, trappingPC := getFuncByName(pkgPath, funcName)
	if funcInfo.Kind == core.Kind_Func {
		if funcInfo.Func != nil {
			calledType, replacerType, match := checkFuncTypeMatch(reflect.TypeOf(funcInfo.Func), t, recvPtr != nil)
			if !match {
				panic(fmt.Errorf("replacer should have type: %s, actual: %s", calledType, replacerType))
			}
		}
	} else if funcInfo.Kind == core.Kind_Var || funcInfo.Kind == core.Kind_VarPtr || funcInfo.Kind == core.Kind_Const {
		varPtrType := reflect.TypeOf(funcInfo.Var)
		var wantValueType reflect.Type
		if funcInfo.Kind == core.Kind_Var {
			wantValueType = reflect.FuncOf(nil, []reflect.Type{varPtrType.Elem()}, false)
		} else {
			// const: type is not pointer
			wantValueType = reflect.FuncOf(nil, []reflect.Type{varPtrType}, false)
		}

		var targetTypeStr string
		var replacerTypeStr string
		var match bool

		var matchPtr bool
		replacerType := reflect.TypeOf(replacer)
		if replacerType.Kind() != reflect.Func {
			targetTypeStr = wantValueType.String()
			replacerTypeStr = replacerType.String()
		} else {
			targetTypeStr, replacerTypeStr, match = checkFuncTypeMatch(wantValueType, replacerType, false)
			if !match && funcInfo.Kind != core.Kind_VarPtr {
				_, replacerTypeStr, match = checkFuncTypeMatch(reflect.FuncOf(nil, []reflect.Type{varPtrType}, false), replacerType, false)
				matchPtr = true
			}
		}
		if !match {
			panic(fmt.Errorf("replacer should have type: %s, actual: %s", targetTypeStr, replacerTypeStr))
		}
		if matchPtr {
			funcInfo = functab.Info(pkgPath, "*"+funcName)
			if funcInfo == nil {
				panic(fmt.Errorf("failed to patch: %s *%s", pkgPath, funcName))
			}
		}
	} else {
		panic(fmt.Errorf("unrecognized func type: %s", funcInfo.Kind.String()))
	}

	handler := buildMockHandler(recvPtr, funcInfo, replacer)
	return pushMockHandler(trappingPC, recvPtr, handler)
}

func pushMockReplacerMethodByName(instance interface{}, method string, replacer interface{}) func() {
	if replacer == nil {
		panic("replacer cannot be nil")
	}
	t := reflect.TypeOf(replacer)
	if t.Kind() != reflect.Func {
		panic(fmt.Errorf("replacer should be func, actual: %T", replacer))
	}

	// check type
	recvPtr, funcInfo, _, trappingPC := getMethodByName(instance, method)
	if funcInfo.Func != nil {
		calledType, replacerType, match := checkFuncTypeMatch(reflect.TypeOf(funcInfo.Func), t, recvPtr != nil)
		if !match {
			panic(fmt.Errorf("replacer should have type: %s, actual: %s", calledType, replacerType))
		}
	}
	handler := buildMockHandler(recvPtr, funcInfo, replacer)
	return pushMockHandler(trappingPC, recvPtr, handler)
}

func getFuncByName(pkgPath string, funcName string) (recvPtr interface{}, fn *core.FuncInfo, funcPC uintptr, trappingPC uintptr) {
	fn = functab.GetFuncByPkg(pkgPath, funcName)
	if fn == nil {
		panic(fmt.Errorf("failed to setup mock for: %s.%s", pkgPath, funcName))
	}
	return nil, fn, fn.PC, fn.PC
}

func getMethodByName(instance interface{}, method string) (recvPtr interface{}, fn *core.FuncInfo, funcPC uintptr, trappingPC uintptr) {
	// extract instance's reflect.Type
	// use that type to query for reflect mapping in functab:
	//    reflectTypeMapping map[reflect.Type]map[string]*funcInfo
	t := reflect.TypeOf(instance)
	funcMapping := functab.GetTypeMethods(t)
	if funcMapping == nil {
		panic(fmt.Errorf("failed to setup mock for type %T", instance))
	}
	fn = funcMapping[method]
	if fn == nil {
		panic(fmt.Errorf("failed to setup mock for: %T.%s", instance, method))
	}

	addr := reflect.New(t)
	addr.Elem().Set(reflect.ValueOf(instance))

	return addr.Interface(), fn, fn.PC, fn.PC
}

func sameReceiver(recvPtr interface{}, actRecvPtr interface{}) bool {
	// assume both are non-nil
	recvPtrVal := reflect.ValueOf(recvPtr)
	actRecvPtrVal := reflect.ValueOf(actRecvPtr)
	return recvPtrVal.Elem().Interface() == actRecvPtrVal.Elem().Interface()
}

// return `true` if hit ptr type
func checkVarType(varPtrType reflect.Type, replacerType reflect.Type, supportPtr bool) bool {
	wantValueType := reflect.FuncOf(nil, []reflect.Type{varPtrType.Elem()}, false)
	if replacerType.Kind() != reflect.Func {
		panic(fmt.Errorf("replacer should have type: %s, actual: %s", wantValueType.String(), replacerType.String()))
	}

	targetTypeStr, replacerTypeStr, match := checkFuncTypeMatch(wantValueType, replacerType, false)
	if match {
		return false
	}
	if supportPtr {
		wantPtrType := reflect.FuncOf(nil, []reflect.Type{varPtrType}, false)
		_, _, matchPtr := checkFuncTypeMatch(wantPtrType, replacerType, false)
		if matchPtr {
			return true
		}
	}
	panic(fmt.Errorf("replacer should have type: %s, actual: %s", targetTypeStr, replacerTypeStr))
}
