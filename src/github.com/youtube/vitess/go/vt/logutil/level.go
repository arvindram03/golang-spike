package logutil

import (
	"flag"
)

func init() {
	threshold := flag.Lookup("stderrthreshold")
	if threshold == nil {
		panic("the logging module doesn't specify a stderrthreshold flag")
	}
	if err := threshold.Value.Set("WARNING"); err != nil {
		panic(err)
	}
	threshold.DefValue = "WARNING"
}
