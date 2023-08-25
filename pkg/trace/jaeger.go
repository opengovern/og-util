package trace

import (
	"fmt"
	"runtime"
)

type TraceName string

const (
	DescribeWorkerTrace = "describe-worker"
)

func GetCurrentFuncName() string {
	pc, _, _, _ := runtime.Caller(1)
	return fmt.Sprintf("%s", runtime.FuncForPC(pc).Name())
}
