# Release v0.2.0 and move both users

hml has two users and no deprecation window, so a breaking change costs
one coordinated afternoon rather than a major version. Spend it
deliberately.

cibot is on v0.1.3 and EDS is on v0.1.1. Both pin a tag, so neither
moves until someone moves it, and neither is broken while this waits.

## What ships together

The field-access fix and the two reporting methods were additive.
`003` was not: it turns markup that renders today into a render error or
a sentinel, and `004` is the correction to how it decides. They go out
as one tag, so each user reads one changelog and does one bump.

The lone-text-child change goes out with them, and it is the one a test
suite notices: every tag whose only child is text now closes on the
opening tag's line. No markup changes meaning, but anything comparing
rendered HTML to a fixture sees a diff on most lines of it.

v0.2.0, not v0.1.4. The number is free and the break is real.

## Order

1. Land `004` on `main`. The three before it are already there.
2. `scripts/tag v0.2.0`, which pushes the tag to the module path and
   nowhere else. cibot holds no tags: it mirrors nothing, so a tag is a
   deliberate act rather than something a push carries along.
3. cibot bumps. The attribute policy was verified clean against it: no
   `on*` attributes, literal `style` values, URLs root-relative or
   hashed asset paths. That check predates the output change, so the
   rendered-HTML assertions are still to be looked at.
4. EDS bumps. Its migration is written and verified the same way: 94
   image fallbacks collapsed to two handler-marked locals, three
   per-row values typed, three mail styles built in Go. Same caveat,
   and it is the one with fixtures.

## Testing against a user before tagging

Neither user should discover a break from a tag. `scripts/tag` refuses
a dirty tree and a `main` behind the farmer's, but it cannot know
whether the code is any good to the two repos that will fetch it. Before
step 2, point each at the working tree:

```sh
go mod edit -replace github.com/croaky/hml=/Users/dcroak/hml
go build ./... && go test ./...
go mod edit -dropreplace github.com/croaky/hml
```

Run it in cibot and in EDS. The `replace` is a local experiment and must
not be committed; EDS's result is what sizes its migration, and it is
worth knowing before the tag exists rather than after.

## A note on version discipline

`003` will not be the last time hml constrains something that used to
pass. A pre-1.0 library with two known users can keep doing this, but
the cheapness depends on knowing every user. If a third appears, that
assumption is gone, and the next constraint needs a deprecation path
instead of an afternoon.
