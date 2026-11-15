package php

import (
	"context"
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/janreges/ai-distiller/internal/processor"
	"github.com/stretchr/testify/assert"
)

// TestPHPConstruct1Basic tests basic PHP constructs
func TestPHPConstruct1Basic(t *testing.T) {
	source := `<?php

declare(strict_types=1);

/**
 * Calculates the final price after applying a discount.
 *
 * @param float $price The original price.
 * @param int $discountPercent The discount percentage.
 * @return float The price after discount.
 */
function calculate_final_price(float $price, int $discountPercent): float
{
    if ($price <= 0) {
        return 0.0;
    }

    $discountAmount = $price * ($discountPercent / 100);

    return $price - $discountAmount;
}

// A simple, empty class definition to test basic OOP parsing.
class Product
{
}

$bookPrice = 20.0;
$finalPrice = calculate_final_price($bookPrice, 15);

echo "Final price: " . $finalPrice;`

	tests := []struct {
		name  string
		opts  processor.ProcessOptions
		check func(t *testing.T, file *ir.DistilledFile)
	}{
		{
			name: "full",
			opts: processor.DefaultProcessOptions(),
			check: func(t *testing.T, file *ir.DistilledFile) {
				// Should have function with docstring
				var fn *ir.DistilledFunction
				for _, child := range file.Children {
					if f, ok := child.(*ir.DistilledFunction); ok && f.Name == "calculate_final_price" {
						fn = f
						break
					}
				}
				if fn == nil {
					t.Fatal("Function calculate_final_price not found")
				}
				if fn.Implementation == "" {
					t.Error("Function should have implementation")
				}

				// Should have empty class
				var cls *ir.DistilledClass
				for _, child := range file.Children {
					if c, ok := child.(*ir.DistilledClass); ok && c.Name == "Product" {
						cls = c
						break
					}
				}
				if cls == nil {
					t.Fatal("Class Product not found")
				}
			},
		},
		{
			name: "no_impl",
			opts: processor.ProcessOptions{
				IncludePrivate:        true,
				IncludeImplementation: false,
				IncludeComments:       true,
				IncludeImports:        true,
			},
			check: func(t *testing.T, file *ir.DistilledFile) {
				// Function should have no implementation
				for _, child := range file.Children {
					if fn, ok := child.(*ir.DistilledFunction); ok && fn.Name == "calculate_final_price" {
						if fn.Implementation != "" {
							t.Error("Function should not have implementation")
						}
						break
					}
				}
			},
		},
	}

	p := NewProcessor()
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(source)
			file, err := p.ProcessWithOptions(ctx, reader, "test.php", tt.opts)
			if err != nil {
				t.Fatalf("Failed to process: %v", err)
			}
			tt.check(t, file)
		})
	}
}

// TestPHPConstruct2PropertyPromotion tests constructor property promotion
func TestPHPConstruct2PropertyPromotion(t *testing.T) {
	source := `<?php

declare(strict_types=1);

// Using PHP 8 Constructor Property Promotion
class User
{
    public function __construct(
        private readonly int $id,
        private string $name,
        private string $email
    ) {}

    public function getId(): int
    {
        return $this->id;
    }

    public function getDisplayName(): string
    {
        return "User: " . $this->name;
    }

    public function changeEmail(string $newEmail): void
    {
        // Basic validation logic
        if (!filter_var($newEmail, FILTER_VALIDATE_EMAIL)) {
            throw new InvalidArgumentException("Invalid email format provided.");
        }
        $this->email = $newEmail;
    }
}`

	tests := []struct {
		name  string
		opts  processor.ProcessOptions
		check func(t *testing.T, file *ir.DistilledFile)
	}{
		{
			name: "full",
			opts: processor.DefaultProcessOptions(),
			check: func(t *testing.T, file *ir.DistilledFile) {
				// Find User class
				var user *ir.DistilledClass
				for _, child := range file.Children {
					if c, ok := child.(*ir.DistilledClass); ok && c.Name == "User" {
						user = c
						break
					}
				}
				if user == nil {
					t.Fatal("Class User not found")
				}

				// Check promoted properties
				fields := 0
				var idField *ir.DistilledField
				for _, child := range user.Children {
					if f, ok := child.(*ir.DistilledField); ok {
						fields++
						if f.Name == "id" {
							idField = f
						}
					}
				}
				if fields != 3 {
					t.Errorf("Expected 3 fields, got %d", fields)
				}
				if idField == nil {
					t.Error("Field 'id' not found")
				} else {
					if idField.Visibility != ir.VisibilityPrivate {
						t.Error("Field 'id' should be private")
					}
					// Check for readonly modifier
					hasReadonly := false
					for _, mod := range idField.Modifiers {
						if mod == ir.ModifierReadonly {
							hasReadonly = true
							break
						}
					}
					if !hasReadonly {
						t.Error("Field 'id' should have readonly modifier")
					}
				}
			},
		},
		{
			name: "no_private",
			opts: processor.ProcessOptions{
				IncludePrivate:        false,
				IncludeImplementation: true,
				IncludeComments:       true,
				IncludeImports:        true,
				IncludeFields:         true, // Fixed: default should include fields
				IncludeMethods:        true, // Fixed: default should include methods
			},
			check: func(t *testing.T, file *ir.DistilledFile) {
				// Find User class
				var user *ir.DistilledClass
				for _, child := range file.Children {
					if c, ok := child.(*ir.DistilledClass); ok && c.Name == "User" {
						user = c
						break
					}
				}
				if user == nil {
					t.Fatal("Class User not found")
				}

				// Should have no fields (all are private)
				fields := 0
				for _, child := range user.Children {
					if _, ok := child.(*ir.DistilledField); ok {
						fields++
					}
				}
				if fields != 0 {
					t.Errorf("Expected 0 fields, got %d", fields)
				}

				// Should have public constructor
				hasConstructor := false
				for _, child := range user.Children {
					if fn, ok := child.(*ir.DistilledFunction); ok && fn.Name == "__construct" {
						hasConstructor = true
						break
					}
				}
				if !hasConstructor {
					t.Error("Constructor should be present")
				}
			},
		},
	}

	p := NewProcessor()
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(source)
			file, err := p.ProcessWithOptions(ctx, reader, "test.php", tt.opts)
			if err != nil {
				t.Fatalf("Failed to process: %v", err)
			}
			tt.check(t, file)
		})
	}
}

// TestPHPConstruct3InterfacesAbstract tests interfaces and abstract classes
func TestPHPConstruct3InterfacesAbstract(t *testing.T) {
	source := `<?php

declare(strict_types=1);

interface Loggable
{
    public function log(string $message): void;
}

abstract class AbstractStorage
{
    protected string $storagePath;

    public function __construct(string $storagePath)
    {
        $this->storagePath = rtrim($storagePath, '/');
    }

    abstract protected function save(string $key, string $data): bool;
    
    final public function getStoragePath(): string
    {
        return $this->storagePath;
    }
}

class FileLogger extends AbstractStorage implements Loggable
{
    public function __construct(string $logDirectory)
    {
        parent::__construct($logDirectory);
    }

    public function log(string $message): void
    {
        $this->save('log_' . date('Y-m-d'), $message . PHP_EOL);
    }

    protected function save(string $key, string $data): bool
    {
        $file = $this->storagePath . '/' . $key . '.log';
        return file_put_contents($file, $data, FILE_APPEND) !== false;
    }
}`

	p := NewProcessor()
	ctx := context.Background()
	reader := strings.NewReader(source)

	file, err := p.ProcessWithOptions(ctx, reader, "test.php", processor.DefaultProcessOptions())
	if err != nil {
		t.Fatalf("Failed to process: %v", err)
	}

	// Check interface
	var intf *ir.DistilledInterface
	for _, child := range file.Children {
		if i, ok := child.(*ir.DistilledInterface); ok && i.Name == "Loggable" {
			intf = i
			break
		}
	}
	if intf == nil {
		t.Fatal("Interface Loggable not found")
	}

	// Check abstract class
	var abstractClass *ir.DistilledClass
	for _, child := range file.Children {
		if c, ok := child.(*ir.DistilledClass); ok && c.Name == "AbstractStorage" {
			abstractClass = c
			break
		}
	}
	if abstractClass == nil {
		t.Fatal("Abstract class AbstractStorage not found")
	}

	// Check for abstract modifier
	hasAbstract := false
	for _, mod := range abstractClass.Modifiers {
		if mod == ir.ModifierAbstract {
			hasAbstract = true
			break
		}
	}
	if !hasAbstract {
		t.Error("AbstractStorage should have abstract modifier")
	}

	// Check final method
	var finalMethod *ir.DistilledFunction
	for _, child := range abstractClass.Children {
		if fn, ok := child.(*ir.DistilledFunction); ok && fn.Name == "getStoragePath" {
			finalMethod = fn
			break
		}
	}
	if finalMethod == nil {
		t.Fatal("Method getStoragePath not found")
	}

	hasFinal := false
	for _, mod := range finalMethod.Modifiers {
		if mod == ir.ModifierFinal {
			hasFinal = true
			break
		}
	}
	if !hasFinal {
		t.Error("getStoragePath should have final modifier")
	}

	// Check FileLogger implements and extends
	var fileLogger *ir.DistilledClass
	for _, child := range file.Children {
		if c, ok := child.(*ir.DistilledClass); ok && c.Name == "FileLogger" {
			fileLogger = c
			break
		}
	}
	if fileLogger == nil {
		t.Fatal("Class FileLogger not found")
	}

	if len(fileLogger.Extends) != 1 || fileLogger.Extends[0].Name != "AbstractStorage" {
		t.Error("FileLogger should extend AbstractStorage")
	}

	if len(fileLogger.Implements) != 1 || fileLogger.Implements[0].Name != "Loggable" {
		t.Error("FileLogger should implement Loggable")
	}
}

// TestPHPConstruct4TraitsUnionTypes tests traits and union types
func TestPHPConstruct4TraitsUnionTypes(t *testing.T) {
	source := `<?php

namespace App\Services\Notification;

use App\Utils\Timestampable;
use Psr\Log\LoggerInterface;

class EmailPayload
{
    use Timestampable;

    public function __construct(
        public readonly string $recipient,
        public readonly string $subject,
        public readonly string $body
    ) {
        $this->touch();
    }
}

class Notifier
{
    public function __construct(private LoggerInterface $logger) {}

    public function send(EmailPayload|string $payload): void
    {
        if (is_string($payload)) {
            $this->logger->info("Raw string notification: {$payload}");
            return;
        }

        $this->logger->info(
            "Sending email to {$payload->recipient} with subject '{$payload->subject}'"
        );
    }
}

namespace App\Utils;

trait Timestampable
{
    private ?\DateTimeImmutable $createdAt = null;

    public function touch(): void
    {
        if ($this->createdAt === null) {
            $this->createdAt = new \DateTimeImmutable();
        }
    }

    public function getCreationDate(): ?\DateTimeImmutable
    {
        return $this->createdAt;
    }
}`

	p := NewProcessor()
	ctx := context.Background()
	reader := strings.NewReader(source)

	file, err := p.ProcessWithOptions(ctx, reader, "test.php", processor.DefaultProcessOptions())
	if err != nil {
		t.Fatalf("Failed to process: %v", err)
	}

	// Check trait use
	var emailPayload *ir.DistilledClass
	for _, child := range file.Children {
		if c, ok := child.(*ir.DistilledClass); ok && c.Name == "EmailPayload" {
			emailPayload = c
			break
		}
	}
	if emailPayload == nil {
		t.Fatal("Class EmailPayload not found")
	}

	// Check that EmailPayload uses traits
	if len(emailPayload.Mixins) == 0 {
		t.Error("EmailPayload should use Timestampable trait")
	} else {
		assert.Equal(t, "Timestampable", emailPayload.Mixins[0].Name, "EmailPayload should use Timestampable trait")
	}

	// Check union type
	var notifier *ir.DistilledClass
	for _, child := range file.Children {
		if c, ok := child.(*ir.DistilledClass); ok && c.Name == "Notifier" {
			notifier = c
			break
		}
	}
	if notifier == nil {
		t.Fatal("Class Notifier not found")
	}

	var sendMethod *ir.DistilledFunction
	for _, child := range notifier.Children {
		if fn, ok := child.(*ir.DistilledFunction); ok && fn.Name == "send" {
			sendMethod = fn
			break
		}
	}
	if sendMethod == nil {
		t.Fatal("Method send not found")
	}

	if len(sendMethod.Parameters) != 1 {
		t.Fatal("Send method should have 1 parameter")
	}

	param := sendMethod.Parameters[0]
	if !strings.Contains(param.Type.Name, "|") {
		t.Errorf("Parameter should have union type, got %s", param.Type.Name)
	}

	// Check trait definition
	var trait *ir.DistilledClass
	for _, child := range file.Children {
		if c, ok := child.(*ir.DistilledClass); ok && c.Name == "Timestampable" {
			// Verify it's marked as a trait
			if c.Extensions != nil && c.Extensions.PHP != nil && c.Extensions.PHP.IsTrait {
				trait = c
				break
			}
		}
	}
	if trait == nil {
		t.Fatal("Trait Timestampable not found or not marked as trait")
	}
}

// TestPHPConstruct5Attributes tests attributes and complex interfaces
func TestPHPConstruct5Attributes(t *testing.T) {
	source := `<?php

namespace App\Data\Repositories;

use App\Data\Models\Product;
use App\Data\Contracts\{Cacheable, Deletable};
use App\Data\Traits\HasSoftDeletes;
use \Serializable;

#[\Attribute(\Attribute::TARGET_CLASS)]
class RepositoryConfig
{
    public function __construct(public string $model) {}
}

interface FindableById
{
    public function find(int $id);
}

#[RepositoryConfig(model: Product::class)]
class ProductRepository extends BaseRepository implements FindableById, Cacheable, Deletable, Serializable
{
    use HasSoftDeletes;

    private static int $queryCount = 0;
    protected array $searchableFields = ['name', 'sku'];

    public function __construct()
    {
        parent::__construct(Product::class);
    }

    public function find(int $id): ?Product
    {
        self::$queryCount++;
        if ($id === 1) {
            return new Product(1, 'Laptop', 1500.00);
        }
        return null;
    }

    /**
     * @return Product[]
     */
    public function findBy(string $field, mixed $value): array
    {
        self::$queryCount++;
        return [new Product(1, 'Laptop', 1500.00)];
    }

    public function clearCache(): bool { return true; }
    public function serialize(): string { return ''; }
    public function unserialize(string $data): void { }
}`

	p := NewProcessor()
	ctx := context.Background()
	reader := strings.NewReader(source)

	file, err := p.ProcessWithOptions(ctx, reader, "test.php", processor.DefaultProcessOptions())
	if err != nil {
		t.Fatalf("Failed to process: %v", err)
	}

	// Check grouped use imports
	cacheableImport := false
	deletableImport := false
	for _, child := range file.Children {
		if imp, ok := child.(*ir.DistilledImport); ok {
			if strings.Contains(imp.Module, "Cacheable") && strings.Contains(imp.Module, "Contracts") {
				cacheableImport = true
			}
			if strings.Contains(imp.Module, "Deletable") && strings.Contains(imp.Module, "Contracts") {
				deletableImport = true
			}
		}
	}
	if !cacheableImport {
		t.Error("Cacheable import not correctly resolved")
	}
	if !deletableImport {
		t.Error("Deletable import not correctly resolved")
	}

	// Check attribute on class
	var productRepo *ir.DistilledClass
	for _, child := range file.Children {
		if c, ok := child.(*ir.DistilledClass); ok && c.Name == "ProductRepository" {
			productRepo = c
			break
		}
	}
	if productRepo == nil {
		t.Fatal("Class ProductRepository not found")
	}

	if len(productRepo.Decorators) == 0 {
		t.Error("ProductRepository should have decorators")
	} else {
		if !strings.Contains(productRepo.Decorators[0], "RepositoryConfig") {
			t.Error("ProductRepository should have RepositoryConfig attribute")
		}
	}

	// Check static property
	var staticField *ir.DistilledField
	for _, child := range productRepo.Children {
		if f, ok := child.(*ir.DistilledField); ok && f.Name == "queryCount" {
			staticField = f
			break
		}
	}
	if staticField == nil {
		t.Fatal("Static field queryCount not found")
	}

	hasStatic := false
	for _, mod := range staticField.Modifiers {
		if mod == ir.ModifierStatic {
			hasStatic = true
			break
		}
	}
	if !hasStatic {
		t.Error("queryCount should have static modifier")
	}

	// Check multiple interface implementation
	if len(productRepo.Implements) != 4 {
		t.Errorf("ProductRepository should implement 4 interfaces, got %d", len(productRepo.Implements))
	}
}
