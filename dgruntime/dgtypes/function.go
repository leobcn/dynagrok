package dgtypes

import "time"

type Function struct {
	Name   string
	FuncPc uintptr
	CFG    [][]int
	IPDom  []int
	Calls  int
	DynCDP []map[int]bool // Dynamic Control Dependence Predecessors
}

type ExportFunction struct {
	CFG    [][]int
	IPDom  []int
	Calls  int
	DynCDP [][]int // Dynamic Control Dependence Predecessors
}

type FuncCall struct {
	Name     string
	FuncPc   uintptr
	CFG      [][]int
	IPDom    []int
	CDStack  []int
	DynCDP   []map[int]bool // Dynamic Control Dependence Predecessors
	Last     BlkEntrance
	LastTime time.Time
}

func ExportFunctions(funcs map[uintptr]*Function) map[string]*ExportFunction {
	export := make(map[string]*ExportFunction, len(funcs))
	for _, fn := range funcs {
		dcdp := make([][]int, len(fn.DynCDP))
		for x, preds := range fn.DynCDP {
			dcdp[x] = make([]int, 0, len(preds))
			for y := range preds {
				dcdp[x] = append(dcdp[x], y)
			}
		}
		export[fn.Name] = &ExportFunction{
			CFG:    fn.CFG,
			IPDom:  fn.IPDom,
			Calls:  fn.Calls,
			DynCDP: dcdp,
		}
	}
	return export
}

func NewFunction(fc *FuncCall) *Function {
	f := &Function{
		Name:   fc.Name,
		FuncPc: fc.FuncPc,
		CFG:    fc.CFG,
		IPDom:  fc.IPDom,
		DynCDP: fc.DynCDP,
	}
	f.Update(fc)
	return f
}

func (f *Function) Merge(b *Function) {
	if f.FuncPc != b.FuncPc || f.Name != b.Name {
		panic("can't merge")
	}
	f.Calls += b.Calls
	for x, preds := range b.DynCDP {
		for pred := range preds {
			f.DynCDP[x][pred] = true
		}
	}
}

func (f *Function) Update(fc *FuncCall) {
	if f.FuncPc != fc.FuncPc || f.Name != fc.Name {
		panic("f not valid for fc")
	}
	f.Calls += 1
	for x, preds := range fc.DynCDP {
		for pred := range preds {
			f.DynCDP[x][pred] = true
		}
	}
}
