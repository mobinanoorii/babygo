package main

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"strconv"
	"strings"
)

func isGlobalVar(obj *ast.Object) bool {
	return getObjData(obj) == -1
}

func setObjData(obj *ast.Object, i int) {
	obj.Data = i
}

func getObjData(obj *ast.Object) int {
	objData, ok := obj.Data.(int)
	if !ok {
		panic("obj.Data is not int")
	}
	return objData
}

func emitVariable(obj *ast.Object) {
	// precondition
	if obj.Kind != ast.Var {
		panic("obj should be ast.Var")
	}
	fmt.Printf("  # obj=%#v\n", obj)

	var typ ast.Expr
	var localOffset int
	switch dcl := obj.Decl.(type) {
	case *ast.ValueSpec:
		typ = dcl.Type
		localOffset = getObjData(obj) * -1
	case *ast.Field:
		typ = dcl.Type
		localOffset = getObjData(obj) // param offset
		fmt.Printf("  # dcl.Names[0].Obj=%#v\n", dcl.Names[0].Obj)
		fmt.Printf("  # param offset=%d\n", localOffset)
	default:
		panic("unexpected")
	}

	fmt.Printf("  # emitVariable\n")
	fmt.Printf("  # Type=%#v\n", typ)
	fmt.Printf("  # obj.Data=%d\n", obj.Data)

	if getPrimType(typ) == gString {
		if isGlobalVar(obj) {
			fmt.Printf("  # global\n")
			fmt.Printf("  movq %s+0(%%rip), %%rax\n", obj.Name)
			fmt.Printf("  movq %s+8(%%rip), %%rcx\n", obj.Name)
		} else {
			fmt.Printf("  # local\n")
			fmt.Printf("  movq %d(%%rbp), %%rax # ptr %s \n", localOffset, obj.Name)
			fmt.Printf("  movq %d(%%rbp), %%rcx # len %s \n", (localOffset + 8), obj.Name)
		}
		fmt.Printf("  pushq %%rax # ptr\n")
		fmt.Printf("  pushq %%rcx # len\n")
	} else if getPrimType(typ) == gInt {
		if isGlobalVar(obj) {
			fmt.Printf("  # global\n")
			fmt.Printf("  movq %s+0(%%rip), %%rax\n", obj.Name)
		} else {
			fmt.Printf("  # local\n")
			fmt.Printf("  movq %d(%%rbp), %%rax # %s \n", localOffset, obj.Name)
		}
		fmt.Printf("  pushq %%rax # int val\n")
	} else {
		panic("Unexpected type")
	}
}

func emitVariableAddr(obj *ast.Object) {
	// precondition
	if obj.Kind != ast.Var {
		panic("obj should be ast.Var")
	}
	decl,ok := obj.Decl.(*ast.ValueSpec)
	if !ok {
		panic("Unexpected case")
	}

	fmt.Printf("  # emitVariable\n")
	fmt.Printf("  # obj.Data=%d\n", obj.Data)

	if getPrimType(decl.Type) == gString {
		if isGlobalVar(obj) {
			fmt.Printf("  # global\n")
			fmt.Printf("  leaq %s+0(%%rip), %%rax # ptr\n", obj.Name)
			fmt.Printf("  leaq %s+8(%%rip), %%rcx # len\n", obj.Name)
		} else {
			fmt.Printf("  # local\n")
			localOffset := (getObjData(obj))
			fmt.Printf("  leaq -%d(%%rbp), %%rax # ptr %s \n", localOffset, obj.Name)
			fmt.Printf("  leaq -%d(%%rbp), %%rcx # len %s \n", localOffset - 8, obj.Name)
		}
		fmt.Printf("  pushq %%rax # ptr\n")
		fmt.Printf("  pushq %%rcx # len\n")
	} else if getPrimType(decl.Type) == gInt {
		if isGlobalVar(obj) {
			fmt.Printf("  # global\n")
			fmt.Printf("  leaq %s+0(%%rip), %%rax\n", obj.Name)
		} else {
			fmt.Printf("  # local\n")
			localOffset := (getObjData(obj))
			fmt.Printf("  leaq -%d(%%rbp), %%rax # %s \n", localOffset, obj.Name)
		}
		fmt.Printf("  pushq %%rax # int var\n")
	} else {
		panic("Unexpected type")
	}
}

func emitAddr(expr ast.Expr) {
	switch e := expr.(type) {
	case *ast.Ident:
		fmt.Printf("  # ident kind=%v\n", e.Obj.Kind)
		fmt.Printf("  # Obj=%v\n", e.Obj)
		if e.Obj.Kind == ast.Var {
			emitVariableAddr(e.Obj)
		} else {
			panic("Unexpected ident kind")
		}
	}
}

func emitExpr(expr ast.Expr) {
	switch e := expr.(type) {
	case *ast.Ident:
		fmt.Printf("  # ident kind=%v\n", e.Obj.Kind)
		fmt.Printf("  # Obj=%v\n", e.Obj)
		if e.Obj.Kind == ast.Var {
			emitVariable(e.Obj)
		} else {
			panic("Unexpected ident kind")
		}
	case *ast.CallExpr:
		fun := e.Fun
		fmt.Printf("  # funcall=%T\n", fun)
		switch fn := fun.(type) {
		case *ast.Ident:
			if fn.Name == "print" {
				// builtin print
				emitExpr(e.Args[0]) // push ptr, push len
				symbol := fmt.Sprintf("runtime.printstring")
				fmt.Printf("  callq %s\n", symbol)
				fmt.Printf("  addq $16, %%rsp # revert\n")
			} else {
				emitExpr(e.Args[0])
				symbol := "main." + fn.Name
				fmt.Printf("  callq %s\n", symbol)
				fmt.Printf("  addq $8, %%rsp # revert\n")
				fmt.Printf("  push %%rax\n")
			}
		case *ast.SelectorExpr:
			emitExpr(e.Args[0])
			symbol := fmt.Sprintf("%s.%s", fn.X, fn.Sel)
			fmt.Printf("  callq %s\n", symbol)
		default:
			panic(fmt.Sprintf("Unexpected expr type %T", fun))
		}
	case *ast.ParenExpr:
		emitExpr(e.X)
	case *ast.BasicLit:
		fmt.Printf("  # start %T\n", e)
		fmt.Printf("  # kind=%s\n", e.Kind)
		if e.Kind.String() == "INT" {
			val := e.Value
			ival, _ := strconv.Atoi(val)
			fmt.Printf("  movq $%d, %%rax\n", ival)
			fmt.Printf("  pushq %%rax\n")
		} else if e.Kind.String() == "STRING" {
			// e.Value == ".S%d:%d"
			splitted := strings.Split(e.Value, ":")
			fmt.Printf("  leaq %s, %%rax\n", splitted[0]) // str.ptr
			fmt.Printf("  pushq %%rax\n")
			fmt.Printf("  pushq $%s\n", splitted[1]) // str.len
		} else {
			panic("Unexpected literal kind:" + e.Kind.String())
		}
		fmt.Printf("  # end %T\n", e)
	case *ast.BinaryExpr:
		fmt.Printf("  # start %T\n", e)
		emitExpr(e.X) // left
		emitExpr(e.Y) // right
		if e.Op.String() == "+" {
			fmt.Printf("  popq %%rdi # right\n")
			fmt.Printf("  popq %%rax # left\n")
			fmt.Printf("  addq %%rdi, %%rax\n")
			fmt.Printf("  pushq %%rax\n")
		} else if e.Op.String() == "-" {
			fmt.Printf("  popq %%rdi # right\n")
			fmt.Printf("  popq %%rax # left\n")
			fmt.Printf("  subq %%rdi, %%rax\n")
			fmt.Printf("  pushq %%rax\n")
		} else if e.Op.String() == "*" {
			fmt.Printf("  popq %%rdi # right\n")
			fmt.Printf("  popq %%rax # left\n")
			fmt.Printf("  imulq %%rdi, %%rax\n")
			fmt.Printf("  pushq %%rax\n")
		} else {
			panic(fmt.Sprintf("Unexpected binary operator %s", e.Op))
		}
		fmt.Printf("  # end %T\n", e)
	default:
		panic(fmt.Sprintf("Unexpected expr type %T", expr))
	}
}

func emitFuncDecl(pkgPrefix string, fnc *Func) {
	funcDecl := fnc.decl
	fmt.Printf("%s.%s: # args %d, locals %d\n",
		pkgPrefix, funcDecl.Name, fnc.argsarea, fnc.localarea)
	fmt.Printf("  pushq %%rbp\n")
	fmt.Printf("  movq %%rsp, %%rbp\n")
	if len(fnc.localvars) > 0 {
		fmt.Printf("  subq $%d, %%rsp # local area\n", fnc.localarea)
	}
	for _, stmt := range funcDecl.Body.List {
		switch s := stmt.(type) {
		case *ast.ExprStmt:
			expr := s.X
			emitExpr(expr)
		case *ast.DeclStmt:
			continue
		case *ast.AssignStmt:
			fmt.Printf("  # *ast.AssignStmt\n")
			lhs := s.Lhs[0]
			rhs := s.Rhs[0]
			emitAddr(lhs)
			emitExpr(rhs)
			if getPrimType(lhs) == gString {
				fmt.Printf("  popq %%rcx # rhs len\n")
				fmt.Printf("  popq %%rax # rhs ptr\n")
				fmt.Printf("  popq %%rdx # lhs len addr\n")
				fmt.Printf("  popq %%rsi # lhs ptr addr\n")
				fmt.Printf("  movq %%rcx, (%%rdx) # len to len\n")
				fmt.Printf("  movq %%rax, (%%rsi) # ptr to ptr\n")
			} else {
				fmt.Printf("  popq %%rdi # rhs evaluated\n")
				fmt.Printf("  popq %%rax # lhs addr\n")
				fmt.Printf("  movq %%rdi, (%%rax)")
			}
		case *ast.ReturnStmt:
			emitExpr(s.Results[0])
			fmt.Printf("  popq %%rax # return\n")
		default:
			panic(fmt.Sprintf("Unexpected stmt type %T", stmt))
		}
	}
	fmt.Printf("  leave\n")
	fmt.Printf("  ret\n")
}

var stringLiterals []string
var stringIndex int

func registerStringLiteral(s string) string {
	rawStringLiteal := s
	stringLiterals = append(stringLiterals, rawStringLiteal)
	r := fmt.Sprintf(".S%d:%d", stringIndex, len(rawStringLiteal) - 2 -1) // \n is counted as 2 ?
	stringIndex++
	return r
}

func walkExpr(expr ast.Expr) {
	switch e := expr.(type) {
	case *ast.Ident:
		// what to do ?
	case *ast.CallExpr:
		for _, arg := range e.Args {
			walkExpr(arg)
		}
	case *ast.ParenExpr:
		walkExpr(e.X)
	case *ast.BasicLit:
		if e.Kind.String() == "INT" {
		} else if e.Kind.String() == "STRING" {
			e.Value = registerStringLiteral(e.Value)
		} else {
			panic("Unexpected literal kind:" + e.Kind.String())
		}
	case *ast.BinaryExpr:
		walkExpr(e.X) // left
		walkExpr(e.Y) // right
	default:
		panic(fmt.Sprintf("Unexpected expr type %T", expr))
	}
}

var gString = &ast.Object{
	Kind: ast.Typ,
	Name: "string",
	Decl: nil,
	Data: nil,
	Type: nil,
}

var gInt = &ast.Object{
	Kind: ast.Typ,
	Name: "int",
	Decl: nil,
	Data: nil,
	Type: nil,
}

var globalVars []*ast.ValueSpec
var globalFuncs []*Func

func semanticAnalyze(fset *token.FileSet, fiile *ast.File) {
	// https://github.com/golang/example/tree/master/gotypes#an-example
	// Type check
	// A Config controls various options of the type checker.
	// The defaults work fine except for one setting:
	// we must specify how to deal with imports.
	conf := types.Config{Importer: importer.Default()}

	// Type-check the package containing only file fiile.
	// Check returns a *types.Package.
	pkg, err := conf.Check("./t", fset, []*ast.File{fiile}, nil)
	if err != nil {
		panic(err)
	}

	fmt.Printf("# Package  %q\n", pkg.Path())
	universe := &ast.Scope{
		Outer:   nil,
		Objects: make(map[string]*ast.Object),
	}

	universe.Insert(gString)
	universe.Insert(gInt)
	universe.Insert(&ast.Object{
		Kind: ast.Fun,
		Name: "print",
		Decl: nil,
		Data: nil,
		Type: nil,
	})

	universe.Insert(&ast.Object{
		Kind: ast.Pkg,
		Name: "os", // why ???
		Decl: nil,
		Data: nil,
		Type: nil,
	})
	//fmt.Printf("Universer:    %v\n", types.Universe)
	ap, _ := ast.NewPackage(fset, map[string]*ast.File{"": fiile}, nil, universe)

	var unresolved []*ast.Ident
	for _, ident := range fiile.Unresolved {
		if obj := universe.Lookup(ident.Name); obj != nil {
			ident.Obj = obj
		} else {
			unresolved = append(unresolved, ident)
		}
	}

	fmt.Printf("# Name:    %s\n", pkg.Name())
	fmt.Printf("# Unresolved: %v\n", unresolved)
	fmt.Printf("# Package:   %s\n", ap.Name)


	for _, decl := range fiile.Decls {
		switch dcl := decl.(type) {
		case *ast.GenDecl:
			switch dcl.Tok {
			case token.VAR:
				spec := dcl.Specs[0]
				valSpec := spec.(*ast.ValueSpec)
				fmt.Printf("# valSpec.type=%#v\n", valSpec.Type)
				typeIdent , ok := valSpec.Type.(*ast.Ident)
				if !ok {
					panic("Unexpected case")
				}
				nameIdent := valSpec.Names[0]
				fmt.Printf("# spec.Name=%s, Value=%v\n", nameIdent, valSpec.Values[0])
				fmt.Printf("# nameIdent.Obj=%v\n", nameIdent.Obj)
				nameIdent.Obj.Data = -1 // mark as global
				if typeIdent.Obj == gString {
					lit,ok := valSpec.Values[0].(*ast.BasicLit)
					if !ok {
						panic("Unexpected type")
					}
					lit.Value = registerStringLiteral(lit.Value)
				} else if typeIdent.Obj == gInt {
					_,ok := valSpec.Values[0].(*ast.BasicLit)
					if !ok {
						panic("Unexpected type")
					}
				} else {
					panic("Unexpected global ident")
				}
				globalVars = append(globalVars, valSpec)
			}
		case *ast.FuncDecl:
			funcDecl := decl.(*ast.FuncDecl)
			var localvars []*ast.ValueSpec = nil
			var localoffset int
			var paramoffset int = 16
			for _, field := range funcDecl.Type.Params.List {
				obj :=field.Names[0].Obj
				var varSize int
				if getPrimType(field.Type) == gString {
					varSize = 16
				} else {
					varSize = 8
				}
				setObjData(obj, paramoffset)
				paramoffset += varSize
				fmt.Printf("# field.Names[0].Obj=%#v\n", obj)
			}
			for _, stmt := range funcDecl.Body.List {
				switch s := stmt.(type) {
				case *ast.ExprStmt:
					expr := s.X
					walkExpr(expr)
				case *ast.DeclStmt:
					decl := s.Decl
					switch dcl := decl.(type) {
					case *ast.GenDecl:
						declSpec := dcl.Specs[0]
						switch ds := declSpec.(type) {
						case *ast.ValueSpec:
							varSpec := ds
							obj := varSpec.Names[0].Obj
							var varSize int
							if getPrimType(varSpec.Type) == gString {
								varSize = 16
							} else {
								varSize = 8
							}

							localoffset -= varSize
							setObjData(obj, localoffset)
							localvars = append(localvars, ds)
						}
					default:
						panic(fmt.Sprintf("Unexpected type:%T", decl))
					}
				case *ast.AssignStmt:
					//lhs := s.Lhs[0]
					rhs := s.Rhs[0]
					walkExpr(rhs)
				case *ast.ReturnStmt:
					for _, r := range s.Results {
						walkExpr(r)
					}
				default:
					panic(fmt.Sprintf("Unexpected stmt type:%T", stmt))
				}
			}
			fnc := &Func{
				decl:      funcDecl,
				localvars: localvars,
				localarea: -localoffset,
				argsarea: paramoffset,
			}
			globalFuncs = append(globalFuncs, fnc)
		default:
			panic("unexpected decl type")
		}
	}
}

type Func struct {
	decl      *ast.FuncDecl
	localvars []*ast.ValueSpec
	localarea int
	argsarea  int
}

func getPrimType(typeExpr ast.Expr) *ast.Object {
	switch e := typeExpr.(type) {
	case *ast.Ident:
			fmt.Printf("  # ident kind=%v\n", e.Obj.Kind)
			if e.Obj.Kind == ast.Var {
				return getPrimType(e.Obj.Decl.(*ast.ValueSpec).Type)
			} else if e.Obj.Kind == ast.Typ {
				return e.Obj
			}
	default:
		panic("Unexpected typeExpr type")
	}

	return nil
}

func emitData() {
	fmt.Printf(".data\n")
	for i, sl := range stringLiterals {
		fmt.Printf("# string literals\n")
		fmt.Printf(".S%d:\n", i)
		fmt.Printf("  .string %s\n", sl)
	}

	fmt.Printf("# Global Variables\n")
	for _, valSpec := range globalVars {
		name := valSpec.Names[0]
		val := valSpec.Values[0]
		var strval string
		switch vl := val.(type) {
		case *ast.BasicLit:
			if getPrimType(valSpec.Type) == gString {
				strval = vl.Value
				splitted := strings.Split(strval,":")
				fmt.Printf("%s:  #\n", name)
				fmt.Printf("  .quad %s\n", splitted[0])
				fmt.Printf("  .quad %s\n", splitted[1])
			} else if getPrimType(valSpec.Type) == gInt {
				fmt.Printf("%s:  #\n", name)
				fmt.Printf("  .quad %s\n", vl.Value)
			} else {
				panic("Unexpected case")
			}

		default:
			panic("Only BasicLit is allowed in Global var's value")
		}
	}
}

func emitText() {
	fmt.Printf(".text\n")
	for _, fnc := range globalFuncs {
		emitFuncDecl("main", fnc)
	}
}

func generateCode(f *ast.File) {
	emitData()
	emitText()
}

func main() {
	fset := &token.FileSet{}
	f, err := parser.ParseFile(fset, "./t/source.go", nil, 0)
	if err != nil {
		panic(err)
	}

	semanticAnalyze(fset, f)
	generateCode(f)
}
