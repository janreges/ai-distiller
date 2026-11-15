package com.example.nested

/**
 * Declarations nested in a class, an object, an interface and an enum:
 * functional interfaces, enums, sealed interfaces, interface properties
 * and companion objects.
 */

/** Callbacks a caller registers. */
object Callbacks {
    fun register(done: Done) {}

    /** Called when the work is done. */
    fun interface Done {
        fun onDone(ok: Boolean)
    }
}

class Screen {
    /** Maps one value to another. */
    fun interface Mapper {
        fun map(x: Int): Int
    }

    /** The colour scheme. */
    enum class Mode {
        LIGHT, DARK;

        fun next(): Mode = if (this == LIGHT) DARK else LIGHT
    }

    /** What the screen shows. */
    sealed interface State {
        /** A label for the state. */
        val label: String

        /** Nothing to show yet. */
        data object Idle : State

        /** Work in progress. */
        data class Busy(override val label: String, val progress: Int) : State
    }

    fun render(mode: Mode, state: State, mapper: Mapper) {}

    private interface Renderer {
        fun draw(state: State)
    }
}

/** A store of strings. */
interface Repository {
    /** The number of entries. */
    val size: Int
    var name: String

    fun get(id: Int): String

    /** Keeps the entries in memory. */
    class InMemory(override val size: Int, override var name: String) : Repository {
        override fun get(id: Int): String = name
    }
}

enum class Level(val weight: Int) {
    LOW(1), HIGH(2);

    fun heavier(): Boolean = weight > 1

    companion object {
        fun parse(s: String): Level = valueOf(s)
    }
}
