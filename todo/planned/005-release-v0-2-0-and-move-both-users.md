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
3. cibot bumps: replace the local `replace` with the pin, and `go mod
   tidy`, which is also what drops hml's wrong `// indirect`.
4. EDS bumps the same way, then staging before production. It is the one
   with fixtures and the one whose migration this sized.

## Testing against a user before tagging

Neither user should discover a break from a tag. `scripts/tag` refuses
a dirty tree and a `main` behind the farmer's, but it cannot know
whether the code is any good to the two repos that will fetch it. So
before step 2, each was pointed at the working tree:

```sh
go mod edit -replace github.com/croaky/hml=/path/to/hml/worktree
go build ./... && go test ./...
```

Both pass, and doing it first is what found the two things below. The
`replace` is a local experiment and must not be committed.

The output change cost cibot 17 assertions, all of them a tag and its
lone line of text written as three. Mechanical, but only visible by
running it.

EDS's own migration was wrong in a way its build could not show. It
marked three per-row `onerror` values `SafeJS` in the handler, and
`/people/show` marshals its whole page struct to JSON and back into a
`map[string]any` before rendering -- which erases a Go type, so the
value arrived as a plain string and the policy refused it. The fix was
to delete those three: each one paired one fallback with one partial, so
the partial reads the marked global it was already next to, and 34
handler keys nobody read went with them. A round trip through JSON is
worth remembering as a place trust types die quietly.

## A note on version discipline

`003` will not be the last time hml constrains something that used to
pass. A pre-1.0 library with two known users can keep doing this, but
the cheapness depends on knowing every user. If a third appears, that
assumption is gone, and the next constraint needs a deprecation path
instead of an afternoon.
