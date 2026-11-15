package php

import (
	"context"
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessorBasicClass(t *testing.T) {
	source := `<?php
namespace App\Models;

use App\Core\BaseModel;

class User extends BaseModel
{
    public const TYPE_USER = 'user';
    
    public string $name;
    protected string $email;
    private string $password;
    
    public function __construct(string $name, string $email)
    {
        $this->name = $name;
        $this->email = $email;
    }
    
    public function getName(): string
    {
        return $this->name;
    }
    
    protected function getEmail(): string
    {
        return $this->email;
    }
    
    private function validatePassword(string $password): bool
    {
        return strlen($password) >= 8;
    }
}`

	proc := NewProcessor()

	ctx := context.Background()
	file, err := proc.Process(ctx, strings.NewReader(source), "test.php")

	require.NoError(t, err)
	require.NotNil(t, file)

	// Check file metadata
	assert.Equal(t, "test.php", file.Path)
	assert.Equal(t, "php", file.Language)

	// Find the User class
	var userClass *ir.DistilledClass
	for _, child := range file.Children {
		if class, ok := child.(*ir.DistilledClass); ok {
			if class.Name == "User" {
				userClass = class
				break
			}
		}
	}

	require.NotNil(t, userClass, "User class not found")
}

func TestProcessorPHP8Features(t *testing.T) {
	source := `<?php
namespace App\Modern;

enum Status: string
{
    case PENDING = 'pending';
    case APPROVED = 'approved';
}

#[Route('/api/users')]
class UserController
{
    public function __construct(
        private readonly string $apiKey,
        public string|int $version = 1
    ) {}
    
    #[Route('/api/users/{id}', methods: ['GET'])]
    public function show(string|int $id): array
    {
        return ['id' => $id];
    }
}`

	proc := NewProcessor()

	ctx := context.Background()
	file, err := proc.Process(ctx, strings.NewReader(source), "modern.php")

	require.NoError(t, err)
	require.NotNil(t, file)

	// Should parse without errors
	assert.Empty(t, file.Errors)

	// Find UserController class
	var controller *ir.DistilledClass
	for _, child := range file.Children {
		if class, ok := child.(*ir.DistilledClass); ok && class.Name == "UserController" {
			controller = class
			break
		}
	}
	require.NotNil(t, controller, "UserController class not found")

	// Check class has attribute
	assert.NotEmpty(t, controller.Decorators, "UserController should have decorators")
	assert.Contains(t, controller.Decorators[0], "Route", "UserController should have Route attribute")

	// Check constructor property promotion
	fieldCount := 0
	var apiKeyField *ir.DistilledField
	for _, child := range controller.Children {
		if field, ok := child.(*ir.DistilledField); ok {
			fieldCount++
			if field.Name == "apiKey" {
				apiKeyField = field
			}
		}
	}
	assert.Equal(t, 2, fieldCount, "Expected 2 promoted properties")
	require.NotNil(t, apiKeyField, "apiKey field not found")
	assert.Equal(t, ir.VisibilityPrivate, apiKeyField.Visibility, "apiKey should be private")

	// Check for readonly modifier
	hasReadonly := false
	for _, mod := range apiKeyField.Modifiers {
		if mod == ir.ModifierReadonly {
			hasReadonly = true
			break
		}
	}
	assert.True(t, hasReadonly, "apiKey should have readonly modifier")

	// Check method with attribute
	var showMethod *ir.DistilledFunction
	for _, child := range controller.Children {
		if fn, ok := child.(*ir.DistilledFunction); ok && fn.Name == "show" {
			showMethod = fn
			break
		}
	}
	require.NotNil(t, showMethod, "show method not found")
	assert.NotEmpty(t, showMethod.Decorators, "show method should have decorators")

	// Check union type parameter
	assert.Len(t, showMethod.Parameters, 1, "show method should have 1 parameter")
	assert.Contains(t, showMethod.Parameters[0].Type.Name, "|", "Parameter should have union type")
}

func TestProcessorNamespaceAndUse(t *testing.T) {
	source := `<?php
namespace App\Core\Services;

use App\Contracts\ServiceInterface;
use App\Traits\{Loggable, Cacheable};
use App\Events\UserCreated as UserCreatedEvent;
use function App\Helpers\sanitize_input;
use const App\Config\DEFAULT_TIMEOUT;

class UserService implements ServiceInterface
{
    use Loggable, Cacheable;
    
    public function createUser(): void
    {
        // Implementation
    }
}`

	proc := NewProcessor()

	ctx := context.Background()
	file, err := proc.Process(ctx, strings.NewReader(source), "namespace.php")

	require.NoError(t, err)
	require.NotNil(t, file)

	// Check that imports are captured
	importCount := 0
	var loggableImport, cacheableImport *ir.DistilledImport
	for _, child := range file.Children {
		if imp, ok := child.(*ir.DistilledImport); ok {
			importCount++
			if strings.Contains(imp.Module, "Loggable") {
				loggableImport = imp
			}
			if strings.Contains(imp.Module, "Cacheable") {
				cacheableImport = imp
			}
		}
	}

	// Should have at least some imports
	assert.Greater(t, importCount, 0, "No imports found")

	// Check grouped use statements are parsed correctly
	assert.NotNil(t, loggableImport, "Loggable import not found")
	assert.NotNil(t, cacheableImport, "Cacheable import not found")
	assert.Contains(t, loggableImport.Module, "App\\Traits\\Loggable")
	assert.Contains(t, cacheableImport.Module, "App\\Traits\\Cacheable")
}

func TestProcessorTraits(t *testing.T) {
	source := `<?php
trait Loggable
{
    public function log(string $message): void
    {
        error_log($message);
    }
}

trait Timestampable
{
    public DateTime $createdAt;
    public DateTime $updatedAt;
}

class User
{
    use Loggable, Timestampable;
    
    public string $name;
}`

	proc := NewProcessor()

	ctx := context.Background()
	file, err := proc.Process(ctx, strings.NewReader(source), "traits.php")

	require.NoError(t, err)
	require.NotNil(t, file)

	// Should have parsed traits and class
	assert.GreaterOrEqual(t, len(file.Children), 3, "Expected at least 3 top-level nodes")

	// Check for Timestampable trait
	var timestampableTrait *ir.DistilledClass
	for _, child := range file.Children {
		if class, ok := child.(*ir.DistilledClass); ok && class.Name == "Timestampable" {
			// Verify it's marked as a trait
			if class.Extensions != nil && class.Extensions.PHP != nil && class.Extensions.PHP.IsTrait {
				timestampableTrait = class
				break
			}
		}
	}
	assert.NotNil(t, timestampableTrait, "Timestampable trait not found or not marked as trait")

	// Check for User class with trait use comment
	var userClass *ir.DistilledClass
	for _, child := range file.Children {
		if class, ok := child.(*ir.DistilledClass); ok && class.Name == "User" {
			userClass = class
			break
		}
	}
	require.NotNil(t, userClass, "User class not found")

	// Check that User class uses traits
	assert.NotEmpty(t, userClass.Mixins, "User class should have mixins (traits)")
	assert.Len(t, userClass.Mixins, 2, "User class should use 2 traits")
	assert.Equal(t, "Loggable", userClass.Mixins[0].Name, "First trait should be Loggable")
	assert.Equal(t, "Timestampable", userClass.Mixins[1].Name, "Second trait should be Timestampable")
}

func TestProcessorInterfaces(t *testing.T) {
	source := `<?php
interface Jsonable
{
    public function toJson(): string;
}

interface Arrayable
{
    public function toArray(): array;
}

interface Serializable extends Jsonable, Arrayable
{
    public function serialize(): string;
}

class Model implements Serializable
{
    public function toJson(): string
    {
        return '{}';
    }
    
    public function toArray(): array
    {
        return [];
    }
    
    public function serialize(): string
    {
        return '';
    }
}`

	proc := NewProcessor()

	ctx := context.Background()
	file, err := proc.Process(ctx, strings.NewReader(source), "interfaces.php")

	require.NoError(t, err)
	require.NotNil(t, file)

	// Should have parsed interfaces and class
	assert.GreaterOrEqual(t, len(file.Children), 4, "Expected at least 4 top-level nodes")

	// Check that interfaces are properly parsed as DistilledInterface
	interfaceCount := 0
	var jsonable, arrayable, serializable *ir.DistilledInterface
	for _, child := range file.Children {
		if intf, ok := child.(*ir.DistilledInterface); ok {
			interfaceCount++
			switch intf.Name {
			case "Jsonable":
				jsonable = intf
			case "Arrayable":
				arrayable = intf
			case "Serializable":
				serializable = intf
			}
		}
	}

	assert.Equal(t, 3, interfaceCount, "Expected 3 interfaces")
	assert.NotNil(t, jsonable, "Jsonable interface not found")
	assert.NotNil(t, arrayable, "Arrayable interface not found")
	assert.NotNil(t, serializable, "Serializable interface not found")

	// Check that Serializable extends other interfaces
	assert.Equal(t, 2, len(serializable.Extends), "Serializable should extend 2 interfaces")
}
