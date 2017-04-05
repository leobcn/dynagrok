package mine

import (
	"strconv"
)

import (
	"github.com/timtadh/getopt"
)

import (
	"github.com/timtadh/dynagrok/cmd"
)

func NewLeapParser(c *cmd.Config, o *Options) cmd.Runnable {
	return cmd.Cmd(
		"leap",
		`[options]`,
		`
Option Flags
    -h,--help                         Show this message
    -k,--top-k=<int>                  Number of graphs to find
    -s,--sigma=<int>                  The leap factor for leaping of sigma similar branches
    --debug=<int>                     Debug level
`,
		"k:s:",
		[]string{
			"top-k=",
			"sigma=",
			"debug=",
		},
		func(r cmd.Runnable, args []string, optargs []getopt.OptArg) ([]string, *cmd.Error) {
			debug := 0
			topk := 10
			sigma := .01
			for _, oa := range optargs {
				switch oa.Opt() {
				case "--debug":
					d, err := strconv.Atoi(oa.Arg())
					if err != nil {
						return nil, cmd.Errorf(1, "Could not parse arg to `%v` expected an int (got %v). err: %v", oa.Opt(), oa.Arg(), err)
					}
					debug = d
				case "-k", "--top-k":
					k, err := strconv.Atoi(oa.Arg())
					if err != nil {
						return nil, cmd.Errorf(1, "Could not parse arg to `%v` expected an int (got %v). err: %v", oa.Opt(), oa.Arg(), err)
					}
					topk = k
				case "-s", "--sigma":
					s, err := strconv.ParseFloat(oa.Arg(), 64)
					if err != nil {
						return nil, cmd.Errorf(1, "Could not parse arg to `%v` expected an float (got %v). err: %v", oa.Opt(), oa.Arg(), err)
					}
					sigma = s
				}
			}
			o.Miner = LEAP(topk, sigma, debug).Mine
			o.MinerName += "LEAP"
			return args, nil
		})
}
