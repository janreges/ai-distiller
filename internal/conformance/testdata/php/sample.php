<?php

namespace Conf;

use DateTimeImmutable;
use Conf\Base\ConfBase as ConfAliasBase;
use Conf\Contracts\{ConfShown as ConfAliasShown, ConfUnusedMember};
use function Conf\Helpers\conf_helper;
use const Conf\Helpers\CONF_LIMIT;

// CONF_LINE_COMMENT

/** CONF_DOC_COMMENT */
class ConfWidget extends ConfAliasBase implements ConfAliasShown {
    use ConfTrait;

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

    public function confCall(): int {
        return conf_helper(CONF_LIMIT);
    }

    public static function confMake(int ...$confIds): static {
        return new static();
    }

    public function &confRef(array &$confArr): array {
        return $confArr;
    }

    public function confNever(): never {
        exit;
    }
}

function conf_dnf((ConfA&ConfB)|null $confDnf): (ConfA&ConfB)|null {
    return $confDnf;
}
