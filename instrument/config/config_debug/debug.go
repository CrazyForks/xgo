package config_debug

import (
	"bytes"
	"go/ast"
	"go/printer"
	"go/token"

	"github.com/xhd2015/xgo/instrument/edit"
)

func Debugpoint() {}

// to debug, add `--debug-xgo` and `-tags dev`:
//
//	go run ./script/run-test --include go1.24.1 -tags dev -run TestFuncTab -v --debug-xgo ./runtime/test/functab

func OnTraverseFile(pkg *edit.Package, file *edit.File) {
	if file.File.Name == "x509.go" {
		Debugpoint()
	}
}

func OnCollectFileDecl(pkg *edit.Package, file *edit.File) {
	if file.File.Name == "type_alias_go_1.20_test.go" {
		Debugpoint()
	}
}

func OnCollectVarRef(fileName string, varName string) {
	if fileName == "math_expr_test.go" && varName == "C" {
		Debugpoint()
	}
}

func OnTraverseFuncDecl(pkg *edit.Package, file *edit.File, fnDecl *ast.FuncDecl) {
	var funcName string
	if fnDecl.Name != nil {
		funcName = fnDecl.Name.Name
	}
	if pkg.LoadPackage.GoPackage.ImportPath == "crypto/x509" {
		if file.File.Name == "x509.go" {
			if recvNoName(fnDecl.Recv) && fnDecl.Name != nil && fnDecl.Name.Name == "Error" {
				Debugpoint()
			}
		}
	}
	if funcName == "TestDebugOpAppend" {
		Debugpoint()
	}
}

func OnTrapFunc(pkgPath string, fnDecl *ast.FuncDecl, identityName string) {
	if identityName == "(*ConstraintViolationError).Error" {
		Debugpoint()
	}
}

func recvNoName(recv *ast.FieldList) bool {
	return recv != nil && len(recv.List) == 1 && len(recv.List[0].Names) == 0
}

func DebugExprStr(expr ast.Expr) string {
	var buf bytes.Buffer
	fset := token.NewFileSet()
	err := printer.Fprint(&buf, fset, expr)
	if err != nil {
		return "Error: " + err.Error()
	}
	return buf.String() // Outputs: fmt.Println(x)
}

func DebugExprStrOld(expr ast.Expr) string {
	switch expr := expr.(type) {
	case *ast.Ident:
		return expr.Name
	case *ast.SelectorExpr:
		return DebugExprStr(expr.X) + "." + expr.Sel.Name
	case *ast.ParenExpr:
		return "(" + DebugExprStr(expr.X) + ")"
	case *ast.UnaryExpr:
	}
	return ""
}

func AfterSelectorResolve(expr ast.Expr) {
	if DebugExprStr(expr) == "reader2222.Reader" {
		Debugpoint()
	}
}

func OnRewriteVarDefAndRefs(pkgPath string, file *edit.File, decl *edit.Decl) {
	fileName := file.File.Name
	var declName string
	if decl.Ident != nil {
		declName = decl.Ident.Name
	}
	if fileName == "mypackage.go" {
		Debugpoint()
	}

	if fileName == "type_ref_multiple_times_test.go" {
		if declName == "testMap" {
			Debugpoint()
		}
	}
	if fileName == "tree.go" {
		if declName == "Tree" {
			Debugpoint()
		}
	}
	if fileName == "math_expr_test.go" {
		if declName == "C" {
			Debugpoint()
		}
	}
	if fileName == "mock_var_generic.go" {
		if declName == "instance" {
			Debugpoint()
		}
	}
	switch fileName {
	case "mock_var_no_type_test.go":
		Debugpoint()
	}
}

// go run ./script/run-test --include go1.24.2 -tags=dev --log-debug --debug-xgo ./runtime/test/patch/patch_var/math_expr/
func OnResolveInfo(pkgPath string, fileName string, expr ast.Expr) {
	if pkgPath == "time" && DebugExprStr(expr) == "Second" {
		Debugpoint()
	}
	if fileName == "math_expr_test.go" && DebugExprStr(expr) == "C" {
		Debugpoint()
	}
	if pkgPath == "github.com/xhd2015/xgo/runtime/test/mock/mock_var/mock_var_generic" {
		exprDebug := DebugExprStr(expr)
		// println(exprDebug)
		if exprDebug == "&instance" {
			Debugpoint()
		}
	}
	exprDebug := DebugExprStr(expr)
	switch exprDebug {
	case "third.MustBuild[int](1)":
		Debugpoint()
	case "Wrapper[Concrete, Concrete2]":
		Debugpoint()
	case "MyTasker":
		Debugpoint()
	case "tom.Name":
		Debugpoint()
	}
}

func OnResolvePackageDeclareInfo(pkgPath string, fileName string, expr ast.Expr) {
	if fileName == "math_expr_test.go" && DebugExprStr(expr) == "D" {
		Debugpoint()
	}
}
