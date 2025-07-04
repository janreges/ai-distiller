// Level 4: ModernJava.java
package com.aidi.test.modern;

import java.util.stream.Stream;

// 1. A sealed interface to control implementations
sealed interface Shape permits Circle, Rectangle {
    double area();
}

// 2. A record implementing the sealed interface
record Circle(double radius) implements Shape {
    // Canonical constructor is implicit.
    // Custom compact constructor for validation:
    public Circle {
        if (radius < 0) {
            throw new IllegalArgumentException("Radius cannot be negative");
        }
    }

    @Override
    public double area() {
        return Math.PI * radius * radius;
    }
}

// 3. A final class implementing the sealed interface
final class Rectangle implements Shape {
    private final double length;
    private final double width;

    public Rectangle(double length, double width) {
        this.length = length;
        this.width = width;
    }

    @Override
    public double area() {
        return length * width;
    }
}

public class ModernJava {
    public static void main(String[] args) {
        // 4. Using streams, lambdas, and method references
        var shapes = Stream.of(new Circle(10), new Rectangle(4, 5));

        shapes.map(Shape::area) // Method reference
              .filter(area -> area > 100.0) // Lambda expression
              .forEach(System.out::println); // Method reference
    }
}