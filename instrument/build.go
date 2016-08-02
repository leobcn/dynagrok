package instrument

import (
	"os"
	"os/exec"
	"io"
	"io/ioutil"
	"path/filepath"
	"fmt"
	"go/ast"
	"go/printer"
	"go/build"
	"go/types"
	"strings"
)

import (
	"github.com/timtadh/data-structures/errors"
	"golang.org/x/tools/go/loader"
)

import (
	"github.com/timtadh/dynagrok/cmd"
)

// var config = printer.Config{Mode: printer.UseSpaces | printer.TabIndent | printer.SourcePos, Tabwidth: 8}
var config = printer.Config{Tabwidth: 8}


type binaryBuilder struct {
	config *cmd.Config
	buildContext *build.Context
	program *loader.Program
	entry string
	work string
	output string
}


func BuildBinary(c *cmd.Config, keepWork bool, work, entryPkgName, output string, program *loader.Program) (err error) {
	if work == "" {
		work, err = ioutil.TempDir("", fmt.Sprintf("dynagrok-build-%v-", filepath.Base(entryPkgName)))
		if err != nil {
			return err
		}
	}
	if !keepWork {
		defer os.RemoveAll(work)
	}
	errors.Logf("INFO", "work-dir %v", work)
	b := &binaryBuilder{
		config: c,
		buildContext: cmd.BuildContext(c),
		program: program,
		entry: entryPkgName,
		work: work,
		output: output,
	}
	return b.Build()
}


func (b *binaryBuilder) basePaths() paths {
	basePaths := make([]string, 0, 10)
	basePaths = append(basePaths, b.buildContext.GOROOT)
	paths := strings.Split(b.buildContext.GOPATH, ":")
	for _, path := range paths {
		if path != "" {
			basePaths = append(basePaths, path)
		}
	}
	return basePaths
}

type paths []string

func (ps paths) TrimPrefix(s string) string {
	for _, path := range ps {
		if strings.HasPrefix(s, path) {
			return strings.TrimPrefix(strings.TrimPrefix(s, path), "/")
		}
	}
	return s
}

func (ps paths) PrefixedBy(s string) string {
	for _, path := range ps {
		if strings.HasPrefix(s, path) {
			return path
		}
	}
	panic("unreachable")
}

func (b *binaryBuilder) Build() error {
	basePaths := b.basePaths()
	for pkgType, pkgInfo := range b.program.AllPackages {
		if err := b.createDir(basePaths, pkgType, pkgInfo.Files); err != nil {
			return err
		}
		if len(pkgInfo.BuildPackage.CgoFiles) > 0 {
			continue
		}
		for _, f := range pkgInfo.Files {
			to := filepath.Join(b.work, basePaths.TrimPrefix(b.program.Fset.File(f.Pos()).Name()))
			fout, err := os.Create(to)
			if err != nil {
				return err
			}
			err = config.Fprint(fout, b.program.Fset, f)
			fout.Close()
			if err != nil {
				return errors.Errorf("Could not serialize tree at %v tree %v error: %v", to, f, err)
			}
		}
	}
	return b.goBuild()
}

func (b *binaryBuilder) goBuild() error {
	c := exec.Command("go", "build", "-o", b.output, b.entry)
	env := make([]string, 0, len(os.Environ()))
	for _, item := range os.Environ() {
		if strings.HasPrefix(item, "GOPATH=") {
			continue
		}
		env = append(env, item)
	}
	c.Env = append(env, fmt.Sprintf("GOPATH=%v", b.work))
	output, err := c.CombinedOutput()
	fmt.Fprintln(os.Stderr, c.Path, strings.Join(c.Args[1:], " "))
	fmt.Fprintln(os.Stderr, string(output))
	return err
}

func (b *binaryBuilder) createDir(basePaths paths, pkg *types.Package, pkgFiles []*ast.File) error {
	path := filepath.Join(b.work, "src", pkg.Path())
	err := os.MkdirAll(path, os.ModeDir|os.ModeTemporary|0775)
	if err != nil {
		return err
	}
	var srcPath string
	for _, path := range basePaths {
		if _, err := os.Stat(filepath.Join(path, "src", pkg.Path())); err == nil {
			srcPath = filepath.Join(path, "src")
			break
		}
	}
	srcDir, err := os.Open(filepath.Join(srcPath, pkg.Path()))
	if err != nil {
		return err
	}
	files, err := srcDir.Readdir(0)
	srcDir.Close()
	if err != nil {
		return err
	}
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		name := f.Name()
		from, err := os.Open(filepath.Join(srcPath, pkg.Path(), name))
		if err != nil {
			return err
		}
		to, err := os.Create(filepath.Join(b.work, "src", pkg.Path(), name))
		if err != nil {
			from.Close()
			return err
		}
		_, err = io.Copy(to, from)
		from.Close()
		to.Close()
		if err != nil {
			return err
		}
	}
	return nil

}

