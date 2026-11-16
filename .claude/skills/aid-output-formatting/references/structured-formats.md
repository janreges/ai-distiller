# Structured formats: jsonl, json-structured, xml

What the three formats that print the IR (not the text) look like at this
commit, and what each leaves out. They are produced by
`internal/formatter/jsonl.go#JSONLFormatter`,
`internal/formatter/json_structured.go#JSONStructuredFormatter` and
`internal/formatter/xml.go#XMLFormatter`. `md` is not here: it is the text
output in a code fence (`SKILL.md`, section 2).

The CLI passes these formatters only `ProcessingInfo` and the debug level; it
never sets `IncludeLocation`, `IncludeMetadata` or `Compact` in
`internal/formatter/formatter.go#Options`. So the output has no source
locations and no file metadata (`internal/conformance/cli_test.go#TestStructuredFormatsDoNotLeakMetadata`),
and a function body is printed whenever the stripper kept it. None of the
three runs the import filter: they show every import the stripper kept.

## The example

```bash
sh .claude/skills/aid-output-formatting/scripts/format-matrix.sh internal/conformance/testdata/java/ConfWidget.java
```

The fixture has a package, `import java.io.File`, a line comment, a doc
comment, and `public class ConfWidget` with a field, three public methods (one
`throws java.io.IOException`), a private method and a private enum. With
default flags the private members, the bodies and the line comment are
stripped. The text output for comparison:

```text
<file path="ConfWidget.java">
package com.example.conf
/**
 * CONF_DOC_COMMENT
 */

public class ConfWidget {
    public int confField = 1;
    public String confPublic();
    public String confOther();
    public String confThrows() throws java.io.IOException;
}
</file>
```

## jsonl

```json
{"language":"java","path":"ConfWidget.java","stats":{"class":1,"comment":1,"field":1,"function":3,"import":1,"package":1},"type":"file","version":"1.0"}
{"file":"ConfWidget.java","name":"com.example.conf","path":[],"type":"package"}
{"file":"ConfWidget.java","import_type":"import","module":"java.io.File","name":"java.io.File","path":[],"symbols":[{"name":"File"}],"type":"import"}
{"file":"ConfWidget.java","format":"doc","name":"comment","path":[],"text":"CONF_DOC_COMMENT","type":"comment"}
{"file":"ConfWidget.java","name":"ConfWidget","path":[],"type":"class","visibility":"public"}
{"default":"1","field_type":"int","file":"ConfWidget.java","name":"confField","path":["ConfWidget"],"type":"field","visibility":"public"}
{"file":"ConfWidget.java","name":"confPublic","parameters":[],"path":["ConfWidget"],"returns":"String","type":"function","visibility":"public"}
{"file":"ConfWidget.java","name":"confOther","parameters":[],"path":["ConfWidget"],"returns":"String","type":"function","visibility":"public"}
{"file":"ConfWidget.java","name":"confThrows","parameters":[],"path":["ConfWidget"],"returns":"String","type":"function","visibility":"public"}
```

Shape:

- Per file, a `"type":"file"` line with `path`, `language`, `version`,
  `stats` (how many nodes of each kind the file holds, at any depth) and
  `errors` when there are any; then one line per IR node, depth first in IR
  order. A directory repeats this per file.
- Every node line has `type` (the node kind), `file`, `path` (the names of the
  enclosing nodes, `[]` at the top level) and `name` (the module for an
  import, `comment` for a comment, the kind for a node without a name).
- Keys are in alphabetical order: each line is a Go map, and `encoding/json`
  sorts map keys.

Fields per kind (`internal/formatter/jsonl.go#JSONLFormatter.nodeToJSON`):

| Kind | Fields |
|---|---|
| `import` | `import_type`, `module`, `is_type`, `symbols` (`name`, `alias`, `is_type`) |
| `class` | `visibility`, `modifiers`, `extends`, `implements` |
| `interface` | `visibility`, `extends` |
| `function` | `visibility`, `modifiers`, `parameters` (always, `[]` when empty; each `name`, `type`, `default`, `optional`, `variadic`, `reference`), `returns`, `implementation` |
| `field` | `visibility`, `modifiers`, `field_type`, `default` |
| `type_alias` | `visibility`, `alias_type` |
| `comment` | `format`, `text` |
| `package`, `enum`, `struct`, `raw_content` | nothing beyond the four common keys; an enum has no `visibility` |

`triage.sh` of `aid-testing-debugging` uses jsonl as its view of the IR; for a
field that is not in this table read the `-vvv` dump instead.

## json-structured

```json
{
  "language": "java",
  "path": "ConfWidget.java",
  "stats": {
    "class": 1,
    "comment": 1,
    "field": 1,
    "function": 3,
    "import": 1,
    "package": 1
  },
  "structure": {
    "classes": [
      {
        "members": {
          "fields": [
            {
              "default": "1",
              "name": "confField",
              "type": "int",
              "visibility": "public"
            }
          ],
          "methods": [
            {
              "name": "confPublic",
              "returns": "String",
              "visibility": "public"
            },
            {
              "name": "confOther",
              "returns": "String",
              "visibility": "public"
            },
            {
              "name": "confThrows",
              "returns": "String",
              "visibility": "public"
            }
          ]
        },
        "name": "ConfWidget",
        "visibility": "public"
      }
    ],
    "imports": [
      {
        "module": "java.io.File",
        "symbols": [
          {
            "name": "File"
          }
        ],
        "type": "import"
      }
    ],
    "packages": [
      {
        "name": "com.example.conf"
      }
    ]
  },
  "type": "file",
  "version": "1.0"
}
```

Shape:

- One file prints one object: `type` `file`, `path`, `language`, `version`,
  `structure`, `stats`, `errors` when there are any. A directory prints one
  object `{"type": "project", "files": [...], "total_stats": {...}}`.
  Indented by two spaces.
- `structure` groups the file's top-level nodes
  (`internal/formatter/json_structured.go#JSONStructuredFormatter.structureNodes`):
  `packages` (`name`), `imports` (`type`, `module`, `is_type`, `symbols`), `classes`
  (`name`, `visibility`, `modifiers`, `extends`, `implements`, `members`),
  `interfaces` (`name`, `visibility`, `extends`, `members`), `functions`
  (`name`, `visibility`, `modifiers`, `parameters` when there are any,
  `returns`, `implementation`), `variables` (top-level fields: `name`,
  `visibility`, `modifiers`, `type`, `default`) and `types` (type aliases:
  `name`, `visibility`, `type`). Empty groups are left out.
- `members` of a class or interface holds only `fields` and `methods`
  (`internal/formatter/json_structured.go#JSONStructuredFormatter.structureClassMembers`).

## xml

```xml
<?xml version="1.0" encoding="UTF-8"?>
<distilled>
  <file path="ConfWidget.java" language="java" version="1.0">
    <nodes>
      <package name="com.example.conf"/>
      <import type="import" module="java.io.File">
        <symbol name="File"/>
      </import>
      <comment format="doc"><![CDATA[CONF_DOC_COMMENT]]></comment>
      <class name="ConfWidget" visibility="public">
        <field name="confField" visibility="public" type="int" default="1"/>
        <function name="confPublic" visibility="public" returns="String"/>
        <function name="confOther" visibility="public" returns="String"/>
        <function name="confThrows" visibility="public" returns="String"/>
      </class>
    </nodes>
  </file>
</distilled>
```

Shape:

- One `<distilled>` root; one `<file path language version>` per file, with
  `<errors>` when there are any and the nodes under `<nodes>`, nested as in the
  IR (`internal/formatter/xml.go#XMLFormatter.formatNode`).
- Elements: `<package name>`, `<import type module is_type>` with `<symbol name alias is_type>`,
  `<class name visibility modifiers>`, `<interface name visibility>`,
  `<struct>`, `<enum>`, `<function name visibility modifiers returns>` with
  `<parameters>` (`<parameter name type default optional variadic reference>`) and
  `<implementation>`, `<field name visibility modifiers type default>`,
  `<type_alias name visibility type>`, `<comment format>`; any other kind is
  `<node kind="..."/>`. Modifiers are one space-separated attribute; an
  attribute with an empty value is left out.
- Comment text and bodies are in CDATA sections, the body as the IR holds it
  (with `--implementation=1` the Java bodies above keep the indentation of
  the source after their first line). `cdata` in `internal/formatter/xml.go`
  splits a literal `]]>` across two sections, so the XML stays well-formed.
- In the text format the `<file path="...">` header escapes `&`, `"`, `<`, `>`
  as `&amp;`, `&quot;`, `&lt;`, `&gt;` (`escapeFilePath`); paths without those
  characters print as is. In Markdown, the single-file
  `internal/formatter/markdown.go#MarkdownFormatter.Format` prints `file.Path`
  as is in its `###` heading; only
  `internal/formatter/markdown.go#MarkdownFormatter.FormatMultiple` reads the
  path back from the text header with `unescapeFilePath`.

## What each format leaves out

| IR content | text | jsonl | json-structured | xml |
|---|---|---|---|---|
| imports the filter removes | removed | kept | kept | kept |
| comments | printed | yes | no | yes |
| decorators and annotations | printed | no | no | no |
| type parameters (`<T>`), `throws` | printed | no | no | no |
| type arguments of a type (`List<U>`) | printed | the bare name (`List`) | the bare name | the bare name |
| `extends`, `implements` | printed | yes | yes | no |
| enums and structs | printed | yes | no | yes |
| members of a `DistilledPackage` (a namespace) | printed | yes | no, only its name | yes |
| a type nested in a class | printed | yes | no, only fields and methods | yes |
| `--raw` file content | printed | a `raw_content` line without the content | only `stats` | `<node kind="raw_content"/>` |

The table comes from the formatter code and from these runs:

```bash
sh .claude/skills/aid-output-formatting/scripts/format-matrix.sh internal/conformance/testdata/java/ConfWidget.java --private=1 --implementation=1
sh .claude/skills/aid-output-formatting/scripts/format-matrix.sh internal/conformance/testdata/typescript/sample.ts
```

In the first, the private `enum ConfColor` nested in the class is a line in
`jsonl` and an `<enum>` in `xml`, and missing from `json-structured`. In the
second, the TypeScript enums and the functions inside `namespace ConfSpace`
are missing from `json-structured`, while its `stats` still count them. Fields
none of the three prints include `Deprecated`, `Description`, `Extensions`,
`Mixins`, `Permits` and a comment's `Inner`.
