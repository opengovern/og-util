package concurrency

import (
	"fmt"
	"os"
	"runtime/debug"
	"time"
)

func EnsureRunGoroutine(f func(), maxTry, currentTry *int) {
	try := 0
	if currentTry != nil {
		try = *currentTry
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("paniced: %v", r)
				fmt.Printf("%s", string(debug.Stack()))
				time.Sleep(1 * time.Second)
				if maxTry != nil && try > *maxTry {
					fmt.Printf("max try reached: %d\n", *maxTry)
					os.Exit(1)
				}
				try++
				fmt.Printf("going to try again, current try: %d\n", try)
				EnsureRunGoroutine(f, maxTry, &try)
			}
		}()

		f()
	}()
}
