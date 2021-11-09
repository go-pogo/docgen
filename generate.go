// Copyright (c) 2021, Roel Schut. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docgen

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"text/template"

	"github.com/go-pogo/errors"
)

func Must(gen *Generator, err error) *Generator {
	if err != nil {
		panic(err)
	}
	return gen
}

type Generator struct {
	scanner

	tmpl   *template.Template
	filter ScannerFilter
}

func New(rootDir string) *Generator {
	if isEmptyPath(rootDir) {
		rootDir = cwd(1)
	} else if !filepath.IsAbs(rootDir) {
		rootDir = filepath.Join(cwd(1), rootDir)
	}

	return &Generator{
		scanner: scanner{
			startDir: rootDir,
		},
		filter: ScannerFilterFunc(ScanFilter),
	}
}

func (g *Generator) Root() string { return g.startDir }

// AbsPath returns an absolute path from Root.
func (g *Generator) AbsPath(path string) string {
	if isEmptyPath(path) {
		path = g.startDir
	} else if !filepath.IsAbs(path) {
		path = filepath.Join(g.startDir, path)
	} else {
		path = filepath.Clean(path)
	}
	return path
}

func (g *Generator) ScanDir(dir string, mode ScanMode, filter ScannerFilter) (*Generator, error) {
	if mode == ScanNone {
		return g, nil
	}
	if filter == nil {
		filter = g.filter
	}

	g.scanner.init()
	return g, g.scanner.scanDir(g.AbsPath(dir), mode, filter)
}

func (g *Generator) template(path, name string) (*template.Template, error) {
	if g.tmpl != nil {
		if t := g.tmpl.Lookup(name); t != nil {
			return t, nil
		}

		res, err := g.tmpl.ParseFiles(path)
		return res, errors.Trace(err)
	}

	res, err := template.ParseFiles(path)
	if err != nil {
		return nil, errors.Trace(err)
	}

	g.tmpl = res
	return res, nil

}

func (g *Generator) Generate(tmplFile string, wr io.Writer) (*Generator, error) {
	if g.scanner.init() {
		if err := g.scanDir(g.startDir, ScanAllDeep, g.filter); err != nil {
			return g, err
		}
	}

	t, err := g.template(g.AbsPath(tmplFile), tmplFile)
	if err != nil {
		return g, err
	}

	return g, errors.Trace(t.Execute(wr, &printer{
		pkgs: g.Packages,
	}))
}

func (g *Generator) GenerateFile(template, file string, perm os.FileMode) (*Generator, error) {
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return g, errors.Trace(err)
	}

	return g.Generate(template, f)
}

func cwd(skipCaller int) string {
	_, f, _, _ := runtime.Caller(skipCaller + 1)
	return filepath.Dir(f)
}

func isEmptyPath(p string) bool {
	return p == "" || p == "." || p == "./"
}
