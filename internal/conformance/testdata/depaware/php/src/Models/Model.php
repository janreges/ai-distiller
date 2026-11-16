<?php

namespace App\Models;

abstract class Model
{
    use \App\Concerns\Timestamps;

    protected string $name = '';
}
