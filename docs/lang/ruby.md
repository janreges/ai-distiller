# Ruby Language Support

AI Distiller supports Ruby codebases through the [tree-sitter-ruby](https://github.com/tree-sitter/tree-sitter-ruby) grammar, compiled into the `aid` binary. It extracts classes, modules, methods, attribute accessors, constants and mixins; metaprogramming is not expanded.

## Overview

Ruby support in AI Distiller captures the essential structure of Ruby code including classes, modules, methods, and constants. The text output uses Ruby keywords (`class`, `module`, `def`) with members indented under their class, and prints no `end`.

## Recent Fixes

- **Visibility sections**: `private`, `protected` and `public` sections apply to the methods below them; `private :name`, `private_class_method :name` and `private attr_reader :x` are honoured
- **require, attr_\* and include**: `require` lines, `attr_*` accessors and `include` are kept
- **Comments**: a run of `#` comments directly above a `class`, `module` or `def` is its documentation and prints by default (`--docstrings=0` removes it); other comments print with `--comments=1`
- **Signatures**: splat (`*args`), double splat (`**opts`) and block (`&blk`) parameters, and scoped class names (`class Api::V1::Users`)

1. **Missing `def` keyword** (✅ Fixed)
   - **Issue**: Methods were displayed without the `def` keyword
   - **Fix**: Updated Ruby formatter to include proper method syntax
   - **Impact**: Correct Ruby method declarations in output

2. **Python-style colons** (✅ Fixed)
   - **Issue**: Classes and modules had Python-style colons after declarations
   - **Fix**: Removed colon from class/module formatting
   - **Impact**: Idiomatic Ruby syntax

## Supported Ruby Constructs

### Core Language Features

| Construct | Support Level | Notes |
|-----------|--------------|-------|
| **Classes** | ✅ Full | Including nested classes and scoped names (`class Api::V1::Users`) |
| **Modules** | ✅ Full | Including module nesting |
| **Methods** | ⚠️ Partial | Instance and class methods (`self.`); default values print as `...` (`b = ...`, `opt:`) |
| **Attr Accessors** | ✅ Full | `attr_reader :id` prints as `def id`, `attr_writer :price` as `def price=(value)` |
| **Constants** | ⚠️ Partial | Name only, without the value |
| **Instance Variables** | ✅ Full | `@id` from `attr_*`, shown as private (with `--private=1`) |
| **Class Variables** | ✅ Full | `@@class_var` |
| **Singleton Methods** | ⚠️ Partial | `def self.method` works; methods in a `class << self` block print under `class <<self>>` without `self.` |
| **Private/Protected** | ✅ Full | Sections, `private :name`, `private_class_method :name`, `private attr_reader :x` |
| **Include/Extend** | ⚠️ Partial | `include` is printed; `extend` and `prepend` are not |
| **Alias** | ✅ Full | `alias` and `alias_method` print as a method (`def run`) |
| **Inheritance** | ✅ Full | Class < SuperClass |
| **Blocks** | ✅ Full | `&block` parameters in method signatures |
| **Metaprogramming** | ❌ Not expanded | `define_method`, `method_missing` and DSL calls (`included do`, `scope`) are not printed |

### Visibility Rules

Ruby visibility in AI Distiller:
- **public**: Default visibility (not marked); methods below `public` are public again
- **private**: Methods below `private`, or named by `private :name` / `private_class_method :name` (shown with `--private=1`)
- **protected**: Methods below `protected` (shown with `--protected=1`)
- **module_function**: The methods stay plain instance methods (`def helper`, not `def self.helper`)

With `--private=1` or `--protected=1`, the output marks each such method with a `# private` or `# protected` line above it.

## Key Features

### 1. **Method Visibility**

AI Distiller handles Ruby's visibility sections:

```ruby
class User
  def public_method
    "public"
  end
  
  private
  
  def private_method
    "private"
  end
  
  protected
  
  def protected_method
    "protected"
  end
end
```

```
<file path="user.rb">

class User
    def public_method
</file>
```

### 2. **Module Mixins**

`include` is preserved; `extend` is not printed:

```ruby
module Trackable
  def track(event)
    # tracking logic
  end
end

class Order
  include Trackable
  extend ActiveModel::Naming
  
  def process
    track(:order_processed)
  end
end
```

```
<file path="order.rb">

module Trackable
    def track(event)

class Order
    include Trackable
    def process
</file>
```

### 3. **Class Methods**

`def self.name` prints as a class method; a `class << self` block prints as a nested `class <<self>>` whose methods lack `self.`:

```ruby
class Calculator
  def self.add(a, b)
    a + b
  end
  
  class << self
    def multiply(a, b)
      a * b
    end
  end
end
```

```
<file path="calculator.rb">

class Calculator
    def self.add(a, b)

    class <<self>>
        def multiply(a, b)
</file>
```

### 4. **Attr Accessors**

Ruby's `attr_*` declarations print as the methods they define:

```ruby
class Product
  attr_reader :id, :name
  attr_writer :price
  attr_accessor :quantity
  
  def initialize(id, name)
    @id = id
    @name = name
  end
end
```

```
<file path="product.rb">

class Product
    def id
    def name
    def price=(value)
    def quantity
    def quantity=(value)
    def initialize(id, name)
</file>
```

## Output Format

### Text Format (Recommended for AI)

The text format uses Ruby keywords. The `extend` line, the `included do` block, the class-level DSL calls (`protect_from_forgery`, `rescue_from`) and the private methods are left out of the default output:

```ruby
module Authentication
  extend ActiveSupport::Concern
  
  included do
    before_action :authenticate_user!
  end
  
  def authenticate_user!
    redirect_to login_path unless current_user
  end
  
  def current_user
    @current_user ||= User.find_by(id: session[:user_id])
  end
  
  private
  
  def set_current_user(user)
    @current_user = user
    session[:user_id] = user.id
  end
end

class ApplicationController < ActionController::Base
  include Authentication
  
  protect_from_forgery with: :exception
  
  rescue_from ActiveRecord::RecordNotFound do |e|
    render_404
  end
  
  private
  
  def render_404
    render file: 'public/404.html', status: :not_found
  end
end
```

Output (default - public only, no implementation):

```
<file path="auth.rb">

module Authentication
    def authenticate_user!
    def current_user

class ApplicationController < ActionController::Base
    include Authentication
</file>
```

## Known Limitations

1. **`extend` and `prepend`**: not printed
2. **`class << self`**: printed as a nested `class <<self>>` whose methods lack `self.`
3. **Metaprogramming**: `define_method`, `method_missing` and DSL calls (`included do`, `scope`, `has_many`) are not printed
4. **Default values**: a default argument prints as `...` (`b = ...`) and a keyword default is dropped (`opt: 1` prints as `opt:`)
5. **Constants**: printed without their value
6. **`require`**: the text format's import filter removes a `require` whose name does not appear in the distilled output; `--filter-imports=false` keeps every `require`

## Best Practices

### 1. **Use Explicit Visibility**

Group methods by visibility:

```ruby
class Service
  # Public interface
  def call(params)
    validate(params)
    process(params)
  end
  
  private
  
  # Implementation details
  def validate(params)
    # ...
  end
  
  def process(params)
    # ...
  end
end
```

### 2. **Module Organization**

Use modules for shared behavior:

```ruby
module Concerns
  module Searchable
    extend ActiveSupport::Concern
    
    included do
      scope :search, ->(query) { where("name LIKE ?", "%#{query}%") }
    end
    
    class_methods do
      def searchable_fields
        %w[name description]
      end
    end
  end
end
```

### 3. **Documentation**

Use YARD-style comments for better AI understanding; a `#` comment block directly above a `def`, `class` or `module` prints by default:

```ruby
# Processes payment for an order
# @param order [Order] the order to process
# @param payment_method [String] the payment method to use
# @return [PaymentResult] the result of the payment
def process_payment(order, payment_method)
  # ...
end
```

## Integration Examples

### Direct CLI Usage

```bash
# Extract public API (the defaults)
aid app/ --stdout > api.txt

# Include all methods
aid app/ --private=1 --protected=1 --output full.txt

# Rails models only
aid app/models --stdout > models.txt
```

### Rails Projects

```bash
# Controllers API
aid app/controllers --stdout > controllers-api.txt

# Models (associations such as has_many are DSL calls and are not printed)
aid app/models --stdout > models.txt

# Service objects with their method bodies
aid app/services --implementation=1 --stdout > services.txt
```

## Future Improvements

- `extend`, `prepend` and `class << self`
- Metaprogramming expansion
- Better heredoc handling
- DSL recognition (RSpec, Rails, etc.)
- Refinements support

## Contributing

Ruby support is actively maintained. Key areas for contribution:
- Complex metaprogramming patterns
- DSL parsing (Rails, RSpec, etc.)
- Block and proc handling
- Performance optimizations

See [CONTRIBUTING.md](../../CONTRIBUTING.md) for development setup.
