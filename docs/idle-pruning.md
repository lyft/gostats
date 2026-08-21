# Bounding memory from high-cardinality tags

By default, `gostats` never forgets a counter or timer name once it sees one, even after the value
stops changing. Don't tag one with a high-cardinality value: that grows memory without limit.

Set `GOSTATS_PRUNE_IDLE_SECONDS` to prune counters and timers that have gone that many seconds
without changing value. (A write that doesn't change the value - `Add(0)`, or `Set` with the value
it already holds - counts as idle, same as no write at all.) It is unset (disabled) by default, so
existing behavior does not change unless you opt in.

Pick a value comfortably longer than the slowest-firing counter or timer you still care about, not
a value that matches how long a leak takes to become noticeable - those are unrelated. A shorter
value bounds runaway cardinality more tightly, since each stale entry is reclaimed sooner regardless
of how long the underlying leak runs; the cost is more churn on any of your own counters or timers
that legitimately go quiet for stretches longer than the value you pick (each cycles through
prune-then-reattach, delaying that one report by up to a flush interval, though nothing is lost -
see below). If you haven't audited how infrequently your own stats can legitimately fire, err
longer:

```sh
export GOSTATS_PRUNE_IDLE_SECONDS=600  # prune anything idle for more than 10 minutes
```

A pruned entry is not gone for good. If your code holds a `Counter` or `Timer` in a struct field -
the common pattern - and writes to it again later, that write reattaches it to the store and it
resumes reporting correctly, including the value from the write that triggered the reattachment.
Nothing is lost. Gauges are never pruned: a gauge holds state a later read may depend on, and
pruning one could make it stop reporting.

The seconds value is converted to a number of flushes using your configured flush interval
(`GOSTATS_FLUSH_INTERVAL_SECONDS`, 5 by default), rounded up. If you drive `Start` with your own
ticker at a different period, pruning follows that many of your own flushes, not
`GOSTATS_PRUNE_IDLE_SECONDS` of wall-clock time.

Once enabled, two metrics are emitted (silent otherwise, so turning pruning on is what turns the
visibility on too):

* `gostats.tracked`, tagged `type=counter|gauge|timer` - how many of each are currently held.
  Alert on this climbing without leveling off: that means something is generating unique tag
  values faster than they go idle.
* `gostats.pruned`, tagged `type=counter|timer` - how many were removed on a given flush. A
  sustained high rate means high-cardinality tags are actively being generated and pruned away -
  the leak is contained, but the tag usage generating it is still worth fixing at the source.

Being gated on pruning means `gostats.tracked` can't tell you whether to turn pruning on in the
first place - only a service that already suspects a cardinality problem and has opted in gets to
watch it. Finding that problem before opting in is still a job for your metrics backend's own
cardinality tooling.
