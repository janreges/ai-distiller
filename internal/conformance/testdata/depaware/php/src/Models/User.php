<?php

namespace App\Models;

use App\Contracts\{Arrayable, Jsonable as Json};
use App\Support\Str;
use function App\Support\helper;
use Vendor\Package\Thing;

/*
use App\Ghost;
*/

require_once __DIR__ . '/../bootstrap.php';

class User extends Model implements Arrayable, Json
{
    use Concerns\HasName;

    public function title(): string
    {
        return Str::title($this->name) . Thing::class;
    }

    public function toArray(): array
    {
        return ['name' => $this->name];
    }

    public function toJson(): string
    {
        return json_encode($this->toArray());
    }
}
