package kotlin

import (
	"context"
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessor_Basic(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected []string
	}{
		{
			name: "simple_class",
			code: `
package com.example

class MyClass {
    private val value: Int = 0
    
    fun getValue(): Int {
        return value
    }
}`,
			expected: []string{
				"com.example",
				"MyClass",
				"value",
				"getValue",
			},
		},
		{
			name: "data_class",
			code: `
data class Person(
    val firstName: String,
    val lastName: String,
    val age: Int
)`,
			expected: []string{
				"Person",
				"firstName",
				"lastName",
				"age",
				"data",
			},
		},
		{
			name: "interface",
			code: `
interface Service {
    fun execute()
    val name: String
}`,
			expected: []string{
				"Service",
				"execute",
				"name",
			},
		},
		{
			name: "object_declaration",
			code: `
object Singleton {
    fun doSomething() {
        println("Doing something")
    }
    
    val instance = "single"
}`,
			expected: []string{
				"Singleton",
				"doSomething",
				"instance",
			},
		},
		{
			name: "companion_object",
			code: `
class Factory {
    companion object {
        fun create(): Factory {
            return Factory()
        }
    }
}`,
			expected: []string{
				"Factory",
				"Companion",
				"create",
			},
		},
		{
			name: "enum_class",
			code: `
enum class Status {
    ACTIVE,
    INACTIVE,
    PENDING
}`,
			expected: []string{
				"Status",
				"ACTIVE",
				"INACTIVE",
				"PENDING",
			},
		},
		{
			name: "sealed_class",
			code: `
sealed class Result {
    data class Success(val data: String) : Result()
    data class Error(val message: String) : Result()
}`,
			expected: []string{
				"Result",
				"sealed",
				"Success",
				"data",
				"Error",
			},
		},
		{
			name: "extension_function",
			code: `
fun String.isPalindrome(): Boolean {
    return this == this.reversed()
}`,
			expected: []string{
				"String.isPalindrome",
			},
		},
	}

	processor := NewProcessor()
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.code)
			result, err := processor.Process(ctx, reader, "test.kt")
			require.NoError(t, err)
			require.NotNil(t, result)

			// Convert result to string for easier checking
			content := convertToString(result)

			// Check that all expected elements are present
			for _, exp := range tt.expected {
				assert.Contains(t, content, exp, "Expected to find %s in output", exp)
			}
		})
	}
}

func TestProcessor_AdvancedFeatures(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected []string
	}{
		{
			name: "coroutines",
			code: `
import kotlinx.coroutines.*

class CoroutineExample {
    suspend fun fetchData(): String {
        delay(1000)
        return "data"
    }
    
    fun processAsync() = GlobalScope.launch {
        val data = fetchData()
        println(data)
    }
}`,
			expected: []string{
				"CoroutineExample",
				"fetchData",
				"processAsync",
				"async", // suspend functions should have async modifier
			},
		},
		{
			name: "generics",
			code: `
class Box<T>(val value: T) {
    fun get(): T = value
}

interface Repository<T> where T : Entity {
    fun save(item: T)
    fun findById(id: Long): T?
}`,
			expected: []string{
				"Box",
				"value",
				"get",
				"Repository",
				"save",
				"findById",
			},
		},
		{
			name: "properties_with_accessors",
			code: `
class Temperature {
    var celsius: Double = 0.0
        set(value) {
            field = value
        }
    
    val fahrenheit: Double
        get() = celsius * 9/5 + 32
}`,
			expected: []string{
				"Temperature",
				"celsius",
				"fahrenheit",
			},
		},
		{
			name: "inline_functions",
			code: `
inline fun <reified T> isInstance(value: Any): Boolean {
    return value is T
}

inline class Password(val value: String)`,
			expected: []string{
				"isInstance",
				"@inline",
				"Password",
			},
		},
		{
			name: "operator_overloading",
			code: `
data class Vector(val x: Int, val y: Int) {
    operator fun plus(other: Vector): Vector {
        return Vector(x + other.x, y + other.y)
    }
}`,
			expected: []string{
				"Vector",
				"plus",
				"@operator",
			},
		},
		{
			name: "lambdas_and_higher_order",
			code: `
class Calculator {
    fun calculate(a: Int, b: Int, operation: (Int, Int) -> Int): Int {
        return operation(a, b)
    }
    
    val add: (Int, Int) -> Int = { x, y -> x + y }
}`,
			expected: []string{
				"Calculator",
				"calculate",
				"add",
			},
		},
	}

	processor := NewProcessor()
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.code)
			result, err := processor.Process(ctx, reader, "test.kt")
			require.NoError(t, err)
			require.NotNil(t, result)

			content := convertToString(result)

			for _, exp := range tt.expected {
				assert.Contains(t, content, exp, "Expected to find %s in output")
			}
		})
	}
}

func TestProcessor_ComplexFeatures(t *testing.T) {
	code := `
package com.example.app

import kotlinx.coroutines.*
import java.util.*

// Annotations
annotation class DSLMarker

@DSLMarker
annotation class HtmlTagMarker

// Sealed class hierarchy
sealed class Response<out T> {
    data class Success<T>(val data: T) : Response<T>()
    data class Error(val exception: Exception) : Response<Nothing>()
}

// Interface with default methods
interface Logger {
    val tag: String
        get() = javaClass.simpleName
    
    fun log(message: String) {
        println("[$tag] $message")
    }
}

// Abstract class
abstract class BaseViewModel : Logger {
    abstract val state: State
    
    protected abstract suspend fun loadData()
    
    open fun refresh() {
        GlobalScope.launch {
            loadData()
        }
    }
}

// Data class with secondary constructor
data class User(
    val id: Long,
    val name: String,
    val email: String
) {
    constructor(name: String, email: String) : this(0, name, email)
}

// Object with nested class
object UserRepository {
    private val users = mutableListOf<User>()
    
    fun add(user: User) {
        users.add(user)
    }
    
    class UserNotFoundException(id: Long) : Exception("User $id not found")
}

// Class with companion object
class ServiceFactory {
    companion object {
        const val DEFAULT_TIMEOUT = 5000L
        
        @JvmStatic
        fun create(): Service {
            return ServiceImpl()
        }
    }
}

// Extension functions
fun <T> List<T>.secondOrNull(): T? = if (size >= 2) this[1] else null

suspend fun <T> Response<T>.getOrThrow(): T = when (this) {
    is Response.Success -> data
    is Response.Error -> throw exception
}

// Inline classes
@JvmInline
value class UserId(val value: Long)

// DSL example
@HtmlTagMarker
class Html {
    fun body(init: Body.() -> Unit): Body {
        val body = Body()
        body.init()
        return body
    }
}

@HtmlTagMarker
class Body {
    fun p(text: String) {
        println("<p>$text</p>")
    }
}

// Type aliases
typealias UserMap = Map<Long, User>
typealias UserPredicate = (User) -> Boolean
`

	processor := NewProcessor()
	ctx := context.Background()

	reader := strings.NewReader(code)
	result, err := processor.Process(ctx, reader, "complex.kt")
	require.NoError(t, err)
	require.NotNil(t, result)

	content := convertToString(result)

	// Check various Kotlin features
	assert.Contains(t, content, "com.example.app")
	assert.Contains(t, content, "Response")
	assert.Contains(t, content, "sealed")
	assert.Contains(t, content, "Logger")
	assert.Contains(t, content, "BaseViewModel")
	assert.Contains(t, content, "User")
	assert.Contains(t, content, "UserRepository")
	assert.Contains(t, content, "ServiceFactory")
	assert.Contains(t, content, "Companion")
	assert.Contains(t, content, "UserId")
	assert.Contains(t, content, "Html")
	assert.Contains(t, content, "Body")
	assert.Contains(t, content, "UserMap")
	assert.Contains(t, content, "data")
	assert.Contains(t, content, "async") // suspend functions
}

func TestProcessor_Visibility(t *testing.T) {
	code := `
class VisibilityTest {
    public val publicField: Int = 1
    private val privateField: Int = 2
    protected val protectedField: Int = 3
    internal val internalField: Int = 4
    
    public fun publicMethod() {}
    private fun privateMethod() {}
    protected fun protectedMethod() {}
    internal fun internalMethod() {}
}`

	processor := NewProcessor()
	ctx := context.Background()

	reader := strings.NewReader(code)
	result, err := processor.Process(ctx, reader, "visibility.kt")
	require.NoError(t, err)
	require.NotNil(t, result)

	// Check visibility assignments
	var checkVisibility func(ir.DistilledNode)
	checkVisibility = func(node ir.DistilledNode) {
		switch n := node.(type) {
		case *ir.DistilledClass:
			// Default visibility in Kotlin is public
			assert.Equal(t, ir.VisibilityPublic, n.Visibility)
			for _, child := range n.Children {
				checkVisibility(child)
			}
		case *ir.DistilledField:
			switch n.Name {
			case "publicField":
				assert.Equal(t, ir.VisibilityPublic, n.Visibility)
			case "privateField":
				assert.Equal(t, ir.VisibilityPrivate, n.Visibility)
			case "protectedField":
				assert.Equal(t, ir.VisibilityProtected, n.Visibility)
			case "internalField":
				assert.Equal(t, ir.VisibilityInternal, n.Visibility)
			}
		case *ir.DistilledFunction:
			switch n.Name {
			case "publicMethod":
				assert.Equal(t, ir.VisibilityPublic, n.Visibility)
			case "privateMethod":
				assert.Equal(t, ir.VisibilityPrivate, n.Visibility)
			case "protectedMethod":
				assert.Equal(t, ir.VisibilityProtected, n.Visibility)
			case "internalMethod":
				assert.Equal(t, ir.VisibilityInternal, n.Visibility)
			}
		}
	}

	for _, node := range result.Children {
		checkVisibility(node)
	}
}

// Helper function to convert result to string representation
func convertToString(file *ir.DistilledFile) string {
	var sb strings.Builder

	// Recursively collect all names and modifiers from the IR structure
	var collectInfo func(node ir.DistilledNode)
	collectInfo = func(node ir.DistilledNode) {
		switch n := node.(type) {
		case *ir.DistilledImport:
			if n.Module != "" {
				sb.WriteString(n.Module + " ")
			}
		case *ir.DistilledClass:
			sb.WriteString(n.Name + " ")
			for _, mod := range n.Modifiers {
				sb.WriteString(string(mod) + " ")
			}
			for _, dec := range n.Decorators {
				sb.WriteString(dec + " ")
			}
		case *ir.DistilledStruct:
			sb.WriteString(n.Name + " ")
		case *ir.DistilledInterface:
			sb.WriteString(n.Name + " ")
		case *ir.DistilledFunction:
			sb.WriteString(n.Name + " ")
			for _, mod := range n.Modifiers {
				sb.WriteString(string(mod) + " ")
			}
			for _, dec := range n.Decorators {
				sb.WriteString(dec + " ")
			}
		case *ir.DistilledField:
			sb.WriteString(n.Name + " ")
			for _, mod := range n.Modifiers {
				sb.WriteString(string(mod) + " ")
			}
		case *ir.DistilledPackage:
			sb.WriteString(n.Name + " ")
		case *ir.DistilledEnum:
			sb.WriteString(n.Name + " ")
		case *ir.DistilledTypeAlias:
			sb.WriteString(n.Name + " ")
		}

		// Process children
		if node != nil {
			for _, child := range node.GetChildren() {
				collectInfo(child)
			}
		}
	}

	// Process all top-level nodes
	for _, node := range file.Children {
		collectInfo(node)
	}

	return sb.String()
}

func parseKotlin(t *testing.T, src string) *ir.DistilledFile {
	t.Helper()
	file, err := NewProcessor().Process(context.Background(), strings.NewReader(src), "test.kt")
	require.NoError(t, err)
	require.NotNil(t, file)
	return file
}

func TestKotlin_FunctionBodies(t *testing.T) {
	src := "class R {\n    fun find(id: Int): String {\n        val x = load(id)\n        return x\n    }\n    fun quick(id: Int): String = load(id)\n}\n"
	file := parseKotlin(t, src)
	fns := map[string]*ir.DistilledFunction{}
	for _, c := range file.Children[0].(*ir.DistilledClass).Children {
		if fn, ok := c.(*ir.DistilledFunction); ok {
			fns[fn.Name] = fn
		}
	}
	require.Contains(t, fns, "find")
	require.Contains(t, fns, "quick")
	assert.Equal(t, "{\n        val x = load(id)\n        return x\n    }", fns["find"].Implementation)
	assert.Equal(t, "= load(id)", fns["quick"].Implementation)
}

// commentsOf returns the format and text of every comment among nodes.
func commentsOf(nodes []ir.DistilledNode) [][2]string {
	var got [][2]string
	for _, c := range nodes {
		if cm, ok := c.(*ir.DistilledComment); ok {
			got = append(got, [2]string{cm.Format, cm.Text})
		}
	}
	return got
}

func TestKotlin_Comments(t *testing.T) {
	file := parseKotlin(t, "// line c\n\n/** Doc c */\nfun f(): Int = 1\n")
	assert.Equal(t, [][2]string{{ir.CommentLine, "line c"}, {ir.CommentDoc, "Doc c"}}, commentsOf(file.Children))
}

// labels names every node: "doc:<text>", "line:<text>" and "block:<text>" for
// comments, the name for the rest.
func labels(nodes []ir.DistilledNode) []string {
	var out []string
	for _, n := range nodes {
		switch n := n.(type) {
		case *ir.DistilledComment:
			out = append(out, n.Format+":"+n.Text)
		case *ir.DistilledFunction:
			out = append(out, n.Name)
		case *ir.DistilledField:
			out = append(out, n.Name)
		case *ir.DistilledClass:
			out = append(out, n.Name)
		case *ir.DistilledInterface:
			out = append(out, n.Name)
		case *ir.DistilledImport:
			out = append(out, "import "+n.Module)
		case *ir.DistilledPackage:
			out = append(out, "package "+n.Name)
		}
	}
	return out
}

// The grammar puts the comments after the package header or an import inside
// that node; they still come out in source order at file level.
func TestKotlin_CommentsAroundHeader(t *testing.T) {
	file := parseKotlin(t, "/** pkg doc */\npackage a.b\n// before imports\nimport x.Y\n// between\nimport x.Z\n\n/** file doc */\n\n/** class doc */\nclass C {\n    // body line\n    /* body block */\n    /** g doc */\n    fun g() {}\n}\n")
	assert.Equal(t, []string{"doc:pkg doc", "package a.b", "line:before imports", "import x.Y", "line:between", "import x.Z",
		"doc:file doc", "doc:class doc", "C"}, labels(file.Children))
	assert.Equal(t, []string{"line:body line", "block:body block", "doc:g doc", "g"}, labels(file.Children[len(file.Children)-1].(*ir.DistilledClass).Children))
}

func TestKotlin_InterfaceAndObjectComments(t *testing.T) {
	file := parseKotlin(t, "interface I {\n    /** h doc */\n    fun h()\n}\nobject O {\n    // o line\n    /** x doc */\n    const val X = 1\n}\n")
	assert.Equal(t, []string{"doc:h doc", "h"}, labels(file.Children[0].(*ir.DistilledInterface).Children))
	assert.Equal(t, []string{"line:o line", "doc:x doc", "X"}, labels(file.Children[1].(*ir.DistilledClass).Children))
}

// KDoc is /** */ only: "///" is a plain line comment.
func TestKotlin_TripleSlashIsALineComment(t *testing.T) {
	file := parseKotlin(t, "/// not KDoc\nfun f() {}\n")
	assert.Equal(t, []string{"line:not KDoc", "f"}, labels(file.Children))
}

// An init block or a function body is implementation: its comments and local
// declarations are not members, and the doc of an init block (which adds no
// declaration) does not document anything.
func TestKotlin_CommentsInsideBodiesStayOut(t *testing.T) {
	file := parseKotlin(t, "class K {\n    /** init doc */\n    init {\n        // inside init\n        val local = 1\n    }\n    fun f() {\n        // inside body\n    }\n}\n")
	assert.Equal(t, []string{"f"}, labels(file.Children[0].(*ir.DistilledClass).Children))
}

// A comment between the class name and the primary constructor (or the
// supertypes) is a child of the declaration node itself; it stays with the
// class, before the members.
func TestKotlin_CommentInDeclarationHeader(t *testing.T) {
	file := parseKotlin(t, "internal class D\n// pools comment\nprivate constructor() : Base {\n    fun f() {}\n}\nobject O // object comment\n    : Base {\n    fun g() {}\n}\ninterface I // interface comment\n    : J {\n    fun h()\n}\n")
	assert.Equal(t, []string{"line:pools comment", "constructor", "f"}, labels(file.Children[0].(*ir.DistilledClass).Children))
	assert.Equal(t, []string{"line:object comment", "g"}, labels(file.Children[1].(*ir.DistilledClass).Children))
	assert.Equal(t, []string{"line:interface comment", "h"}, labels(file.Children[2].(*ir.DistilledInterface).Children))
}
