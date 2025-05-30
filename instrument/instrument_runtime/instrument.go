package instrument_runtime

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/xhd2015/xgo/instrument/constants"
	"github.com/xhd2015/xgo/instrument/instrument_runtime/instrument_testing"
	"github.com/xhd2015/xgo/instrument/instrument_runtime/template"
	"github.com/xhd2015/xgo/instrument/patch"
	"github.com/xhd2015/xgo/support/goinfo"
)

var instrumentMarkPath = patch.FilePath{"xgo_trap_instrument_mark.txt"}
var procPath = patch.FilePath{"src", "runtime", "proc.go"}

var jsonEncodingPath = patch.FilePath{"src", "encoding", "json", "encode.go"}

type InstrumentMode int

const (
	InstrumentMode_UseMark InstrumentMode = iota
	InstrumentMode_Force
	InstrumentMode_ForceAndIgnoreMark
)

type InstrumentRuntimeOptions struct {
	Mode                  InstrumentMode
	InstrumentVersionMark string
}

// only support go1.19 for now
func InstrumentRuntime(goroot string, goVersion *goinfo.GoVersion, xgoTrapTemplate string, opts InstrumentRuntimeOptions) error {
	srcDir := filepath.Join(goroot, "src")

	srcStat, statErr := os.Stat(srcDir)
	if statErr != nil {
		if !os.IsNotExist(statErr) {
			return statErr
		}
		return fmt.Errorf("GOROOT/src does not exist, please use newer go distribution which contains src code for runtime: %w", statErr)
	}
	if !srcStat.IsDir() {
		return fmt.Errorf("GOROOT/src is not a directory, please use newer go distribution: %s", srcDir)
	}

	instrumentMark := opts.InstrumentVersionMark
	if instrumentMark == "" {
		instrumentMark = "v0.0.1"
	}

	markFile := instrumentMarkPath.JoinPrefix(goroot)
	if opts.Mode == InstrumentMode_UseMark {
		markContent, statErr := os.ReadFile(markFile)
		if statErr != nil {
			if !os.IsNotExist(statErr) {
				return statErr
			}
		}
		if string(markContent) == instrumentMark {
			return nil
		}
	}

	err := instrumentRuntime2(goroot, goVersion.Major, goVersion.Minor)
	if err != nil {
		return fmt.Errorf("instrument runtime2: %w", err)
	}

	err = instrumentProc(goroot, goVersion)
	if err != nil {
		return fmt.Errorf("instrument proc: %w", err)
	}

	err = instrumentTimeNow(goroot, goVersion.Major, goVersion.Minor)
	if err != nil {
		return fmt.Errorf("instrument time: %w", err)
	}

	err = instrumentTimeSleep(goroot, goVersion.Major, goVersion.Minor)
	if err != nil {
		return fmt.Errorf("instrument time sleep: %w", err)
	}

	err = instrumentRuntimeTimeSleep(goroot, goVersion.Major, goVersion.Minor)
	if err != nil {
		return fmt.Errorf("instrument runtime time sleep: %w", err)
	}

	err = instrumentJsonEncoding(goroot, goVersion.Major, goVersion.Minor)
	if err != nil {
		return fmt.Errorf("instrument json encoding: %w", err)
	}

	err = instrument_testing.Instrument(goroot, goVersion)
	if err != nil {
		return fmt.Errorf("instrument testing: %w", err)
	}

	// instrument xgo_trap.go
	xgoTrapContent, err := template.InstantiateXgoTrap(xgoTrapTemplate, goVersion)
	if err != nil {
		return err
	}
	xgoTrapFile := constants.GetGoRuntimeXgoTrapFile(goroot)
	err = os.WriteFile(xgoTrapFile, []byte(xgoTrapContent), 0644)
	if err != nil {
		return err
	}

	if opts.Mode != InstrumentMode_ForceAndIgnoreMark {
		err := os.WriteFile(markFile, []byte(instrumentMark), 0644)
		if err != nil {
			return err
		}
	}
	return nil
}
