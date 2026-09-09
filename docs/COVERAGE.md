# Action Coverage Matrix

> Reflects the action catalog as of **2026-08-23**: 340 actions (98 wired, 117 read, 123 blocked, 2 excluded).
>
> Generated from `catalogPrimitives` in `internal/quickbooks/cli/actions.go` — the same
> source that feeds `qb actions`. Regenerate after any grammar, wiring, or mode change.
> Filter the live catalog with `qb actions --mode read|wired|blocked|excluded`; `qb doctor`
> reports the same totals as its informational `coverage` check.

Modes: **wired** executes against QBO honouring flags · **read** is read-only ·
**blocked** documents flags then returns not-wired · **excluded** has no command by policy.

## Totals

| mode | actions |
|------|--------:|
| wired | 98 |
| read | 117 |
| blocked | 123 |
| excluded | 2 |
| **total** | **340** |

## By domain

| domain | wired | read-only | blocked | excluded | total |
|---|---|---|---|---|---|
| accounting | 27 | 22 | 1 | 0 | 50 |
| advanced | 0 | 4 | 14 | 1 | 19 |
| company | 5 | 12 | 20 | 1 | 38 |
| customers | 3 | 13 | 4 | 0 | 20 |
| expenses | 20 | 13 | 16 | 0 | 49 |
| feed | 7 | 9 | 9 | 0 | 25 |
| inventory | 7 | 8 | 4 | 0 | 19 |
| payroll | 3 | 11 | 22 | 0 | 36 |
| reports | 0 | 6 | 9 | 0 | 15 |
| sales | 26 | 13 | 11 | 0 | 50 |
| tax | 0 | 6 | 13 | 0 | 19 |
| **total** | 98 | 117 | 123 | 2 | **340** |

## Matrix: domain × verb

### accounting

| verb | wired | read-only | blocked | excluded | total |
|---|---|---|---|---|---|
| create | 10 | 0 | 1 | 0 | 11 |
| delete | 7 | 0 | 0 | 0 | 7 |
| get | 0 | 17 | 0 | 0 | 17 |
| search | 0 | 5 | 0 | 0 | 5 |
| update | 10 | 0 | 0 | 0 | 10 |

### advanced

| verb | wired | read-only | blocked | excluded | total |
|---|---|---|---|---|---|
| (none) | 0 | 0 | 0 | 1 | 1 |
| create | 0 | 0 | 5 | 0 | 5 |
| delete | 0 | 0 | 1 | 0 | 1 |
| get | 0 | 4 | 0 | 0 | 4 |
| import | 0 | 0 | 1 | 0 | 1 |
| run | 0 | 0 | 2 | 0 | 2 |
| update | 0 | 0 | 4 | 0 | 4 |
| verify | 0 | 0 | 1 | 0 | 1 |

### company

| verb | wired | read-only | blocked | excluded | total |
|---|---|---|---|---|---|
| (none) | 0 | 0 | 0 | 1 | 1 |
| create | 2 | 0 | 4 | 0 | 6 |
| delete | 2 | 0 | 3 | 0 | 5 |
| edit | 0 | 0 | 1 | 0 | 1 |
| get | 0 | 11 | 0 | 0 | 11 |
| import | 0 | 0 | 5 | 0 | 5 |
| revalue | 0 | 0 | 1 | 0 | 1 |
| run | 0 | 1 | 1 | 0 | 2 |
| update | 1 | 0 | 5 | 0 | 6 |

### customers

| verb | wired | read-only | blocked | excluded | total |
|---|---|---|---|---|---|
| create | 1 | 0 | 2 | 0 | 3 |
| delete | 1 | 0 | 0 | 0 | 1 |
| edit | 1 | 0 | 0 | 0 | 1 |
| get | 0 | 7 | 0 | 0 | 7 |
| merge | 0 | 0 | 1 | 0 | 1 |
| run | 0 | 0 | 1 | 0 | 1 |
| search | 0 | 6 | 0 | 0 | 6 |

### expenses

| verb | wired | read-only | blocked | excluded | total |
|---|---|---|---|---|---|
| bounce | 0 | 0 | 1 | 0 | 1 |
| create | 7 | 0 | 4 | 0 | 11 |
| delete | 6 | 0 | 3 | 0 | 9 |
| edit | 4 | 0 | 0 | 0 | 4 |
| export | 0 | 0 | 1 | 0 | 1 |
| get | 0 | 8 | 0 | 0 | 8 |
| import | 0 | 0 | 2 | 0 | 2 |
| merge | 0 | 0 | 1 | 0 | 1 |
| pay | 1 | 0 | 0 | 0 | 1 |
| recategorise | 0 | 0 | 1 | 0 | 1 |
| search | 0 | 5 | 0 | 0 | 5 |
| split | 0 | 0 | 1 | 0 | 1 |
| update | 2 | 0 | 2 | 0 | 4 |

### feed

| verb | wired | read-only | blocked | excluded | total |
|---|---|---|---|---|---|
| attach | 0 | 0 | 1 | 0 | 1 |
| batch-accept | 1 | 0 | 0 | 0 | 1 |
| categorise | 1 | 0 | 0 | 0 | 1 |
| create | 0 | 0 | 1 | 0 | 1 |
| delete | 0 | 0 | 1 | 0 | 1 |
| exclude | 1 | 0 | 0 | 0 | 1 |
| excluded | 0 | 1 | 0 | 0 | 1 |
| get | 0 | 3 | 0 | 0 | 3 |
| import | 1 | 0 | 0 | 0 | 1 |
| list | 0 | 2 | 0 | 0 | 2 |
| match | 1 | 0 | 0 | 0 | 1 |
| population | 0 | 1 | 0 | 0 | 1 |
| posted | 0 | 1 | 0 | 0 | 1 |
| run | 0 | 0 | 2 | 0 | 2 |
| search | 0 | 1 | 0 | 0 | 1 |
| split | 1 | 0 | 0 | 0 | 1 |
| tag | 0 | 0 | 1 | 0 | 1 |
| transfer | 0 | 0 | 1 | 0 | 1 |
| undo-excluded | 1 | 0 | 0 | 0 | 1 |
| unpost | 0 | 0 | 1 | 0 | 1 |
| update | 0 | 0 | 1 | 0 | 1 |

### inventory

| verb | wired | read-only | blocked | excluded | total |
|---|---|---|---|---|---|
| create | 3 | 0 | 2 | 0 | 5 |
| delete | 2 | 0 | 0 | 0 | 2 |
| edit | 1 | 0 | 0 | 0 | 1 |
| get | 0 | 4 | 0 | 0 | 4 |
| partial | 0 | 0 | 1 | 0 | 1 |
| search | 0 | 4 | 0 | 0 | 4 |
| update | 1 | 0 | 1 | 0 | 2 |

### payroll

| verb | wired | read-only | blocked | excluded | total |
|---|---|---|---|---|---|
| create | 1 | 0 | 11 | 0 | 12 |
| delete | 1 | 0 | 0 | 0 | 1 |
| finalise | 0 | 0 | 1 | 0 | 1 |
| get | 0 | 8 | 0 | 0 | 8 |
| reset | 0 | 0 | 1 | 0 | 1 |
| run | 0 | 0 | 3 | 0 | 3 |
| search | 0 | 3 | 0 | 0 | 3 |
| update | 1 | 0 | 6 | 0 | 7 |

### reports

| verb | wired | read-only | blocked | excluded | total |
|---|---|---|---|---|---|
| create | 0 | 0 | 5 | 0 | 5 |
| delete | 0 | 0 | 1 | 0 | 1 |
| get | 0 | 6 | 0 | 0 | 6 |
| update | 0 | 0 | 3 | 0 | 3 |

### sales

| verb | wired | read-only | blocked | excluded | total |
|---|---|---|---|---|---|
| copy | 2 | 0 | 0 | 0 | 2 |
| create | 8 | 0 | 5 | 0 | 13 |
| customise | 0 | 0 | 1 | 0 | 1 |
| delete | 7 | 0 | 1 | 0 | 8 |
| discount | 1 | 0 | 0 | 0 | 1 |
| edit | 1 | 0 | 0 | 0 | 1 |
| get | 0 | 9 | 0 | 0 | 9 |
| progress | 0 | 0 | 1 | 0 | 1 |
| remind | 0 | 0 | 1 | 0 | 1 |
| search | 0 | 4 | 0 | 0 | 4 |
| send | 0 | 0 | 1 | 0 | 1 |
| update | 6 | 0 | 1 | 0 | 7 |
| write-off | 1 | 0 | 0 | 0 | 1 |

### tax

| verb | wired | read-only | blocked | excluded | total |
|---|---|---|---|---|---|
| create | 0 | 0 | 4 | 0 | 4 |
| delete | 0 | 0 | 2 | 0 | 2 |
| export | 0 | 0 | 2 | 0 | 2 |
| get | 0 | 5 | 0 | 0 | 5 |
| run | 0 | 0 | 3 | 0 | 3 |
| search | 0 | 1 | 0 | 0 | 1 |
| update | 0 | 0 | 2 | 0 | 2 |

## Verb inventory

| verb | domains | wired | read-only | blocked | excluded | total |
|---|---|---|---|---|---|---|
| get | 11 | 0 | 82 | 0 | 0 | 82 |
| create | 11 | 32 | 0 | 44 | 0 | 76 |
| update | 10 | 21 | 0 | 25 | 0 | 46 |
| delete | 11 | 26 | 0 | 12 | 0 | 38 |
| search | 8 | 0 | 29 | 0 | 0 | 29 |
| run | 6 | 0 | 1 | 12 | 0 | 13 |
| import | 4 | 1 | 0 | 8 | 0 | 9 |
| edit | 5 | 7 | 0 | 1 | 0 | 8 |
| export | 2 | 0 | 0 | 3 | 0 | 3 |
| (none) | 2 | 0 | 0 | 0 | 2 | 2 |
| copy | 1 | 2 | 0 | 0 | 0 | 2 |
| list | 1 | 0 | 2 | 0 | 0 | 2 |
| merge | 2 | 0 | 0 | 2 | 0 | 2 |
| split | 2 | 1 | 0 | 1 | 0 | 2 |
| attach | 1 | 0 | 0 | 1 | 0 | 1 |
| batch-accept | 1 | 1 | 0 | 0 | 0 | 1 |
| bounce | 1 | 0 | 0 | 1 | 0 | 1 |
| categorise | 1 | 1 | 0 | 0 | 0 | 1 |
| customise | 1 | 0 | 0 | 1 | 0 | 1 |
| discount | 1 | 1 | 0 | 0 | 0 | 1 |
| exclude | 1 | 1 | 0 | 0 | 0 | 1 |
| excluded | 1 | 0 | 1 | 0 | 0 | 1 |
| finalise | 1 | 0 | 0 | 1 | 0 | 1 |
| match | 1 | 1 | 0 | 0 | 0 | 1 |
| partial | 1 | 0 | 0 | 1 | 0 | 1 |
| pay | 1 | 1 | 0 | 0 | 0 | 1 |
| population | 1 | 0 | 1 | 0 | 0 | 1 |
| posted | 1 | 0 | 1 | 0 | 0 | 1 |
| progress | 1 | 0 | 0 | 1 | 0 | 1 |
| recategorise | 1 | 0 | 0 | 1 | 0 | 1 |
| remind | 1 | 0 | 0 | 1 | 0 | 1 |
| reset | 1 | 0 | 0 | 1 | 0 | 1 |
| revalue | 1 | 0 | 0 | 1 | 0 | 1 |
| send | 1 | 0 | 0 | 1 | 0 | 1 |
| tag | 1 | 0 | 0 | 1 | 0 | 1 |
| transfer | 1 | 0 | 0 | 1 | 0 | 1 |
| undo-excluded | 1 | 1 | 0 | 0 | 0 | 1 |
| unpost | 1 | 0 | 0 | 1 | 0 | 1 |
| verify | 1 | 0 | 0 | 1 | 0 | 1 |
| write-off | 1 | 1 | 0 | 0 | 0 | 1 |
