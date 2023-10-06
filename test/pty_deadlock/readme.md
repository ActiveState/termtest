These tests are an attempt to narrow down a deadlock issue we've seen a few
times.

The basic repro steps are interacting with a terminal prompt repeatedly in a
loop. Eventually it will dealock on sending input. When this happens it is
locked on a write to stdin, while at the same time blocked on the executable
side on waiting for new stdin data to arrive (it never does).

The issue has only been reproduced with the Survey library so far. This doesn't
mean the survey lib is at fault though, it could also be because either the pty
or vt10x libraries we're using isn't respecting the same standard as the survey
lib.
