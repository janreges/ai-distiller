<?php

namespace App\Concerns;

trait Timestamps
{
    public function touch(): bool
    {
        return true;
    }
}
