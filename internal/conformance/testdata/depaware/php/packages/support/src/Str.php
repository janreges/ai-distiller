<?php

namespace App\Support;

// App\Text is mapped only by the root composer.json, not by this package's.
use App\Text\Inflector;

class Str
{
    public static function title(string $value): string
    {
        return Inflector::capitalize($value);
    }
}
