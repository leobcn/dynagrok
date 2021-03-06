package locavore

import (
	"strconv"
	"strings"
)

import (
	"github.com/timtadh/getopt"
)

import (
	"github.com/timtadh/dynagrok/cmd"
)

func NewCommand(c *cmd.Config) cmd.Runnable {
	return cmd.Cmd(
		"locavore",
		`[options] <failing-profiles> <succeeding-profiles>`,
		`

<failing-profiles> should be a file containing object profiles from
                   failed executions of an instrumented copy of the program
                   under test (PUT).

<succeeding-profiles> should be a file containing object profiles from
                      successful executions of an instrumented copy of the
                      program under test (PUT).

Option Flags
    -h,--help                         Show this message
    -o,--output=<path>                Output file to create (defaults to localized.json)
`,
		"o:b:",
		[]string{
			"output=",
			"numbins=",
		},
		func(r cmd.Runnable, args []string, optargs []getopt.OptArg) ([]string, *cmd.Error) {
			output := "localized.json"
			binstring := "10"
			for _, oa := range optargs {
				switch oa.Opt() {
				case "-o", "--output":
					output = oa.Arg()
				case "-b", "--numbins":
					binstring = oa.Arg()
				}
			}
			_ = output
			var binum int
			if bins, err := strconv.Atoi(binstring); err != nil {
				return nil, cmd.Errorf(2, "Expected int argument for --numbins, received: [%v]", binstring)
			} else {
				binum = bins
			}
			if len(args) != 2 {
				return nil, cmd.Usage(r, 2, "Expected exactly 2 arguments for successful/failing test profiles got: [%v]", strings.Join(args, ", "))
			}
			failFile, failClose, err := cmd.Input(args[0])
			if err != nil {
				return nil, cmd.Errorf(2, "Could not read profiles from failed executions: %v\n%v", args[0], err)
			}
			defer failClose()
			okFile, okClose, err := cmd.Input(args[1])
			if err != nil {
				return nil, cmd.Errorf(2, "Could not read profiles from successful executions: %v\n%v", args[1], err)
			}
			defer okClose()
			types, ok, fail := ParseProfiles(okFile, failFile)
			Localize(ok, fail, types, binum)
			return nil, nil
		})
}
