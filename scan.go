// Copyright (c) 2021, Roel Schut. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docgen

import (
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-pogo/errors"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
)

type ScanMode uint8

func (s ScanMode) has(f ScanMode) bool { return s&f != 0 }

//goland:noinspection GoUnusedConst
const (
	ScanModule ScanMode = 1 << iota
	ScanPackages
	ScanDeep

	ScanNone         ScanMode = 0
	ScanAll                   = ScanModule | ScanPackages
	ScanAllDeep               = ScanModule | ScanPackages | ScanDeep
	ScanPackagesDeep          = ScanPackages | ScanDeep
)

type Module struct {
	module.Version
	Deps     []module.Version
	FilePath string
}

func NewModule(ver module.Version) *Module {
	return &Module{
		Version: ver,
		Deps:    make([]module.Version, 0, 6),
	}
}

func NewModuleFromModfile(file *modfile.File) *Module {
	mod := NewModule(file.Module.Mod)
	for _, req := range file.Require {
		mod.Deps = append(mod.Deps, req.Mod)
	}
	return mod
}

type ScannerFilter interface {
	FilterScan(path, name string, entry fs.DirEntry) bool
}

type ScannerFilterFunc func(path, name string, entry fs.DirEntry) bool

func (f ScannerFilterFunc) FilterScan(path, name string, entry fs.DirEntry) bool {
	return f(path, name, entry)
}

// ScanFilter is the default ScannerFilterFunc used in ScanDir. It returns
// false when name starts with a dot or equals "internal".
func ScanFilter(_, name string, _ fs.DirEntry) bool {
	return name[0] != '.' && name != "internal"
}

// ScanDir scans for modules and packages according to mode. When mode has flag
// ScanDeep, subdirectories are also scanned if ScannerFilter filter returns
// true for the subdirectory.
// When filter is nil, ScanFilter is used as a ScannerFilterFunc.
func ScanDir(path string, mode ScanMode, filter ScannerFilter) ([]*Module, []*Package, error) {
	if mode == ScanNone {
		return nil, nil, nil
	}
	if filter == nil {
		filter = ScannerFilterFunc(ScanFilter)
	}

	s := &scanner{
		startDir: path,
		Modules:  make([]*Module, 0, 2),
		Packages: make([]*Package, 0, 6),
	}
	return s.Modules, s.Packages, s.scanDir(path, mode, filter)
}

type scanner struct {
	startDir string
	Modules  []*Module
	Packages []*Package
}

func (s *scanner) init() bool {
	if s.Modules != nil || s.Packages != nil {
		return false
	}

	s.Modules = make([]*Module, 0, 2)
	s.Packages = make([]*Package, 0, 6)
	return true
}

func (s *scanner) scanDir(dir string, mode ScanMode, filter ScannerFilter) error {
	var mod *Module
	if mode.has(ScanModule) {
		var err error
		if mod, err = s.scanModule(dir); err != nil {
			return err
		}
	}

	if mode.has(ScanPackages) {
		var path string
		if mod == nil && strings.HasPrefix(dir, s.startDir) {
			// strip workdir to get submodule path
			path = filepath.ToSlash(dir[len(s.startDir)+1:])
		}

		if err := s.scanPackages(dir, path, mod); err != nil {
			return err
		}
	}

	if mode.has(ScanDeep) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}

		var scanErr error
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			name := entry.Name()
			path := filepath.Join(dir, name)
			if !filter.FilterScan(path, name, entry) {
				continue
			}

			errors.Append(
				&scanErr,
				s.scanDir(path, mode, filter),
			)
		}
		if scanErr != nil {
			return scanErr
		}
	}
	return nil
}

// scanModule tries to make a new Module from a possible go.mod file located in
// the directory dir.
func (s *scanner) scanModule(dir string) (*Module, error) {
	path := filepath.Join(dir, "go.mod")

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Trace(err)
	}

	f, err := modfile.ParseLax(path, data, nil)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		} else {
			return nil, errors.Trace(err)
		}
	}

	mod := NewModuleFromModfile(f)
	mod.FilePath = path
	s.Modules = append(s.Modules, mod)

	return mod, nil
}

// scanPackages parses the directory dir for any packages.
func (s *scanner) scanPackages(dir, path string, mod *Module) error {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, nil, parser.ParseComments)
	if err != nil {
		return errors.Trace(err)
	}

	var uw unmarshaler
	for _, p := range pkgs {
		// convert map[string]*File to []*ast.File
		files := make([]*ast.File, 0, len(p.Files))
		for _, f := range p.Files {
			files = append(files, f)
		}

		importPath := p.Name
		if mod != nil {
			importPath = pkgImportPath(path, mod)
		}

		d, err := doc.NewFromFiles(fset, files, importPath)
		if err != nil {
			return errors.Trace(err)
		}

		pkg := NewPackage(d.Name, path, mod)
		doc.ToHTML(uw.reset(pkg), d.Doc, nil)
		s.Packages = append(s.Packages, pkg)
	}

	return nil
}
