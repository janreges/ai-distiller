// Level 3: GenericsAndInterfaces.java
package com.aidi.test.inheritance;

import java.util.List;
import java.util.Optional;

// 1. A generic interface
interface DataStore<T> {
    void save(T item);
    Optional<T> findById(String id);
    List<T> findAll();
}

// 2. A custom annotation
@interface Auditable {
    String value() default "DEFAULT";
}

// 3. An abstract class implementing the interface
abstract class BaseStore<T> implements DataStore<T> {
    @Override
    public void save(T item) {
        System.out.println("Saving item: " + item.toString());
        log(item);
    }

    // An abstract method to be implemented by subclasses
    protected abstract void log(T item);
}

// 4. A concrete implementation
public class UserStore extends BaseStore<User> {
    @Override
    public Optional<User> findById(String id) {
        // Mock implementation
        if ("1".equals(id)) {
            return Optional.of(new User("Admin"));
        }
        return Optional.empty();
    }

    @Override
    public List<User> findAll() {
        return List.of(new User("Admin"), new User("Guest"));
    }

    @Override
    @Auditable("USER_LOG") // Using the custom annotation
    protected void log(User item) {
        System.out.println("[AUDIT] " + item.getName());
    }
}

// 5. A simple class used as the generic type
class User {
    private String name;
    public User(String name) { this.name = name; }
    public String getName() { return name; }
    @Override public String toString() { return "User:" + name; }
}