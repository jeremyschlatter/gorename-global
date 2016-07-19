# gorename-global
--
Command gorename-global is like gorename, but replaces multiple identifiers at
once.

The tradeoff is that gorename-global is less careful than gorename. It does not
scan packages other than the ones you name on the command line. It does not
check that the rename is safe.

It is still safer than using sed, though. It will only replace Go identifiers
that exactly match the --from argument.

You can use the --auto flag to fix any identifier that 'go lint' would flag.
