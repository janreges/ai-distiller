<?php

namespace App\Contracts;

interface Jsonable
{
    public function toJson(): string;
}
