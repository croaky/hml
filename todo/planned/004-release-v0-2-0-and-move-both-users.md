# Release v0.2.0 and move both users

hml has two users and no deprecation window, so a breaking change costs
one coordinated afternoon rather than a major version. Spend it
deliberately.

cibot is on v0.1.3 and EDS is on v0.1.1. Both pin a tag, so neither
moves until someone moves it, and neither is broken while this waits.

## What ships together

`001` and `002` are additive. `003` is not: it turns markup that renders
today into a render error or a sentinel. All three go out as one tag, so
each user reads one changelog and does one bump.

v0.2.0, not v0.1.4. The number is free and the break is real.

## Order

1. Land `001`, `002`, `003` on `main`, each its own change.
2. `scripts/tag v0.2.0`, which pushes the tag to the module path and
   nowhere else. cibot holds no tags: it mirrors nothing, so a tag is a
   deliberate act rather than something a push carries along.
3. cibot bumps. It should be a clean upgrade -- see the compatibility
   note in `003` -- and it is the smaller of the two, so it is the
   better canary.
4. EDS bumps, with the `on*` and `style` migration `003` describes. This
   is the real work, and it is EDS's to schedule.

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
