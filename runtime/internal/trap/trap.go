package trap

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/xhd2015/xgo/runtime/core"
	"github.com/xhd2015/xgo/runtime/internal/constants"
	"github.com/xhd2015/xgo/runtime/internal/flags"
	xgo_runtime "github.com/xhd2015/xgo/runtime/internal/runtime"
	"github.com/xhd2015/xgo/runtime/internal/stack"
	"github.com/xhd2015/xgo/runtime/trace/stack_model"
)

// skip 2: <user func> -> runtime.XgoTrap -> trap
const SKIP = 2

// trap is the function called upon every go function call,
// it implements mock, recording and tracing functionality
// the trap discarded the interceptor design in xgo v1.0,
// and uses a simpler and more efficient design:
//   - mapping by pc and variable pointer
//
// this avoids the infinite trap problem
func trap(infoPtr unsafe.Pointer, recvPtr interface{}, args []interface{}, results []interface{}) (func(), bool) {
	// === start init ===
	funcInfo := (*core.FuncInfo)(infoPtr)
	recvName := funcInfo.RecvName
	argNames := funcInfo.ArgNames
	resultNames := funcInfo.ResNames

	var begin time.Time

	var pcs [1]uintptr
	runtime.Callers(SKIP+1, pcs[:])
	pc := pcs[0]
	runtimeFuncInfo := runtime.FuncForPC(pc)
	fnPC := runtimeFuncInfo.Entry()

	pkg := funcInfo.Pkg
	name := funcInfo.IdentityName

	var mock func(fnInfo *core.FuncInfo, recvPtr interface{}, args []interface{}, results []interface{}) bool

	var isTesting bool
	var testName string

	var postRecorder func()

	stk := stack.Get()
	if stk == stack.NilGStack {
		return nil, false
	}
	depth := xgo_runtime.GetG().IncTrappingDepth()
	defer xgo_runtime.GetG().DecTrappingDepth()

	stackData := getStackDataOf(stk)
	// === end init ===
	//
	//
	// === start detect trapping and tracing ===
	var isTracing bool
	var isStartTracing bool
	if stackData != nil {
		if stackData.inspecting != nil {
			stackData.inspecting(pc, funcInfo, recvPtr, args, results)
			return nil, true
		}
		begin = xgo_runtime.XgoRealTimeNow()
		isTracing = stackData.hasStartedTracing
	}

	var stackAttached bool
	if depth <= 1 && !isTracing {
		// detect if we need to start tracing
		if pkg == constants.TRACE_PKG && name == constants.TRACE_FUNC {
			isStartTracing = true
			isTracing = true
		} else if stackData == nil {
			// try detect testing
			if flags.COLLECT_TEST_TRACE {
				if recvPtr == nil && len(args) == 1 && len(results) == 0 {
					t, ok := args[0].(**testing.T)
					if ok {
						// detect if we are called from TestX(t *testing.T)
						var pcs [1]uintptr
						runtime.Callers(SKIP+2, pcs[:])
						pc := pcs[0]
						funcInfo := runtime.FuncForPC(pc)

						if funcInfo != nil && funcInfo.Name() == constants.TESTING_RUNNER {
							isTesting = true
							isStartTracing = true
							isTracing = true
							testName = (*t).Name()
						}
					}
				}
			}
		}
		if isStartTracing {
			if stackData == nil {
				// trace starting cannot happen on empty stack
				// stk might be InitGStack
				if stk != nil {
					// this should never happen
					panic("stackData is nil while stk is not nil!")
				}
				stackData = &StackData{
					hasStartedTracing: true,
				}
				begin = xgo_runtime.XgoRealTimeNow()
				stk = &stack.Stack{
					Begin: begin,
					Data: map[interface{}]interface{}{
						dataKey: stackData,
					},
				}
				stack.Attach(stk)
				stackAttached = true
			} else {
				stackData.hasStartedTracing = true
			}
		}
	}
	// === end detect trapping and tracing ===
	//
	//
	// === start check mock and interceptors ===
	wantPtr, mockFn := stackData.getLastMock(fnPC)
	recordHandlers := stackData.getRecordHandlers(fnPC)
	var interceptors []*recorderHolder
	if depth <= 1 {
		// when stack is trapping, we cannot not
		// call into interceptors which are not
		// targeting specific functions, can
		// cause infinite loop.
		// mock and recorders do not have such
		// problem because they explicitly have
		// targeted function
		interceptors = stackData.getGeneralInterceptors()
	}
	if mockFn != nil && (wantPtr == nil || (recvPtr != nil && sameReceiver(recvPtr, wantPtr))) {
		mock = mockFn
	}

	var postRecordersAndInterceptors []func()
	var mocked bool
	for _, h := range recordHandlers {
		if h.wantRecvPtr != nil && (recvPtr == nil || !sameReceiver(recvPtr, h.wantRecvPtr)) {
			continue
		}
		var data interface{}
		var stop bool
		if h.pre != nil {
			data, stop = h.pre(funcInfo, recvPtr, args, results)
			if stop {
				mocked = true
				break
			}
		}
		if h.post != nil {
			postRecordersAndInterceptors = append(postRecordersAndInterceptors, func() {
				h.post(funcInfo, recvPtr, args, results, data)
			})
		}
	}

	if !mocked {
		for _, h := range interceptors {
			var data interface{}
			var stop bool
			if h.pre != nil {
				// TODO: handle abort
				data, stop = h.pre(funcInfo, recvPtr, args, results)
				if stop {
					mocked = true
					break
				}
			}

			if h.post != nil {
				postRecordersAndInterceptors = append(postRecordersAndInterceptors, func() {
					h.post(funcInfo, recvPtr, args, results, data)
				})
			}
		}
	}
	if len(postRecordersAndInterceptors) > 0 {
		if len(postRecordersAndInterceptors) == 1 {
			postRecorder = postRecordersAndInterceptors[0]
		} else {
			postRecorder = func() {
				// reversed
				n := len(postRecordersAndInterceptors)
				for i := n - 1; i >= 0; i-- {
					postRecordersAndInterceptors[i]()
				}
			}
		}
	}

	var callRecorderWithDepth func()
	if postRecorder != nil {
		callRecorderWithDepth = func() {
			xgo_runtime.GetG().IncTrappingDepth()
			defer xgo_runtime.GetG().DecTrappingDepth()
			postRecorder()
		}
	}
	// === end check mock and interceptors ===
	if !mocked {
		if depth > 1 {
			// when stack is trapping, only allow pc-related
			// mock and recorders to run
			// no tracing and general interceptors can run
			if mock != nil {
				ok := mock(funcInfo, recvPtr, args, results)
				// ok=true indicates not call old function
				return callRecorderWithDepth, ok
			}
			return callRecorderWithDepth, false
		}

		if !isTracing {
			// without tracing, mock becomes simpler
			if mock != nil {
				ok := mock(funcInfo, recvPtr, args, results)
				// ok=true indicates not call old function
				return callRecorderWithDepth, ok
			}
			return callRecorderWithDepth, false
		}
	} else {
		if !isTracing {
			return callRecorderWithDepth, true
		}
	}

	// === init tracing stacks ===
	//
	//
	// === tracing records ===
	file, line := runtimeFuncInfo.FileLine(pc)
	cur := stk.NewEntry(begin, name)
	oldTop := stk.Push(cur)
	cur.File = file
	cur.Line = line
	cur.FuncInfo = funcInfo

	if isStartTracing && !isTesting {
		var onFinish func(stack stack_model.IStack)
		var outputFile string
		var config interface{}
		for i, arg := range args {
			if argNames[i] == "config" {
				config = arg
				break
			}
		}
		if config != nil {
			rvalue := reflect.ValueOf(config)
			if rvalue.Kind() == reflect.Ptr {
				rvalue = rvalue.Elem()
			}
			if rvalue.IsValid() && rvalue.Kind() == reflect.Struct {
				outputFileField := rvalue.FieldByName("OutputFile")
				if outputFileField.IsValid() {
					file, ok := outputFileField.Interface().(string)
					if ok {
						outputFile = file
					}
				}
				onFinishField := rvalue.FieldByName("OnFinish")
				if onFinishField.IsValid() {
					f, ok := onFinishField.Interface().(func(stack stack_model.IStack))
					if ok {
						onFinish = f
					}
				}
			}
		}
		if outputFile == "" && onFinish == nil {
			if stackAttached {
				stack.Detach()
			}
			return postRecorder, false
		}
		stackData.stackOutputFile = outputFile
		stackData.onFinish = onFinish
	}

	// fmt.Fprintf(os.Stderr, "%sargs: %s\n", prefix, string(argsJSON))
	argNamesNoCtx, argsNoCtx := tryRemoveFirstCtx(argNames, args)
	marshalNames := argNamesNoCtx
	marshalArgs := argsNoCtx
	if recvPtr != nil {
		marshalNames = make([]string, 1+len(argNamesNoCtx))
		marshalArgs = make([]interface{}, 1+len(argsNoCtx))
		marshalNames[0] = recvName
		marshalArgs[0] = recvPtr
		copy(marshalNames[1:], argNamesNoCtx)
		copy(marshalArgs[1:], argsNoCtx)
	}
	cur.Args = json.RawMessage(xgo_runtime.MarshalNoError(newStructValue(marshalNames, marshalArgs)))
	stk.Depth++

	var hitMock bool
	post := func() {
		xgo_runtime.GetG().IncTrappingDepth()
		defer xgo_runtime.GetG().DecTrappingDepth()

		// NOTE: this defer might be executed on system stack
		// so cannot defer
		if postRecorder != nil {
			postRecorder()
		}
		// on Windows, short stack might resolve to same
		// nanosecond
		// see https://github.com/xhd2015/xgo/issues/307
		// so we add a standalone flag `Finished`
		end := xgo_runtime.XgoRealTimeNow()
		if isStartTracing {
			stk.End = end
		}
		cur.EndNs = end.UnixNano() - stk.Begin.UnixNano()
		cur.Finished = true
		cur.HitMock = hitMock
		var hasPanic bool
		if pe, retpc := xgo_runtime.XgoPeekPanic(); pe != nil {
			hasPanic = true
			cur.Panic = true
			// frame:
			//   0: trap.trap
			//   1: runtime.gopanic
			//   2: <func1>
			//   3: <func2> -- may have recover()
			//   ...
			fnPC := funcInfo.PC
			if fnPC == 0 {
				fnPC = retpc
			}
			retEntryPC := runtime.FuncForPC(fnPC).Entry()

			// we look back at most 10 frames
			// to find which ret pc matches the
			// frame, and us that pc as line
			//
			// when panic inside a deferred func,
			// which does not belong to the
			// caller itself, this reports the
			// last line that sees the panic,
			// not the inside
			// example:
			//    func SomeFunc(){
			//      defer func(){
			//         panic("panic in defer")  <-- this does not belong to SomeFunc
			//      }()
			//      doThingsPanic()  <-- this reported
			//    }
			callerPCs := make([]uintptr, 10)
			npc := runtime.Callers(2, callerPCs)
			callerPCs = callerPCs[:npc]
			// check which pc matches
			for _, pc := range callerPCs {
				entryPC := runtime.FuncForPC(pc).Entry()
				if entryPC == retEntryPC {
					frame, _ := runtime.CallersFrames([]uintptr{pc}).Next()
					cur.PanicLine = frame.Line
					break
				}
			}
			cur.Error = fmt.Sprint(pe)
		}

		resultNamesNoErr, resultsNoErr, resErr := trySplitLastError(resultNames, results)
		cur.Results = json.RawMessage(xgo_runtime.MarshalNoError(newStructValue(resultNamesNoErr, resultsNoErr)))
		if !hasPanic && resErr != nil {
			cur.Error = resErr.Error()
		}

		stk.Top = oldTop
		stk.Depth--
		if isStartTracing {
			exportedStack := stack.Export(stk, 0)
			exportedStackJSON := xgo_runtime.MarshalNoError(exportedStack)
			if isTesting {
				outputFile := filepath.Join(flags.COLLECT_TEST_TRACE_DIR, testName+".json")
				os.MkdirAll(filepath.Dir(outputFile), 0755)
				err := os.WriteFile(outputFile, exportedStackJSON, 0644)
				if err != nil {
					fmt.Fprintf(os.Stderr, "error writing stack: %v\n", err)
				}
			} else {
				if stackData.onFinish != nil {
					stackData.onFinish(&StackDataExportImpl{
						data: exportedStack,
						json: exportedStackJSON,
					})
				}
				if stackData.stackOutputFile != "" {
					err := os.WriteFile(stackData.stackOutputFile, exportedStackJSON, 0644)
					if err != nil {
						fmt.Fprintf(os.Stderr, "error writing stack: %v\n", err)
					}
				}
			}
			stack.Detach()
		}
	}
	if mocked {
		hitMock = true
		return post, true
	} else if mock != nil {
		hitMock = mock(funcInfo, recvPtr, args, results)
		return post, hitMock
	}
	return post, false
}

type StackDataExportImpl struct {
	data *stack_model.Stack
	json []byte
}

func (c *StackDataExportImpl) Data() *stack_model.Stack {
	return c.data
}

func (c *StackDataExportImpl) JSON() ([]byte, error) {
	return c.json, nil
}
