package stripper

import (
	"reflect"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStripper(t *testing.T) {
	opts := Options{
		RemoveComments: true,
		RemovePrivate: true,
	}
	stripper := New(opts)
	
	assert.NotNil(t, stripper)
	assert.Equal(t, opts, stripper.options)
}


func TestShouldRemoveByVisibility(t *testing.T) {
	tests := []struct {
		name       string
		options    Options
		nodeName   string
		visibility ir.Visibility
		expected   bool
	}{
		// Legacy RemovePrivate behavior (removes both private and protected)
		{"LegacyPrivate", Options{RemovePrivate: true}, "MyClass", ir.VisibilityPrivate, true},
		{"LegacyProtected", Options{RemovePrivate: true}, "MyClass", ir.VisibilityProtected, true},
		{"LegacyInternal", Options{RemovePrivate: true}, "MyClass", ir.VisibilityInternal, true},
		{"LegacyFilePrivate", Options{RemovePrivate: true}, "MyClass", ir.VisibilityFilePrivate, true},
		{"LegacyPublic", Options{RemovePrivate: true}, "MyClass", ir.VisibilityPublic, false},
		{"LegacyOpen", Options{RemovePrivate: true}, "MyClass", ir.VisibilityOpen, false},
		
		// RemovePrivateOnly behavior
		{"PrivateOnlyPrivate", Options{RemovePrivateOnly: true}, "MyClass", ir.VisibilityPrivate, true},
		{"PrivateOnlyProtected", Options{RemovePrivateOnly: true}, "MyClass", ir.VisibilityProtected, false},
		{"PrivateOnlyPublic", Options{RemovePrivateOnly: true}, "MyClass", ir.VisibilityPublic, false},
		
		// RemoveProtectedOnly behavior
		{"ProtectedOnlyPrivate", Options{RemoveProtectedOnly: true}, "MyClass", ir.VisibilityPrivate, false},
		{"ProtectedOnlyProtected", Options{RemoveProtectedOnly: true}, "MyClass", ir.VisibilityProtected, true},
		{"ProtectedOnlyPublic", Options{RemoveProtectedOnly: true}, "MyClass", ir.VisibilityPublic, false},
		
		// Python convention with RemovePrivate
		{"PythonPrivate", Options{RemovePrivate: true}, "_private_func", "", true},
		{"PythonDunder", Options{RemovePrivate: true}, "__init__", "", true},
		{"PythonPublic", Options{RemovePrivate: true}, "public_func", "", false},
		
		// Python convention with RemovePrivateOnly
		{"PythonPrivateOnly", Options{RemovePrivateOnly: true}, "_private_func", "", true},
		{"PythonPublicOnly", Options{RemovePrivateOnly: true}, "public_func", "", false},
		
		// No removal options
		{"NoRemoval", Options{}, "MyClass", ir.VisibilityPrivate, false},
		
		// Edge cases
		{"EmptyName", Options{RemovePrivate: true}, "", "", false},
		{"Underscore", Options{RemovePrivate: true}, "_", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stripper := New(tt.options)
			result := stripper.shouldRemoveByVisibility(tt.nodeName, tt.visibility)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestVisit(t *testing.T) {
	// Create a test file with various nodes
	file := &ir.DistilledFile{
		Path:     "test.py",
		Language: "python",
		Children: []ir.DistilledNode{
			&ir.DistilledComment{
				BaseNode: ir.BaseNode{},
				Text:     "File comment",
			},
			&ir.DistilledImport{
				BaseNode: ir.BaseNode{},
				Module:   "typing",
			},
			&ir.DistilledFunction{
				BaseNode:       ir.BaseNode{},
				Name:           "public_function",
				Visibility:     ir.VisibilityPublic,
				Implementation: "return 42",
			},
			&ir.DistilledFunction{
				BaseNode:       ir.BaseNode{},
				Name:           "_private_function",
				Visibility:     "",
				Implementation: "return secret",
			},
			&ir.DistilledClass{
				BaseNode:   ir.BaseNode{},
				Name:       "PublicClass",
				Visibility: ir.VisibilityPublic,
				Children: []ir.DistilledNode{
					&ir.DistilledField{
						BaseNode:   ir.BaseNode{},
						Name:       "_private_field",
						Visibility: "",
					},
					&ir.DistilledField{
						BaseNode:   ir.BaseNode{},
						Name:       "protected_field",
						Visibility: ir.VisibilityProtected,
					},
					&ir.DistilledFunction{
						BaseNode:   ir.BaseNode{},
						Name:       "protected_method",
						Visibility: ir.VisibilityProtected,
					},
				},
			},
		},
	}

	tests := []struct {
		name      string
		options   Options
		checkFunc func(t *testing.T, result *ir.DistilledFile)
	}{
		{
			name: "StripComments",
			options: Options{
				RemoveComments: true,
			},
			checkFunc: func(t *testing.T, result *ir.DistilledFile) {
				// Should not have comments
				for _, child := range result.Children {
					_, isComment := child.(*ir.DistilledComment)
					assert.False(t, isComment, "Should not have comments")
				}
			},
		},
		{
			name: "StripImports",
			options: Options{
				RemoveImports: true,
			},
			checkFunc: func(t *testing.T, result *ir.DistilledFile) {
				// Should not have imports
				for _, child := range result.Children {
					_, isImport := child.(*ir.DistilledImport)
					assert.False(t, isImport, "Should not have imports")
				}
			},
		},
		{
			name: "StripPrivate",
			options: Options{
				RemovePrivate: true,
			},
			checkFunc: func(t *testing.T, result *ir.DistilledFile) {
				// Should not have private functions
				for _, child := range result.Children {
					if fn, ok := child.(*ir.DistilledFunction); ok {
						assert.NotEqual(t, "_private_function", fn.Name)
					}
				}
			},
		},
		{
			name: "StripImplementation",
			options: Options{
				RemoveImplementations: true,
			},
			checkFunc: func(t *testing.T, result *ir.DistilledFile) {
				// Functions should have empty implementation
				for _, child := range result.Children {
					if fn, ok := child.(*ir.DistilledFunction); ok {
						assert.Empty(t, fn.Implementation)
					}
				}
			},
		},
		{
			name: "StripPrivateOnly",
			options: Options{
				RemovePrivateOnly: true,
			},
			checkFunc: func(t *testing.T, result *ir.DistilledFile) {
				// Should not have private members but should keep protected
				hasProtected := false
				hasPrivate := false
				
				for _, child := range result.Children {
					if class, ok := child.(*ir.DistilledClass); ok {
						for _, member := range class.Children {
							if field, ok := member.(*ir.DistilledField); ok {
								if field.Name == "protected_field" {
									hasProtected = true
								}
								if field.Name == "_private_field" {
									hasPrivate = true
								}
							}
						}
					}
				}
				
				assert.True(t, hasProtected, "Should keep protected members")
				assert.False(t, hasPrivate, "Should remove private members")
			},
		},
		{
			name: "StripProtectedOnly",
			options: Options{
				RemoveProtectedOnly: true,
			},
			checkFunc: func(t *testing.T, result *ir.DistilledFile) {
				// Should not have protected members but should keep private
				hasProtected := false
				hasPrivate := false
				
				for _, child := range result.Children {
					if class, ok := child.(*ir.DistilledClass); ok {
						for _, member := range class.Children {
							if field, ok := member.(*ir.DistilledField); ok {
								if field.Name == "protected_field" {
									hasProtected = true
								}
								if field.Name == "_private_field" {
									hasPrivate = true
								}
							}
						}
					}
				}
				
				assert.False(t, hasProtected, "Should remove protected members")
				assert.True(t, hasPrivate, "Should keep private members")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stripper := New(tt.options)
			walker := ir.NewWalker(stripper)
			result := walker.Walk(file)
			
			assert.NotNil(t, result)
			resultFile := result.(*ir.DistilledFile)
			assert.Equal(t, file.Path, resultFile.Path)
			
			if tt.checkFunc != nil {
				tt.checkFunc(t, resultFile)
			}
		})
	}
}
// TestVisitPackageStripsChildren verifies that declarations nested inside a
// DistilledPackage (e.g. C# namespaces) are stripped by visibility and
// implementation options. Before the visitPackage handler existed, package
// subtrees fell through to visitChildren and were returned unchanged (issue #3).
func TestVisitPackageStripsChildren(t *testing.T) {
	makeFile := func() *ir.DistilledFile {
		return &ir.DistilledFile{
			Path:     "demo.cs",
			Language: "csharp",
			Children: []ir.DistilledNode{
				&ir.DistilledPackage{
					Name: "Demo",
					Children: []ir.DistilledNode{
						&ir.DistilledClass{
							Name:       "Hidden",
							Visibility: ir.VisibilityInternal,
						},
						&ir.DistilledClass{
							Name:       "Calc",
							Visibility: ir.VisibilityPublic,
							Children: []ir.DistilledNode{
								&ir.DistilledFunction{
									Name:           "Add",
									Visibility:     ir.VisibilityPublic,
									Implementation: "{ return a + b; }",
								},
							},
						},
					},
				},
			},
		}
	}

	t.Run("internal class inside namespace is removed", func(t *testing.T) {
		s := New(Options{RemoveInternalOnly: true})
		result := makeFile().Accept(s).(*ir.DistilledFile)

		pkg := result.Children[0].(*ir.DistilledPackage)
		assert.Len(t, pkg.Children, 1, "internal class should be stripped from package")
		assert.Equal(t, "Calc", pkg.Children[0].(*ir.DistilledClass).Name)
	})

	t.Run("implementation inside namespace is stripped", func(t *testing.T) {
		s := New(Options{RemoveImplementations: true})
		result := makeFile().Accept(s).(*ir.DistilledFile)

		pkg := result.Children[0].(*ir.DistilledPackage)
		calc := pkg.Children[1].(*ir.DistilledClass)
		method := calc.Children[0].(*ir.DistilledFunction)
		assert.Empty(t, method.Implementation, "method body inside namespace should be stripped")
	})

	t.Run("no options preserves package subtree", func(t *testing.T) {
		s := New(Options{})
		// HasAnyOption is false, but Accept should still round-trip the package.
		result := makeFile().Accept(s).(*ir.DistilledFile)
		pkg := result.Children[0].(*ir.DistilledPackage)
		assert.Len(t, pkg.Children, 2)
	})
}

// TestExpandSymbols verifies that --expand keeps the implementation of symbols
// whose name matches an expand glob, even when implementations are otherwise
// stripped, while leaving non-matching symbols as signatures only.
func TestExpandSymbols(t *testing.T) {
	makeFile := func() *ir.DistilledFile {
		return &ir.DistilledFile{
			Path: "demo.go",
			Children: []ir.DistilledNode{
				&ir.DistilledFunction{
					Name:           "GetUser",
					Visibility:     ir.VisibilityPublic,
					Implementation: "{ return db.find(id) }",
				},
				&ir.DistilledFunction{
					Name:           "GetOrder",
					Visibility:     ir.VisibilityPublic,
					Implementation: "{ return db.order(id) }",
				},
			},
		}
	}

	t.Run("exact name match keeps only that body", func(t *testing.T) {
		s := New(Options{RemoveImplementations: true, ExpandSymbols: []string{"GetUser"}})
		result := makeFile().Accept(s).(*ir.DistilledFile)

		got := map[string]string{}
		for _, c := range result.Children {
			fn := c.(*ir.DistilledFunction)
			got[fn.Name] = fn.Implementation
		}
		assert.Equal(t, "{ return db.find(id) }", got["GetUser"], "matched symbol keeps body")
		assert.Empty(t, got["GetOrder"], "non-matched symbol is stripped to signature")
	})

	t.Run("glob pattern matches multiple", func(t *testing.T) {
		s := New(Options{RemoveImplementations: true, ExpandSymbols: []string{"Get*"}})
		result := makeFile().Accept(s).(*ir.DistilledFile)
		for _, c := range result.Children {
			fn := c.(*ir.DistilledFunction)
			assert.NotEmpty(t, fn.Implementation, "Get* should expand %s", fn.Name)
		}
	})

	t.Run("no expand patterns strips all bodies", func(t *testing.T) {
		s := New(Options{RemoveImplementations: true})
		result := makeFile().Accept(s).(*ir.DistilledFile)
		for _, c := range result.Children {
			fn := c.(*ir.DistilledFunction)
			assert.Empty(t, fn.Implementation)
		}
	})
}

// TestExpandSymbols_ClassLevel verifies that matching a class name with --expand
// keeps the implementation of ALL methods inside that class, while methods of
// non-matching classes are still stripped to signatures.
func TestExpandSymbols_ClassLevel(t *testing.T) {
	mkClass := func(name string) *ir.DistilledClass {
		return &ir.DistilledClass{
			Name:       name,
			Visibility: ir.VisibilityPublic,
			Children: []ir.DistilledNode{
				&ir.DistilledFunction{Name: "Get", Visibility: ir.VisibilityPublic, Implementation: "{ get }"},
				&ir.DistilledFunction{Name: "Set", Visibility: ir.VisibilityPublic, Implementation: "{ set }"},
			},
		}
	}
	file := &ir.DistilledFile{
		Path:     "demo.cs",
		Children: []ir.DistilledNode{mkClass("UserService"), mkClass("OrderService")},
	}

	s := New(Options{RemoveImplementations: true, ExpandSymbols: []string{"UserService"}})
	result := file.Accept(s).(*ir.DistilledFile)

	classes := map[string]*ir.DistilledClass{}
	for _, c := range result.Children {
		cl := c.(*ir.DistilledClass)
		classes[cl.Name] = cl
	}

	for _, m := range classes["UserService"].Children {
		fn := m.(*ir.DistilledFunction)
		assert.NotEmpty(t, fn.Implementation, "matched class: UserService.%s body should be kept", fn.Name)
	}
	for _, m := range classes["OrderService"].Children {
		fn := m.(*ir.DistilledFunction)
		assert.Empty(t, fn.Implementation, "non-matched class: OrderService.%s body should be stripped", fn.Name)
	}
}

// TestExpandSymbols_StructLevel verifies that class-level expansion also works
// for structs (Go/Rust/Swift/C#), exercising the visitStruct path.
func TestExpandSymbols_StructLevel(t *testing.T) {
	file := &ir.DistilledFile{
		Path: "demo.go",
		Children: []ir.DistilledNode{
			&ir.DistilledStruct{
				Name:       "Server",
				Visibility: ir.VisibilityPublic,
				Children: []ir.DistilledNode{
					&ir.DistilledFunction{Name: "Start", Visibility: ir.VisibilityPublic, Implementation: "{ listen() }"},
					&ir.DistilledFunction{Name: "Stop", Visibility: ir.VisibilityPublic, Implementation: "{ close() }"},
				},
			},
		},
	}

	s := New(Options{RemoveImplementations: true, ExpandSymbols: []string{"Server"}})
	result := file.Accept(s).(*ir.DistilledFile)

	strct := result.Children[0].(*ir.DistilledStruct)
	for _, m := range strct.Children {
		fn := m.(*ir.DistilledFunction)
		assert.NotEmpty(t, fn.Implementation, "matched struct: Server.%s body should be kept", fn.Name)
	}
}

// fillNonZero sets every settable exported field it understands to a non-zero value.
// depth stops the recursion for self-referential types such as TypeRef.TypeArgs.
func fillNonZero(v reflect.Value, depth int) {
	if depth > 3 {
		return
	}
	switch v.Kind() {
	case reflect.String:
		v.SetString("x")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int64:
		v.SetInt(7)
	case reflect.Ptr:
		if v.Type().Elem().Kind() == reflect.Struct {
			v.Set(reflect.New(v.Type().Elem()))
			fillNonZero(v.Elem(), depth+1)
		}
	case reflect.Slice:
		if k := v.Type().Elem().Kind(); k == reflect.String || k == reflect.Struct {
			s := reflect.MakeSlice(v.Type(), 1, 1)
			fillNonZero(s.Index(0), depth+1)
			v.Set(s)
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Field(i).CanSet() {
				fillNonZero(v.Field(i), depth+1)
			}
		}
	}
}

func TestStripperPreservesEveryField(t *testing.T) {
	nodes := []ir.DistilledNode{
		&ir.DistilledFunction{}, &ir.DistilledClass{}, &ir.DistilledInterface{},
		&ir.DistilledStruct{}, &ir.DistilledEnum{}, &ir.DistilledField{},
		&ir.DistilledTypeAlias{}, &ir.DistilledPackage{}, &ir.DistilledFile{},
	}
	for _, n := range nodes {
		fillNonZero(reflect.ValueOf(n).Elem(), 0)
		if f := reflect.ValueOf(n).Elem().FieldByName("Visibility"); f.IsValid() {
			f.SetString(string(ir.VisibilityPublic))
		}
		// RemoveComments makes the stripper run without touching these node types.
		got := New(Options{RemoveComments: true}).Visit(n)
		assert.Equal(t, n, got, "%T lost a field in the stripper", n)
	}
}

// TestExpandSymbols_QualifiedName verifies that a pattern matches the member
// part of a qualified function name (a C++ out-of-class definition such as
// "Bar::vis"), like the method name of a Go receiver method, as well as the
// whole qualified name. The class part alone does not match: the definition
// is not nested inside the class.
func TestExpandSymbols_QualifiedName(t *testing.T) {
	body := "{ return 1; }"
	cases := []struct {
		pattern string
		name    string
		want    bool
	}{
		{"vis", "Bar::vis", true},
		{"vis", "ns::Bar::In::vis", true},
		{"v*", "Tp<T>::vis", true},
		{"Bar::vis", "Bar::vis", true},
		{"Bar::*", "Bar::vis", true},
		{"Bar", "Bar::vis", false},
		{"is", "Bar::vis", false},
		{"vis", "Bar::visible", false},
	}
	for _, tc := range cases {
		file := &ir.DistilledFile{Children: []ir.DistilledNode{
			&ir.DistilledFunction{Name: tc.name, Visibility: ir.VisibilityPublic, Implementation: body},
		}}
		s := New(Options{RemoveImplementations: true, ExpandSymbols: []string{tc.pattern}})
		fn := file.Accept(s).(*ir.DistilledFile).Children[0].(*ir.DistilledFunction)
		assert.Equal(t, tc.want, fn.Implementation == body, "--expand=%s on %s", tc.pattern, tc.name)
	}
}

func TestRemovePublic_RemovesPublicMembers(t *testing.T) {
	file := &ir.DistilledFile{Children: []ir.DistilledNode{
		&ir.DistilledFunction{Name: "Top", Visibility: ir.VisibilityPublic},
		&ir.DistilledFunction{Name: "low", Visibility: ir.VisibilityPrivate},
	}}
	got := New(Options{RemovePublic: true}).Visit(file).(*ir.DistilledFile)
	require.Len(t, got.Children, 1)
	assert.Equal(t, "low", got.Children[0].(*ir.DistilledFunction).Name)
}

func TestRemovePublic_KeepsContainersWithSurvivors(t *testing.T) {
	file := &ir.DistilledFile{Children: []ir.DistilledNode{
		&ir.DistilledClass{Name: "A", Visibility: ir.VisibilityPublic, Children: []ir.DistilledNode{
			&ir.DistilledFunction{Name: "pub", Visibility: ir.VisibilityPublic},
			&ir.DistilledFunction{Name: "priv", Visibility: ir.VisibilityPrivate},
		}},
		&ir.DistilledClass{Name: "Empty", Visibility: ir.VisibilityPublic, Children: []ir.DistilledNode{
			&ir.DistilledFunction{Name: "onlyPub", Visibility: ir.VisibilityPublic},
		}},
	}}
	got := New(Options{RemovePublic: true}).Visit(file).(*ir.DistilledFile)
	require.Len(t, got.Children, 1)
	a := got.Children[0].(*ir.DistilledClass)
	assert.Equal(t, "A", a.Name)
	require.Len(t, a.Children, 1)
	assert.Equal(t, "priv", a.Children[0].(*ir.DistilledFunction).Name)
}

func TestRemovePublic_ContainerKindsAndInputUnchanged(t *testing.T) {
	mk := func() *ir.DistilledFile {
		return &ir.DistilledFile{Children: []ir.DistilledNode{
			&ir.DistilledInterface{Name: "I", Visibility: ir.VisibilityPublic, Children: []ir.DistilledNode{
				&ir.DistilledFunction{Name: "m", Visibility: ir.VisibilityPublic}}},
			&ir.DistilledStruct{Name: "S", Visibility: ir.VisibilityPublic, Children: []ir.DistilledNode{
				&ir.DistilledField{Name: "f", Visibility: ir.VisibilityPublic},
				&ir.DistilledField{Name: "g", Visibility: ir.VisibilityPrivate}}},
			&ir.DistilledEnum{Name: "E", Visibility: ir.VisibilityPublic, Children: []ir.DistilledNode{
				&ir.DistilledField{Name: "V", Visibility: ir.VisibilityPublic}}},
			&ir.DistilledPackage{Name: "P", Children: []ir.DistilledNode{
				&ir.DistilledTypeAlias{Name: "T", Visibility: ir.VisibilityPublic}}},
			&ir.DistilledPackage{Name: "Q", Children: []ir.DistilledNode{
				&ir.DistilledTypeAlias{Name: "U", Visibility: ir.VisibilityPrivate}}},
			&ir.DistilledPackage{Name: "clause"},
		}}
	}
	in := mk()
	got := New(Options{RemovePublic: true}).Visit(in).(*ir.DistilledFile)
	require.Len(t, got.Children, 3)
	s := got.Children[0].(*ir.DistilledStruct)
	require.Len(t, s.Children, 1)
	assert.Equal(t, "g", s.Children[0].(*ir.DistilledField).Name)
	assert.Equal(t, "Q", got.Children[1].(*ir.DistilledPackage).Name)
	assert.Equal(t, "clause", got.Children[2].(*ir.DistilledPackage).Name, "childless package clause is kept")
	assert.Equal(t, mk(), in, "input IR must stay unchanged")
}

func TestRemovePublic_CountsAsOption(t *testing.T) {
	assert.True(t, Options{RemovePublic: true}.HasAnyOption())
}

func TestRemovePublic_NonPublicContainerKeepsPublicTaggedMembers(t *testing.T) {
	file := &ir.DistilledFile{Children: []ir.DistilledNode{
		&ir.DistilledEnum{Name: "Color", Visibility: ir.VisibilityPrivate, Children: []ir.DistilledNode{
			&ir.DistilledField{Name: "RED", Visibility: ir.VisibilityPublic},
			&ir.DistilledField{Name: "GREEN", Visibility: ir.VisibilityPublic}}},
		&ir.DistilledInterface{Name: "Cb", Visibility: ir.VisibilityPackage, Children: []ir.DistilledNode{
			&ir.DistilledFunction{Name: "call", Visibility: ir.VisibilityPublic}}},
		&ir.DistilledClass{Name: "Pub", Visibility: ir.VisibilityPublic, Children: []ir.DistilledNode{
			&ir.DistilledFunction{Name: "gone", Visibility: ir.VisibilityPublic},
			&ir.DistilledClass{Name: "Inner", Visibility: ir.VisibilityPrivate, Children: []ir.DistilledNode{
				&ir.DistilledFunction{Name: "kept", Visibility: ir.VisibilityPublic}}},
			&ir.DistilledFunction{Name: "gone2", Visibility: ir.VisibilityPublic}}},
	}}
	got := New(Options{RemovePublic: true}).Visit(file).(*ir.DistilledFile)
	require.Len(t, got.Children, 3)
	assert.Len(t, got.Children[0].(*ir.DistilledEnum).Children, 2)
	assert.Len(t, got.Children[1].(*ir.DistilledInterface).Children, 1)
	pub := got.Children[2].(*ir.DistilledClass)
	require.Len(t, pub.Children, 1, "scope flag restored after the non-public inner class")
	assert.Equal(t, "Inner", pub.Children[0].(*ir.DistilledClass).Name)
	assert.Len(t, pub.Children[0].(*ir.DistilledClass).Children, 1)
}
