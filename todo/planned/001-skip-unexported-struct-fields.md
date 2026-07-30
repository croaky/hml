# Skip unexported fields in struct access

A template typo can panic the process instead of erroring.

`buildFieldIndex` walks every field of a struct type and indexes it by
lower-cased name, json tag, and db tag. It does not ask whether the
field is exported. `evalField` then calls `rv.Field(idx).Interface()`,
which panics on an unexported field: "cannot return value obtained from
unexported field or method".

So a struct with an unexported field is a landmine. `.vars` on anything
shaped like hml's own `Context` reaches one. The template author sees a
panic with no file or line, and the app sees a dead handler where a
render error would have been a 500 and a log line.

This contradicts what the package already promises. `doc.go` says a
missing key or field is a render error "so a typo fails loudly instead
of rendering blank". Loud is right; a panic is too loud, and it names
neither the template nor the field.

## Proposed changes

Skip `!field.IsExported()` in `buildFieldIndex`. An unexported field
then never enters the index, `structFieldIndex` misses, and `evalField`
returns its existing `undefined field %q on struct %T`, which is the
answer a template author can act on.

A test that renders a field path onto a struct with an unexported field
of that name, asserting an error rather than a panic.

## Note

This is a behavior fix with no API change, and nothing depends on it.
Land it first because it is the smallest, and because the two plans
after it touch the same file.
