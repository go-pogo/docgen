// Copyright (c) 2021, Roel Schut. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docgen

import (
	"go/doc"
	"html"
	"path"
	"strings"
)

type Package struct {
	// Name of package as used with a package statement.
	Name string
	// Path relative to Module.Path, when joined they form ImportPath.
	Path string

	Module   *Module
	Sections []*Section
}

func NewPackage(name, path string, mod *Module) *Package {
	return &Package{
		Name: name,
		Path: path,

		Module:   mod,
		Sections: make([]*Section, 0, 10),
	}
}

func (p *Package) ImportPath() string { return pkgImportPath(p.Path, p.Module) }

func pkgImportPath(pkgPath string, mod *Module) string {
	if mod == nil {
		return pkgPath
	}

	if isEmptyPath(pkgPath) {
		return mod.Path
	} else {
		return path.Join(mod.Path, pkgPath)
	}
}

func (p *Package) Section(i int) *Section { return p.Sections[i] }

func (p *Package) Synopsis() string {
	return doc.Synopsis(p.Sections[0].Blocks[0].String())
}

type Section struct {
	Id      string
	Heading string
	Blocks  []Block
}

func NewSection(id string) *Section {
	return &Section{
		Id:     id,
		Blocks: make([]Block, 0, 16),
	}
}

type Block interface {
	block()
	String() string
}

type ParaBlock string
type PreBlock string

func (b ParaBlock) block() {}
func (b PreBlock) block()  {}

func (b ParaBlock) String() string { return string(b) }
func (b PreBlock) String() string  { return string(b) }

type op int

const (
	opId op = iota
	opHead
	opPara
	opPre
)

type unmarshaler struct {
	pkg  *Package
	cur  *Section
	op   op
	line strings.Builder
}

func (u *unmarshaler) reset(pkg *Package) *unmarshaler {
	u.pkg = pkg
	u.cur = NewSection("")
	u.op = 0
	u.line.Reset()

	return u
}

func (u *unmarshaler) Write(b []byte) (int, error) {
	s := string(b)

	switch true {
	case s == `<h3 id="`:
		u.op = opId

	case s == "<p>\n":
		u.op = opPara

	case s == "</p>\n":
		u.cur.Blocks = append(u.cur.Blocks, ParaBlock(u.line.String()))
		u.line.Reset()

	case s == "<pre>":
		u.op = opPre

	case s == "</pre>\n":
		u.cur.Blocks = append(u.cur.Blocks, PreBlock(u.line.String()))
		u.line.Reset()

	default:
		switch u.op {
		case opId:
			if s == `">` {
				u.op = opHead
				break
			}

			u.pkg.Sections = append(u.pkg.Sections, u.cur)
			u.cur = NewSection(s)

		case opHead:
			if s == "</h3>\n" {
				u.cur.Heading = u.line.String()
				u.line.Reset()
				break
			}
			fallthrough

		case opPara:
			fallthrough

		case opPre:
			u.line.WriteString(html.UnescapeString(s))
		}
	}

	return len(b), nil
}
