package instrument_xgo_runtime

import (
	"errors"
	"fmt"

	"github.com/xhd2015/xgo/instrument/constants"
	"github.com/xhd2015/xgo/instrument/edit"
	"github.com/xhd2015/xgo/instrument/instrument_func"
	"github.com/xhd2015/xgo/instrument/load"
	"github.com/xhd2015/xgo/instrument/overlay"
	"github.com/xhd2015/xgo/support/goinfo"
)

var ErrLinkFileNotFound = errors.New("xgo: link file not found")
var ErrLinkFileNotRequired = errors.New("xgo: link file not required")
var ErrRuntimeVersionDeprecatedV1_0_0 = errors.New("runtime version deprecated")

func LinkXgoRuntime(projectDir string, xgoRuntimeModuleDir string, overlayFS overlay.Overlay, mod string, modfile string, xgoVersion string, xgoRevision string, xgoNumber int, collectTestTrace bool, collectTestTraceDir string, overrideContent func(absFile overlay.AbsFile, content string)) (*edit.Packages, error) {
	opts := load.LoadOptions{
		Dir:     projectDir,
		Overlay: overlayFS,
		Mod:     mod,
		ModFile: modfile,
	}
	if xgoRuntimeModuleDir != "" {
		// xgo runtime is replaced in a separate module
		// so we need to load packages from the separate module
		opts = load.LoadOptions{
			Dir: xgoRuntimeModuleDir,
		}
	}
	packages, err := load.LoadPackages([]string{
		constants.RUNTIME_INTERNAL_RUNTIME_PKG,
		constants.RUNTIME_CORE_PKG,
		constants.RUNTIME_TRAP_FLAGS_PKG,
		constants.RUNTIME_FUNC_INFO_PKG,
		constants.RUNTIME_MOCK_PKG,
		constants.RUNTIME_TRACE_PKG,
		constants.RUNTIME_TRAP_PKG,
	}, opts)
	if err != nil {
		// TODO: handle the case where error indicates the package is not found
		return nil, err
	}
	editPackages := edit.Edit(packages)
	var foundLink bool
	var foundMock bool
	var foundTrace bool
	var foundTrap bool
	for _, pkg := range editPackages.Packages {
		goPkg := pkg.LoadPackage.GoPackage
		if goPkg.Incomplete {
			continue
		}
		importPath := goPkg.ImportPath
		suffixPkg, ok := goinfo.PkgWithinModule(importPath, constants.RUNTIME_MODULE)
		if !ok {
			continue
		}
		n := len(constants.RUNTIME_MODULE) + 1
		switch suffixPkg {
		case constants.RUNTIME_MOCK_PKG[n:]:
			foundMock = true
		case constants.RUNTIME_TRACE_PKG[n:]:
			foundTrace = true
		case constants.RUNTIME_TRAP_PKG[n:]:
			foundTrap = true
		}
		if suffixPkg == constants.RUNTIME_FUNC_INFO_PKG[n:] ||
			suffixPkg == constants.RUNTIME_MOCK_PKG[n:] ||
			suffixPkg == constants.RUNTIME_TRAP_PKG[n:] {
			// only for lookup
			continue
		}
		for _, file := range pkg.Files {
			loadFile := file.File
			content := loadFile.Content
			absFile := overlay.AbsFile(loadFile.AbsPath)
			var funcInfos []*edit.FuncInfo
			switch loadFile.Name {
			case constants.RUNTIME_LINK_FILE:
				if suffixPkg == constants.RUNTIME_INTERNAL_RUNTIME_PKG[n:] {
					foundLink = true
					overrideContent(absFile, GetLinkRuntimeCode())
				}
			case constants.VERSION_FILE:
				if suffixPkg == constants.RUNTIME_CORE_PKG[n:] {
					coreVersion, err := ParseCoreVersion(content)
					if err != nil {
						return nil, err
					}
					if isDeprecatedCoreVersion(coreVersion) {
						return nil, fmt.Errorf("%w: %s", ErrRuntimeVersionDeprecatedV1_0_0, coreVersion)
					}
					versionContent := ReplaceActualXgoVersion(content, xgoVersion, xgoRevision, xgoNumber)
					overrideContent(absFile, versionContent)
				}
			case constants.FLAG_FILE:
				if suffixPkg == constants.RUNTIME_TRAP_FLAGS_PKG[n:] && collectTestTrace {
					flagsContent := InjectFlags(content, collectTestTrace, collectTestTraceDir)
					overrideContent(absFile, flagsContent)
				}
			case constants.TRACE_FILE:
				if suffixPkg == constants.RUNTIME_TRACE_PKG[n:] {
					edit := file.Edit
					funcInfos = instrument_func.InjectRuntimeTrap(edit, importPath, loadFile.Syntax, file.Index)
					if edit.HasEdit() {
						overrideContent(absFile, edit.Buffer().String())
					}
				}
			}
			file.TrapFuncs = funcInfos
		}
	}
	// found any usage of xgo public API, but does not found
	// link file, it means the runtime is not instrumented
	if !foundLink {
		if foundMock || foundTrace || foundTrap {
			return editPackages, ErrLinkFileNotFound
		}
		return editPackages, ErrLinkFileNotRequired
	}
	return editPackages, nil
}
