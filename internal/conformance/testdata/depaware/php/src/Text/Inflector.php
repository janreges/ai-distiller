<?php

namespace App\Text;

class Inflector
{
    public static function capitalize(string $value): string
    {
        return ucfirst($value);
    }
}
