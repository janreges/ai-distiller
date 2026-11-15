<?php

namespace Conf;

use DateTimeImmutable;

// CONF_LINE_COMMENT

/** CONF_DOC_COMMENT */
class ConfWidget {
    public int $confField = 1;

    public function confPublic(): string {
        $a = "CONF_BODY_A";
        return $a;
    }

    /** CONF_PRIVATE_DOC */
    private function confPrivate(): string {
        return (new DateTimeImmutable())->format("Y");
    }

    public function confOther(): string {
        $b = "CONF_BODY_B";
        return $b;
    }
}
