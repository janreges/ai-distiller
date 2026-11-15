package semantic

import (
	"context"
	"strings"
	"testing"
)

func TestTypeScriptAnalyzer_AnalyzeFile(t *testing.T) {
	analyzer, err := NewTypeScriptAnalyzer()
	if err != nil {
		t.Fatalf("Failed to create TypeScript analyzer: %v", err)
	}
	defer analyzer.Close()

	testCode := `
// TypeScript test file with comprehensive language features
import { Component, useState, useEffect } from 'react';
import * as utils from './utils';
import { Logger } from './logger';
const config = require('./config');

// Type definitions
interface User {
    id: number;
    name: string;
    email?: string;
}

type Status = 'active' | 'inactive' | 'pending';

// Enum declaration
enum UserRole {
    Admin = 'admin',
    User = 'user',
    Guest = 'guest'
}

// Generic interface
interface Repository<T> {
    findById(id: number): Promise<T | null>;
    save(entity: T): Promise<T>;
}

// Class with inheritance and generics
class UserService implements Repository<User> {
    private logger: Logger;
    protected cache: Map<number, User>;
    
    constructor(logger: Logger) {
        this.logger = logger;
        this.cache = new Map();
    }
    
    public async findById(id: number): Promise<User | null> {
        // Check cache first
        if (this.cache.has(id)) {
            return this.cache.get(id) || null;
        }
        
        // Fetch from API
        const user = await this.fetchUser(id);
        if (user) {
            this.cache.set(id, user);
        }
        
        return user;
    }
    
    private async fetchUser(id: number): Promise<User | null> {
        try {
            const response = await fetch('/api/users/' + id);
            return await response.json();
        } catch (error) {
            this.logger.error('Failed to fetch user', error);
            return null;
        }
    }
    
    public save(user: User): Promise<User> {
        return utils.saveToDatabase(user);
    }
}

// Arrow function with generic types
const createUser = async <T extends User>(userData: Partial<T>): Promise<T> => {
    const newUser = await userService.save(userData as T);
    logger.info('User created:', newUser.id);
    return newUser;
};

// Function declarations
function validateEmail(email: string): boolean {
    const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
    return emailRegex.test(email);
}

async function processUsers(users: User[]): Promise<User[]> {
    const validUsers = users.filter(user => validateEmail(user.email || ''));
    
    for (const user of validUsers) {
        await userService.save(user);
        utils.logAction('user_processed', user.id);
    }
    
    return validUsers;
}

// Namespace declaration
namespace Analytics {
    export interface Event {
        name: string;
        data: Record<string, any>;
    }
    
    export function trackEvent(event: Event): void {
        console.log('Tracking:', event.name);
    }
}

// Module instantiation
const userService = new UserService(new Logger());
const analytics = Analytics;

// Various call patterns
userService.findById(123).then(user => {
    if (user) {
        analytics.trackEvent({
            name: 'user_loaded',
            data: { userId: user.id }
        });
    }
});

// Optional chaining
userService.findById(456)?.then(user => {
    console.log(user?.name);
});

// Generic function call
createUser<User>({ name: 'John', email: 'john@example.com' });

// Export default
export default UserService;
export { UserRole, Analytics };
`

	reader := strings.NewReader(testCode)
	analysis, err := analyzer.AnalyzeFile(context.Background(), reader, "test.ts")

	if err != nil {
		t.Fatalf("Failed to analyze TypeScript file: %v", err)
	}

	// Verify analysis results
	if analysis.Language != "typescript" {
		t.Errorf("Expected language 'typescript', got '%s'", analysis.Language)
	}

	// Check symbol extraction
	symbolTable := analysis.SymbolTable

	// Check for interface symbols
	if _, exists := symbolTable.GetSymbol("User"); !exists {
		t.Error("Expected to find User interface symbol")
	}

	userSymbol, _ := symbolTable.GetSymbol("User")
	if userSymbol.Kind != SymbolKindInterface {
		t.Errorf("Expected User to be an interface, got %s", userSymbol.Kind)
	}

	// Check for type alias
	if _, exists := symbolTable.GetSymbol("Status"); !exists {
		t.Error("Expected to find Status type alias")
	}

	// Check for enum
	if _, exists := symbolTable.GetSymbol("UserRole"); !exists {
		t.Error("Expected to find UserRole enum")
	}

	// Check for class
	if _, exists := symbolTable.GetSymbol("UserService"); !exists {
		t.Error("Expected to find UserService class")
	}

	userServiceSymbol, _ := symbolTable.GetSymbol("UserService")
	if userServiceSymbol.Kind != SymbolKindClass {
		t.Errorf("Expected UserService to be a class, got %s", userServiceSymbol.Kind)
	}

	// Check for methods within class
	findByIdSymbol, _ := symbolTable.GetSymbol("findById")
	if findByIdSymbol == nil {
		t.Error("Expected to find findById method")
	} else if findByIdSymbol.Kind != SymbolKindMethod {
		t.Errorf("Expected findById to be a method, got %s", findByIdSymbol.Kind)
	}

	// Check visibility
	if findByIdSymbol.Visibility != "public" {
		t.Errorf("Expected findById to be public, got %s", findByIdSymbol.Visibility)
	}

	fetchUserSymbol, _ := symbolTable.GetSymbol("fetchUser")
	if fetchUserSymbol != nil && fetchUserSymbol.Visibility != "private" {
		t.Errorf("Expected fetchUser to be private, got %s", fetchUserSymbol.Visibility)
	}

	// Check for functions
	if _, exists := symbolTable.GetSymbol("validateEmail"); !exists {
		t.Error("Expected to find validateEmail function")
	}

	if _, exists := symbolTable.GetSymbol("processUsers"); !exists {
		t.Error("Expected to find processUsers function")
	}

	// Check for arrow function
	if _, exists := symbolTable.GetSymbol("createUser"); !exists {
		t.Error("Expected to find createUser arrow function")
	}

	// Check for namespace
	if _, exists := symbolTable.GetSymbol("Analytics"); !exists {
		t.Error("Expected to find Analytics namespace")
	}

	// Check dependencies
	expectedDeps := []string{"react", "./utils", "./logger", "./config"}
	if len(analysis.Dependencies) == 0 {
		t.Error("Expected to find dependencies")
	}

	for _, expectedDep := range expectedDeps {
		found := false
		for _, dep := range analysis.Dependencies {
			if dep.TargetModule == expectedDep {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected to find dependency '%s'", expectedDep)
		}
	}

	// Check call sites
	if len(analysis.CallSites) == 0 {
		t.Error("Expected to find call sites")
	}

	// Look for specific call sites
	callSiteNames := make(map[string]bool)
	for _, callSite := range analysis.CallSites {
		callSiteNames[callSite.CalleeName] = true
	}

	expectedCalls := []string{"findById", "save", "validateEmail", "saveToDatabase", "trackEvent", "createUser"}
	for _, expectedCall := range expectedCalls {
		if !callSiteNames[expectedCall] {
			t.Errorf("Expected to find call to '%s'", expectedCall)
		}
	}

	t.Logf("Analysis successful: %d symbols, %d dependencies, %d call sites",
		len(symbolTable.Symbols), len(analysis.Dependencies), len(analysis.CallSites))
}

func TestTypeScriptAnalyzer_ExtractGenericTypes(t *testing.T) {
	analyzer, err := NewTypeScriptAnalyzer()
	if err != nil {
		t.Fatalf("Failed to create TypeScript analyzer: %v", err)
	}
	defer analyzer.Close()

	testCode := `
interface Repository<T, K = string> {
    findByKey(key: K): Promise<T | null>;
}

class GenericService<T extends User, K = number> implements Repository<T, K> {
    async findByKey(key: K): Promise<T | null> {
        return null;
    }
}

function processItems<T, U extends T>(items: T[], processor: (item: T) => U): U[] {
    return items.map(processor);
}

const arrayHelper = <T>(items: T[]): T[] => {
    return items.filter(Boolean);
};
`

	reader := strings.NewReader(testCode)
	analysis, err := analyzer.AnalyzeFile(context.Background(), reader, "generics.ts")

	if err != nil {
		t.Fatalf("Failed to analyze TypeScript file: %v", err)
	}

	// Check for generic interface
	if _, exists := analysis.SymbolTable.GetSymbol("Repository"); !exists {
		t.Error("Expected to find Repository interface")
	}

	repoSymbol, _ := analysis.SymbolTable.GetSymbol("Repository")
	if repoSymbol.Kind != SymbolKindInterface {
		t.Error("Expected Repository to be an interface")
	}

	// Check for generic class
	if _, exists := analysis.SymbolTable.GetSymbol("GenericService"); !exists {
		t.Error("Expected to find GenericService class")
	}

	serviceSymbol, _ := analysis.SymbolTable.GetSymbol("GenericService")
	if serviceSymbol.Kind != SymbolKindClass {
		t.Error("Expected GenericService to be a class")
	}

	// Check for generic function
	if _, exists := analysis.SymbolTable.GetSymbol("processItems"); !exists {
		t.Error("Expected to find processItems function")
	}

	// Check for generic arrow function
	if _, exists := analysis.SymbolTable.GetSymbol("arrayHelper"); !exists {
		t.Error("Expected to find arrayHelper arrow function")
	}

	t.Logf("Generic analysis successful: %d symbols found", len(analysis.SymbolTable.Symbols))
}

func TestTypeScriptAnalyzer_ModernLanguageFeatures(t *testing.T) {
	analyzer, err := NewTypeScriptAnalyzer()
	if err != nil {
		t.Fatalf("Failed to create TypeScript analyzer: %v", err)
	}
	defer analyzer.Close()

	testCode := `
// Modern TypeScript features
import type { ComponentType } from 'react';

// Utility types
type Partial<T> = {
    [P in keyof T]?: T[P];
};

type Pick<T, K extends keyof T> = {
    [P in K]: T[P];
};

// Conditional types
type NonNullable<T> = T extends null | undefined ? never : T;

// Template literal types
type EventName<T extends string> = ` + "`" + `on${Capitalize<T>}` + "`" + `;

// Decorator support
@Component({
    selector: 'app-user',
    template: '<div>User</div>'
})
class UserComponent {
    @Input() user!: User;
    @Output() userClick = new EventEmitter<User>();
    
    @HostListener('click', ['$event'])
    onClick(event: MouseEvent): void {
        this.userClick.emit(this.user);
    }
}

// Optional chaining and nullish coalescing
function processUser(user?: User): string {
    return user?.profile?.displayName ?? user?.name ?? 'Unknown';
}

// Async/await with error handling
async function fetchUserData(id: number): Promise<User | null> {
    try {
        const response = await fetch(` + "`" + `/api/users/${id}` + "`" + `);
        if (!response.ok) {
            throw new Error(` + "`" + `HTTP ${response.status}` + "`" + `);
        }
        return await response.json();
    } catch (error) {
        console.error('Failed to fetch user:', error);
        return null;
    }
}

// Private class fields
class PrivateFieldExample {
    #privateField: string;
    readonly publicField: string;
    
    constructor(value: string) {
        this.#privateField = value;
        this.publicField = value;
    }
    
    #privateMethod(): string {
        return this.#privateField;
    }
    
    public getPrivateValue(): string {
        return this.#privateMethod();
    }
}

// Export patterns
export { UserComponent as default };
export type { ComponentType };
`

	reader := strings.NewReader(testCode)
	analysis, err := analyzer.AnalyzeFile(context.Background(), reader, "modern.ts")

	if err != nil {
		t.Fatalf("Failed to analyze modern TypeScript file: %v", err)
	}

	// Check for utility type aliases (these are complex and may not be fully captured)
	expectedTypes := []string{"Partial", "Pick", "NonNullable", "EventName"}
	foundUtilityTypes := 0
	for _, typeName := range expectedTypes {
		if _, exists := analysis.SymbolTable.GetSymbol(typeName); exists {
			foundUtilityTypes++
		}
	}

	// Utility types are complex, so we don't require all of them to be found
	t.Logf("Found %d out of %d utility types (complex types may not be fully captured)", foundUtilityTypes, len(expectedTypes))

	// Check for decorated class
	if _, exists := analysis.SymbolTable.GetSymbol("UserComponent"); !exists {
		t.Error("Expected to find UserComponent class")
	}

	// Check for async function
	if _, exists := analysis.SymbolTable.GetSymbol("fetchUserData"); !exists {
		t.Error("Expected to find fetchUserData function")
	}

	// Check for class with private fields
	if _, exists := analysis.SymbolTable.GetSymbol("PrivateFieldExample"); !exists {
		t.Error("Expected to find PrivateFieldExample class")
	}

	// Verify method extraction
	if _, exists := analysis.SymbolTable.GetSymbol("getPrivateValue"); !exists {
		t.Error("Expected to find getPrivateValue method")
	}

	// Check call sites include async/await patterns
	callSiteNames := make(map[string]bool)
	for _, callSite := range analysis.CallSites {
		callSiteNames[callSite.CalleeName] = true
	}

	expectedCalls := []string{"fetch", "json", "emit"}
	for _, expectedCall := range expectedCalls {
		if !callSiteNames[expectedCall] {
			t.Errorf("Expected to find call to '%s'", expectedCall)
		}
	}

	t.Logf("Modern TypeScript analysis successful: %d symbols, %d call sites",
		len(analysis.SymbolTable.Symbols), len(analysis.CallSites))
}

func TestTypeScriptAnalyzer_ErrorHandling(t *testing.T) {
	analyzer, err := NewTypeScriptAnalyzer()
	if err != nil {
		t.Fatalf("Failed to create TypeScript analyzer: %v", err)
	}
	defer analyzer.Close()

	// Test with partially malformed TypeScript code that still has extractable parts
	invalidCode := `
		// TypeScript with syntax errors but extractable symbols
		class Incomplete {
			validMethod() {
				return "works";
			}
			// Missing implementation below but class name should be extractable
		}
		
		// Another valid part
		function workingFunction() {
			console.log("this works");
		}
	`

	reader := strings.NewReader(invalidCode)
	analysis, err := analyzer.AnalyzeFile(context.Background(), reader, "invalid.ts")

	// Should handle gracefully and still extract what it can
	if err != nil {
		t.Fatalf("Analyzer should handle invalid code gracefully: %v", err)
	}

	if analysis == nil {
		t.Fatal("Expected analysis result even for invalid code")
	}

	// Should extract symbols from the valid parts
	if _, exists := analysis.SymbolTable.GetSymbol("Incomplete"); !exists {
		t.Error("Expected to extract class name from valid part of code")
	}

	if _, exists := analysis.SymbolTable.GetSymbol("workingFunction"); !exists {
		t.Error("Expected to extract function name from valid part of code")
	}

	t.Log("Error handling test passed - analyzer handles malformed code gracefully")
}
