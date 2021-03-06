package analysis

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
)

import (
	"github.com/timtadh/data-structures/set"
	ds_types "github.com/timtadh/data-structures/types"
)

type BlockLocation struct {
	Block int // Basic block id
	Stmt  int // Statement Id in the Block
}

type Object struct {
	Id       token.Pos
	Position token.Position
	Ident    *ast.Ident
	Object   types.Object
	Location *BlockLocation
	Redefs   *set.SortedSet
}

func (o *Object) String() string {
	return fmt.Sprintf("%v", o.Ident.Name)
}

type Reference struct {
	Id       int
	Position token.Position
	Ident    *ast.Ident
	Oid      token.Pos // object location - for non-local objects
	Obj      *Object
}

func (r *Reference) String() string {
	return fmt.Sprintf("%v:%v", r.Id, r.Obj)
}

type Definitions struct {
	cfg  *CFG
	info *types.Info
	objs map[token.Pos]*Object
	refs map[token.Pos]*Reference
}

type ReachingDefinitions struct {
	Definitions
	in  map[BlockLocation]*set.SortedSet // reaching inputs (as def Id)
	out map[BlockLocation]*set.SortedSet // reaching outputs (as def Id)
}

func FindDefinitions(cfg *CFG, info *types.Info) *Definitions {
	d := &Definitions{
		cfg:  cfg,
		objs: make(map[token.Pos]*Object),
		refs: make(map[token.Pos]*Reference),
		info: info,
	}
	decl := func(bid, sid int, e *ast.Ident, obj types.Object) {
		object := &Object{
			Id:       obj.Pos(),
			Position: cfg.FSet.Position(obj.Pos()),
			Ident:    e,
			Object:   obj,
			Location: &BlockLocation{
				Block: bid,
				Stmt:  sid,
			},
			Redefs: set.NewSortedSet(10),
		}
		ref := &Reference{
			Id:       int(e.Pos()),
			Position: cfg.FSet.Position(e.Pos()),
			Ident:    e,
			Obj:      object,
			Oid:      obj.Pos(),
		}
		d.objs[object.Id] = object
		d.refs[token.Pos(ref.Id)] = ref
		object.Redefs.Add(ds_types.Int(ref.Id))
	}
	param := func(fields *ast.FieldList) {
		if fields == nil {
			return
		}
		for _, field := range fields.List {
			for _, name := range field.Names {
				obj := info.Defs[name]
				decl(-1, -1, name, obj)
			}
		}
	}
	param(cfg.Receiver)
	param(cfg.Type.Params)
	param(cfg.Type.Results)
	for _, blk := range cfg.Blocks {
		for sid, stmt := range blk.Stmts {
			blkExprs(*stmt, func(expr ast.Expr) {
				switch e := expr.(type) {
				case *ast.Ident:
					if obj := info.Defs[e]; obj != nil {
						// this is a definition
						fmt.Fprintln(os.Stderr, "def", FmtNode(cfg.FSet, e), ":", obj.Id(), obj.Pos())
						decl(blk.Id, sid, e, obj)
					} else if obj := info.Uses[e]; obj != nil {
						fmt.Fprintln(os.Stderr, "use", FmtNode(cfg.FSet, e), ":", obj.Id(), obj.Pos())
						object := d.objs[obj.Pos()]
						ref := &Reference{
							Id:       int(e.Pos()),
							Position: cfg.FSet.Position(e.Pos()),
							Ident:    e,
							Obj:      object,
							Oid:      obj.Pos(),
						}
						d.refs[token.Pos(ref.Id)] = ref
					}
				}
			})
		}
		add := func(e *ast.Ident) {
			ref := d.refs[e.Pos()]
			if ref == nil {
				var obj types.Object = nil
				var oid token.Pos = 0
				var object *Object = nil
				if o := info.Defs[e]; o != nil {
					obj = o
					oid = obj.Pos()
					object = d.objs[oid]
				} else if o := info.Uses[e]; o != nil {
					obj = o
					oid = obj.Pos()
					object = d.objs[oid]
				}
				ref = &Reference{
					Id:       int(e.Pos()),
					Position: cfg.FSet.Position(e.Pos()),
					Ident:    e,
					Obj:      object,
					Oid:      oid,
				}
				d.refs[token.Pos(ref.Id)] = ref
			}
			obj := ref.Obj
			if obj != nil {
				obj.Redefs.Add(ds_types.Int(ref.Id))
			}
		}
		for _, stmt := range blk.Stmts {
			switch s := (*stmt).(type) {
			case *ast.IncDecStmt:
				switch e := s.X.(type) {
				case *ast.Ident:
					add(e)
				}
			case *ast.AssignStmt:
				for _, expr := range s.Lhs {
					switch e := expr.(type) {
					case *ast.Ident:
						add(e)
					}
				}
			}
		}
	}
	return d
}

func (d *Definitions) Objects() map[token.Pos]*Object {
	return d.objs
}

func (d *Definitions) References() map[token.Pos]*Reference {
	return d.refs
}

func (d *Definitions) ReachingDefinitions() *ReachingDefinitions {
	rd := &ReachingDefinitions{
		Definitions: *d,
	}
	rd.in, rd.out = ForwardSolveSets(d.cfg, rd.Flow)
	return rd
}

func (rd *ReachingDefinitions) In(loc *BlockLocation) []*Reference {
	in := make([]*Reference, 0, rd.in[*loc].Size())
	for x, next := rd.in[*loc].Items()(); next != nil; x, next = next() {
		in = append(in, rd.refs[token.Pos(x.(ds_types.Int))])
	}
	return in
}

func (rd *ReachingDefinitions) Out(loc *BlockLocation) []*Reference {
	out := make([]*Reference, 0, rd.out[*loc].Size())
	for x, next := rd.out[*loc].Items()(); next != nil; x, next = next() {
		out = append(out, rd.refs[token.Pos(x.(ds_types.Int))])
	}
	return out
}

func (rd *ReachingDefinitions) Flow(loc *BlockLocation, in *set.SortedSet) (out *set.SortedSet) {
	gen, kill := rd.GenKill(loc)
	x, err := in.Subtract(kill)
	if err != nil {
		panic(err)
	}
	o, err := gen.Union(x)
	if err != nil {
		panic(err)
	}
	return o.(*set.SortedSet)
}

func (rd *ReachingDefinitions) GenKill(loc *BlockLocation) (gen, kill *set.SortedSet) {
	proc := func(e *ast.Ident) {
		if rd.info.Uses[e] == nil && rd.info.Defs[e] == nil {
			return
		}
		ref := rd.refs[e.Pos()]
		if ref.Obj == nil {
			return
		}
		gen.Add(ds_types.Int(ref.Id))
		for redef, next := ref.Obj.Redefs.Items()(); next != nil; redef, next = next() {
			if int(redef.(ds_types.Int)) != ref.Id {
				kill.Add(redef)
			}
		}
	}
	gen = set.NewSortedSet(len(rd.objs))
	kill = set.NewSortedSet(len(rd.objs))
	if loc.Block >= 0 && loc.Block < len(rd.cfg.Blocks) {
		blk := rd.cfg.Blocks[loc.Block]
		if loc.Stmt >= 0 && loc.Stmt < len(blk.Stmts) {
			stmt := blk.Stmts[loc.Stmt]
			switch s := (*stmt).(type) {
			case *ast.IncDecStmt:
				switch e := s.X.(type) {
				case *ast.Ident:
					proc(e)
				}
			case *ast.AssignStmt:
				for _, expr := range s.Lhs {
					switch e := expr.(type) {
					case *ast.Ident:
						proc(e)
					}
				}
			}
		}
	} else if loc.Block < 0 {
		param := func(fields *ast.FieldList) {
			if fields == nil {
				return
			}
			for _, field := range fields.List {
				for _, name := range field.Names {
					proc(name)
				}
			}
		}
		param(rd.cfg.Receiver)
		param(rd.cfg.Type.Params)
		param(rd.cfg.Type.Results)
	}
	return gen, kill
}

func ForwardSolveSets(cfg *CFG, flow func(*BlockLocation, *set.SortedSet) *set.SortedSet) (in, out map[BlockLocation]*set.SortedSet) {
	lastLocation := func(blk *Block) BlockLocation {
		return BlockLocation{blk.Id, len(blk.Stmts) - 1}
	}
	in = make(map[BlockLocation]*set.SortedSet)
	out = make(map[BlockLocation]*set.SortedSet)
	stack := make([]BlockLocation, 0, 10)
	for bid := len(cfg.Blocks) - 1; bid >= 0; bid-- {
		for sid := len(cfg.Blocks[bid].Stmts) - 1; sid >= 0; sid-- {
			loc := BlockLocation{bid, sid}
			in[loc] = set.NewSortedSet(10)
			out[loc] = set.NewSortedSet(10)
			stack = append(stack, loc)
		}
	}
	{
		loc := BlockLocation{-1, -1} // function entry location
		in[loc] = set.NewSortedSet(10)
		out[loc] = set.NewSortedSet(10)
		stack = append(stack, loc)
	}
	for len(stack) > 0 {
		var cur BlockLocation
		stack, cur = stack[:len(stack)-1], stack[len(stack)-1]
		var blk *Block = nil
		if cur.Block >= 0 && cur.Block < len(cfg.Blocks) {
			blk = cfg.Blocks[cur.Block]
		}
		input := set.NewSortedSet(10)
		if blk != nil && cur.Stmt == 0 {
			if len(blk.Prev) == 0 {
				prev := BlockLocation{-1, -1}
				for x, next := out[prev].Items()(); next != nil; x, next = next() {
					input.Add(x)
				}
			}
			for _, f := range blk.Prev {
				prev := lastLocation(f.Block)
				for x, next := out[prev].Items()(); next != nil; x, next = next() {
					input.Add(x)
				}
			}
		} else if blk != nil {
			prev := BlockLocation{cur.Block, cur.Stmt - 1}
			for x, next := out[prev].Items()(); next != nil; x, next = next() {
				input.Add(x)
			}
		}
		in[cur] = input
		res := flow(&cur, input)
		fmt.Fprintln(os.Stderr, cfg.Name, cur, res, out[cur])
		if out[cur] == nil || !res.Equals(out[cur]) {
			out[cur] = res
			if blk != nil && cur.Stmt+1 < len(blk.Stmts) {
				next := BlockLocation{blk.Id, cur.Stmt + 1}
				stack = append(stack, next)
			} else if blk != nil {
				for _, n := range blk.Next {
					next := BlockLocation{n.Block.Id, 0}
					stack = append(stack, next)
				}
			} else {
				next := BlockLocation{0, 0}
				stack = append(stack, next)
			}
		}
	}
	return in, out
}
