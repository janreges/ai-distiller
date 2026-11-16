<?php

namespace App\Models\Concerns;

trait HasName
{
    public function rename(string $name): void
    {
        $this->name = $name;
    }
}
