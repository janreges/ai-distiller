// Level 2: SimpleOOP.java
package com.aidi.test.oop;

import java.util.Objects;

public class SimpleOOP {

    public final String id;
    protected String name;
    private int version;
    volatile boolean dirty; // Test for volatile keyword

    public SimpleOOP(String id, String name) {
        this.id = Objects.requireNonNull(id, "ID cannot be null");
        this.name = name;
        this.version = 1;
        this.dirty = true;
    }

    // Overloaded constructor
    public SimpleOOP(String id) {
        this(id, "Default Name");
    }

    public String getName() {
        return name;
    }

    public void setName(String name) {
        this.name = name;
        this.version++;
        this.dirty = true;
    }

    @Override
    public String toString() {
        return "SimpleOOP{" +
               "id='" + id + '\'' +
               ", name='" + name + '\'' +
               ", version=" + version +
               '}';
    }
}