package instrument

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/types"
	"go/token"
	"strconv"
)

import (
	"github.com/timtadh/data-structures/errors"
	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/loader"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

import (
	"github.com/timtadh/dynagrok/dgruntime/excludes"
)

type instrumenter struct {
	program *loader.Program
	ssa *ssa.Program
	entry string
}

func buildSSA(program *loader.Program) *ssa.Program {
	sp := ssautil.CreateProgram(program, ssa.GlobalDebug)
	sp.Build()
	return sp
}

func Instrument(entryPkgName string, program *loader.Program) (err error) {
	entry := program.Package(entryPkgName)
	if entry == nil {
		return errors.Errorf("The entry package was not found in the loaded program")
	}
	if entry.Pkg.Name() != "main" {
		return errors.Errorf("The entry package was not main")
	}
	i := &instrumenter{
		program: program,
		ssa: buildSSA(program),
		entry: entryPkgName,
	}
	return i.instrument()
}


func (i *instrumenter) instrument() (err error) {
	for _, pkg := range i.program.AllPackages {
		if len(pkg.BuildPackage.CgoFiles) > 0 {
			continue
		}
		if excludes.ExcludedPkg(pkg.Pkg.Path()) {
			continue
		}
		for _, fileAst := range pkg.Files {
			hadFunc := false
			err = Functions(fileAst, func(fn ast.Node, parent *ast.FuncDecl, count int) error {
				hadFunc = true
				switch x := fn.(type) {
				case *ast.FuncDecl:
					if x.Body == nil {
						return nil
					}
					fnName := FuncName(pkg.Pkg, pkg.Info.TypeOf(x.Name).(*types.Signature), x)
					return i.fnBody(pkg, fnName, fn, &x.Body.List)
				case *ast.FuncLit:
					if x.Body == nil {
						return nil
					}
					parentName := pkg.Pkg.Path()
					if parent != nil {
						parentType := pkg.Info.TypeOf(parent.Name)
						if parentType != nil {
							parentName = FuncName(pkg.Pkg, parentType.(*types.Signature), parent)
						}
					}
					fnName := fmt.Sprintf("%v$%d", parentName, count)
					return i.fnBody(pkg, fnName, fn, &x.Body.List)
				default:
					return errors.Errorf("unexpected type %T", x)
				}
			})
			if err != nil {
				return err
			}
			if hadFunc {
				astutil.AddImport(i.program.Fset, fileAst, "dgruntime")
			}
		}
	}
	return nil
}

func (i *instrumenter) fnBody(pkg *loader.PackageInfo, fnName string, fnAst ast.Node, fnBody *[]ast.Stmt) error {
	if true {
		err := Blocks(fnBody, nil, func(blk *[]ast.Stmt, id int) error {
			var pos token.Pos = fnAst.Pos()
			if len(*blk) > 0 {
				pos = (*blk)[0].Pos()
			}
			if id != 0 {
				*blk = insert(*blk, 0, i.mkEnterBlk(pos, id))
			}
			for j := 0; j < len(*blk) - 1; j++ {
				if j+1 < len(*blk) {
					pos = (*blk)[j+1].Pos()
				} else {
					pos = (*blk)[j].Pos()
				}
				switch stmt := (*blk)[j].(type) {
				case *ast.BranchStmt:
					// *blk = insert(*blk, j, i.mkPrint(pos, fmt.Sprintf("exit-blk:\t %d\t of %v", id, fnName)))
					// j++
				case *ast.IfStmt, *ast.ForStmt, *ast.SelectStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.RangeStmt:
					*blk = insert(*blk, j+1, i.mkRe_enterBlk(pos, id, 2+j+1))
					j++
				case *ast.LabeledStmt:
					switch stmt.Stmt.(type) {
					case *ast.ForStmt, *ast.SwitchStmt, *ast.SelectStmt, *ast.TypeSwitchStmt, *ast.RangeStmt:
						*blk = insert(*blk, j+1, i.mkRe_enterBlk(pos, id, 2+j+1))
					default:
						// errors.Logf("DEBUG", "label stmt %T %T in %v", stmt.Stmt, (*blk)[j+1], fnName)
						*blk = insert(*blk, j+1, stmt.Stmt)
						stmt.Stmt = i.mkRe_enterBlk(pos, id, 2+j)
					}
				}
			}
			for j := 0; j < len(*blk); j++ {
				pos = (*blk)[j].Pos()
				switch stmt := (*blk)[j].(type) {
				default:
					err := Statement(stmt, func(expr ast.Expr) error {
						switch e := expr.(type) {
						case *ast.SelectorExpr:
							if ident, ok := e.X.(*ast.Ident); ok {
								if ident.Name == "os" && e.Sel.Name == "Exit" {
									*blk = insert(*blk, j, i.mkShutdownNow(pos))
									j++
								}
							}
						}
						return nil
					})
					if err != nil {
						return err
					}
				}
			}
			return nil
		})
		if err != nil {
			return nil
		}
	}
	*fnBody = insert(*fnBody, 0, i.mkEnterFunc(fnAst.Pos(), fnName))
	*fnBody = insert(*fnBody, 1, i.mkExitFunc(fnAst.Pos(), fnName))
	if pkg.Pkg.Path() == i.entry && fnName == fmt.Sprintf("%v.main", pkg.Pkg.Path()) {
		*fnBody = insert(*fnBody, 0, i.mkShutdown(fnAst.Pos()))
	}
	return nil
}

func FuncName(pkg *types.Package, fnType *types.Signature, fnAst *ast.FuncDecl) string {
	recv := fnType.Recv()
	recvName := pkg.Path()
	if recv != nil {
		recvName = fmt.Sprintf("(%v)", TypeName(pkg, recv.Type()))
	}
	return fmt.Sprintf("%v.%v", recvName, fnAst.Name.Name)
}

func TypeName(pkg *types.Package, t types.Type) string {
	switch r := t.(type) {
	case *types.Pointer:
		return fmt.Sprintf("*%v", TypeName(pkg, r.Elem()))
	case *types.Named:
		return fmt.Sprintf("%v.%v", pkg.Path(), r.Obj().Name())
	default:
		panic(errors.Errorf("unexpected recv %T", t))
	}
}

func insert(blk []ast.Stmt, j int, stmt ast.Stmt) []ast.Stmt {
	if j > len(blk) {
		j = len(blk)
	}
	if j < 0 {
		j = 0
	}
	if cap(blk) <= len(blk) + 1 {
		nblk := make([]ast.Stmt, len(blk), (cap(blk)+1)*2)
		copy(nblk, blk)
		blk = nblk
	}
	blk = blk[:len(blk)+1]
	for i := len(blk)-1; i > 0; i-- {
		if j == i {
			blk[i] = stmt
			break
		}
		blk[i] = blk[i-1]
	}
	if j == 0 {
		blk[j] = stmt
	}
	return blk
}

func (i *instrumenter) mkPrint(pos token.Pos, data string) ast.Stmt {
	s := fmt.Sprintf("dgruntime.Println(%v)", strconv.Quote(data))
	e, err := parser.ParseExprFrom(i.program.Fset, i.program.Fset.File(pos).Name(), s, parser.Mode(0))
	if err != nil {
		panic(fmt.Errorf("mkPrint (%v) error: %v", s, err))
	}
	return &ast.ExprStmt{e}
}

func (i *instrumenter) mkDeferPrint(pos token.Pos, data string) ast.Stmt {
	s := fmt.Sprintf("func() { dgruntime.Println(%v) }()", strconv.Quote(data))
	e, err := parser.ParseExprFrom(i.program.Fset, i.program.Fset.File(pos).Name(), s, parser.Mode(0))
	if err != nil {
		panic(fmt.Errorf("mkDeferPrint (%v) error: %v", s, err))
	}
	return &ast.DeferStmt{Call: e.(*ast.CallExpr)}
}

func (i *instrumenter) mkEnterFunc(pos token.Pos, name string) ast.Stmt {
	p := i.program.Fset.Position(pos)
	s := fmt.Sprintf("dgruntime.EnterFunc(%v, %v)", strconv.Quote(name), strconv.Quote(p.String()))
	e, err := parser.ParseExprFrom(i.program.Fset, i.program.Fset.File(pos).Name(), s, parser.Mode(0))
	if err != nil {
		panic(fmt.Errorf("mkEnterFunc (%v) error: %v", s, err))
	}
	return &ast.ExprStmt{e}
}

func (i *instrumenter) mkExitFunc(pos token.Pos, name string) ast.Stmt {
	s := fmt.Sprintf("func() { dgruntime.ExitFunc(%v) }()", strconv.Quote(name))
	e, err := parser.ParseExprFrom(i.program.Fset, i.program.Fset.File(pos).Name(), s, parser.Mode(0))
	if err != nil {
		panic(fmt.Errorf("mkExitFunc (%v) error: %v", s, err))
	}
	return &ast.DeferStmt{Call: e.(*ast.CallExpr)}
}

func (i *instrumenter) mkShutdown(pos token.Pos) ast.Stmt {
	s := "func() { dgruntime.Shutdown() }()"
	e, err := parser.ParseExprFrom(i.program.Fset, i.program.Fset.File(pos).Name(), s, parser.Mode(0))
	if err != nil {
		panic(fmt.Errorf("mkShutdown (%v) error: %v", s, err))
	}
	return &ast.DeferStmt{Call: e.(*ast.CallExpr)}
}

func (i *instrumenter) mkShutdownNow(pos token.Pos) ast.Stmt {
	s := "dgruntime.Shutdown()"
	e, err := parser.ParseExprFrom(i.program.Fset, i.program.Fset.File(pos).Name(), s, parser.Mode(0))
	if err != nil {
		panic(fmt.Errorf("mkShutdown (%v) error: %v", s, err))
	}
	return &ast.ExprStmt{e}
}

func (i *instrumenter) mkEnterBlk(pos token.Pos, blkid int) ast.Stmt {
	p := i.program.Fset.Position(pos)
	s := fmt.Sprintf("dgruntime.EnterBlk(%d, %v)", blkid, strconv.Quote(p.String()))
	e, err := parser.ParseExprFrom(i.program.Fset, i.program.Fset.File(pos).Name(), s, parser.Mode(0))
	if err != nil {
		panic(fmt.Errorf("mkEnterBlk (%v) error: %v", s, err))
	}
	return &ast.ExprStmt{e}
}

func (i *instrumenter) mkRe_enterBlk(pos token.Pos, blkid, at int) ast.Stmt {
	p := i.program.Fset.Position(pos)
	s := fmt.Sprintf("dgruntime.Re_enterBlk(%d, %d, %v) // %v", blkid, at, strconv.Quote(p.String()))
	e, err := parser.ParseExprFrom(i.program.Fset, i.program.Fset.File(pos).Name(), s, parser.Mode(0))
	if err != nil {
		panic(fmt.Errorf("mkEnterBlk (%v) error: %v", s, err))
	}
	return &ast.ExprStmt{e}
}

