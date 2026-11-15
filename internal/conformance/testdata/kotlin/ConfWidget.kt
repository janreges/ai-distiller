package com.example.conf

import java.io.File

// CONF_LINE_COMMENT

/** CONF_DOC_COMMENT */
class ConfWidget {
    val confField: Int = 1

    fun confPublic(): String {
        val a = "CONF_BODY_A"
        return a
    }

    /** CONF_PRIVATE_DOC */
    private fun confPrivate(): String {
        return File.separator
    }

    fun confOther(): String {
        val b = "CONF_BODY_B"
        return b
    }
}
