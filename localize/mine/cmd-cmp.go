package mine

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"time"

	"github.com/timtadh/dynagrok/cmd"
	"github.com/timtadh/getopt"
)

// TODO(tim):
//
// Thread timeouts through all algorithms [done]
// Provide FP-filtering analysis. (maybe????)
// Provide Fault Localization Accuracy Report [done]
// Compare top-k, top-k maximal for branch-and-bound, sLeap, LEAP [done]

// https://math.stackexchange.com/questions/75968/expectation-of-number-of-trials-before-success-in-an-urn-problem-without-replace

func algorithmParser(c *cmd.Config) func(o *Options, args []string) (*Options, []string, *cmd.Error) {
	var wo walkOpts
	return func(o *Options, args []string) (*Options, []string, *cmd.Error) {
		bb := NewBranchAndBoundParser(c, o)
		sleap := NewSLeapParser(c, o)
		leap := NewLeapParser(c, o)
		urw := NewURWParser(c, o, &wo)
		swrw := NewSWRWParser(c, o, &wo)
		walks := NewWalksParser(c, o, &wo)
		topColors := NewWalkTopColorsParser(c, o, &wo)
		walkTypes := cmd.Commands(map[string]cmd.Runnable{
			walks.Name():     walks,
			topColors.Name(): topColors,
		})
		options := map[string]cmd.Runnable{
			bb.Name():    bb,
			sleap.Name(): sleap,
			leap.Name():  leap,
			urw.Name():   cmd.Concat(urw, walkTypes),
			swrw.Name():  cmd.Concat(swrw, walkTypes),
		}
		command := cmd.Commands(options)
		if len(args) <= 0 {
			return nil, nil, nil
		}
		if _, has := options[args[0]]; !has {
			return nil, args, nil
		}
		leftover, err := command.Run(args)
		if err != nil {
			return nil, nil, err
		}
		return o, leftover, nil
	}
}

func NewCompareParser(c *cmd.Config, o *Options) cmd.Runnable {
	parser := algorithmParser(c)
	return cmd.Concat(
		cmd.Cmd(
			"compare",
			`[options]`,
			`
Compare a walk based method against leap, s-leap, or branch and bound.

Option Flags
    -h,--help                         Show this message
    -t,--time-out=<seconds>           Timeout for each algorithm (default 120 seconds)
    -f,--faults=<path>                Path to a fault file.
    -o,--output=<path>                Place to write CSV of evaluation
`,
			"o:f:t:",
			[]string{
				"output=",
				"faults=",
				"time-out=",
			},
			func(r cmd.Runnable, args []string, optargs []getopt.OptArg) ([]string, *cmd.Error) {
				outputPath := ""
				faultsPath := ""
				timeout := 120 * time.Second
				for _, oa := range optargs {
					switch oa.Opt() {
					case "-o", "--output":
						outputPath = oa.Arg()
					case "-f", "--faults":
						faultsPath = oa.Arg()
					case "-t", "--time-out":
						t, err := time.ParseDuration(oa.Arg())
						if err != nil {
							return nil, cmd.Errorf(1, "For flag %v expected a duration got %v. err: %v", oa.Opt, oa.Arg(), err)
						}
						timeout = t
					}
				}
				if faultsPath == "" {
					return nil, cmd.Errorf(1, "You must supply the `-f` flag and give a path to the faults")
				}
				faults, err := LoadFaults(faultsPath)
				if err != nil {
					return nil, cmd.Err(1, err)
				}
				for _, f := range faults {
					fmt.Println(f)
				}
				ouf := os.Stdout
				if outputPath != "" {
					f, err := os.Create(outputPath)
					if err != nil {
						return nil, cmd.Err(1, err)
					}
					defer f.Close()
					ouf = f
				}
				opts := make([]*Options, 0, 10)
				for {
					var opt *Options
					var err *cmd.Error
					opt, args, err = parser(o.Copy(), args)
					if err != nil {
						return nil, err
					}
					if opt == nil {
						break
					}
					opts = append(opts, opt)
				}
				min := func(a, b int) int {
					if a < b {
						return a
					}
					return b
				}
				timeit := func(m *Miner) ([]*SearchNode, time.Duration) {
					ctx, cancel := context.WithTimeout(context.Background(), timeout)
					defer cancel()
					s := time.Now()
					nodes := m.Mine(ctx).Unique()
					e := time.Now()
					return nodes, e.Sub(s)
				}
				rankScore := func(nodes []*SearchNode) (int, float64) {
					gid := -1
					min := -1.0
					for _, f := range faults {
						sum := 0.0
						for i, g := range group(nodes) {
							count := 0
							for _, n := range g {
								for _, v := range n.Node.SubGraph.V {
									b, fn, _ := o.Lattice.Info.Get(v.Color)
									if fn == f.FnName && b == f.BasicBlockId {
										count++
										break
									}
								}
							}
							if count > 0 {
								r := float64(len(g) - count)
								b := float64(count)
								score := ((b + r + 1) / (b + 1)) + sum
								if min <= 0 || score < min {
									min = score
									gid = i
								}
							}
							sum += float64(len(g))
						}
					}
					if min <= 0 {
						return -1, math.Inf(1)
					}
					return gid, min
				}
				sum := func(nodes []*SearchNode) float64 {
					sum := 0.0
					for _, n := range nodes {
						sum += n.Score
					}
					return sum
				}
				mean := func(nodes []*SearchNode) float64 {
					return sum(nodes) / float64(len(nodes))
				}
				stddev := func(nodes []*SearchNode) float64 {
					u := mean(nodes)
					variance := 0.0
					for _, n := range nodes {
						variance += (n.Score - u) * (n.Score - u)
					}
					if len(nodes) > 2 {
						variance = (1 / (float64(len(nodes)) - 1)) * variance
					} else {
						variance = (1 / float64(len(nodes))) * variance
					}
					return math.Sqrt(variance)
				}
				stderr := func(X, Y []*SearchNode) float64 {
					T := min(len(X), len(Y))
					variance := 0.0
					for i := 0; i < T; i++ {
						variance += (X[i].Score - Y[i].Score) * (X[i].Score - Y[i].Score)
					}
					if T > 2 {
						variance = (1 / (float64(T) - 1)) * variance
					} else {
						variance = (1 / float64(T)) * variance
					}
					if sum(X[:T]) >= sum(Y[:T]) {
						return math.Sqrt(variance)
					} else {
						return -math.Sqrt(variance)
					}
				}
				statsHeader := func() {
					fmt.Fprintf(ouf,
						"%9v, %9v, %3v, %-27v, %10v, %10v, %10v, %11v, %11v, %11v, %11v, %11v, %11v\n",
						"max-edges", "min-fails", "row", "name", "sum", "mean", "stddev", "stderr (0)", "stderr (1)",
						"rank-group", "rank-score", "dur (sec)", "duration")
				}
				stats := func(maxEdges, minFails, row int, name string, minout int, base1, base2, nodes []*SearchNode, dur time.Duration) {
					clamp := nodes[:minout]
					gid, score := rankScore(nodes)
					fmt.Fprintf(ouf,
						"%9v, %9v, %3v, %-27v, %10.5g, %10.5g, %10.5g, %11.5g, %11.5g, %11v, %11.5g, %11.5g, %11v\n",
						maxEdges, minFails, row, name,
						sum(clamp), mean(clamp), stddev(clamp), stderr(base1, nodes), stderr(base2, nodes), gid, score,
						dur.Seconds(), dur)
				}
				output := func(name string, nodes []*SearchNode) {
					fmt.Println()
					fmt.Println(name)
					for i, n := range nodes {
						fmt.Printf("  - subgraph %-5d %v\n", i, n)
						fmt.Println()
					}
					fmt.Println()
				}
				maxEdges := 0
				minFails := 0
				minout := -1
				outputs := make([][]*SearchNode, 0, len(opts))
				times := make([]time.Duration, 0, len(opts))
				for _, opt := range opts {
					a := NewMiner(opt.Miner, opt.Lattice, opt.Score, opt.Opts...)
					A, aTime := timeit(a)
					outputs = append(outputs, A)
					times = append(times, aTime)
					if minout < 0 || len(A) < minout {
						minout = len(A)
					}
					maxEdges = a.MaxEdges
					minFails = a.MinFails
				}
				for i := range outputs {
					output(opts[i].MinerName, outputs[i][:minout])
				}
				statsHeader()
				for i := range outputs {
					stats(maxEdges, minFails, i, opts[i].MinerName, minout, outputs[0], outputs[1], outputs[i], times[i])
				}
				fmt.Println()
				return args, nil
			}),
	)
}

func (nodes SearchNodes) Unique() (unique []*SearchNode) {
	added := make(map[string]bool)
	for n, next := nodes(); next != nil; n, next = next() {
		if n.Node.SubGraph == nil {
			continue
		}
		label := string(n.Node.SubGraph.Label())
		if added[label] {
			continue
		}
		added[label] = true
		unique = append(unique, n)
	}
	sort.Slice(unique, func(i, j int) bool {
		return unique[i].Score > unique[j].Score
	})
	return unique
}

func (nodes SearchNodes) Group() [][]*SearchNode {
	return group(nodes.Unique())
}

func group(unique []*SearchNode) [][]*SearchNode {
	groups := make([][]*SearchNode, 0, 10)
	for _, n := range unique {
		lg := len(groups)
		if lg > 0 && n.Score == groups[lg-1][0].Score {
			groups[lg-1] = append(groups[lg-1], n)
		} else {
			groups = append(groups, make([]*SearchNode, 0, 10))
			groups[lg] = append(groups[lg], n)
		}
	}
	return groups
}
