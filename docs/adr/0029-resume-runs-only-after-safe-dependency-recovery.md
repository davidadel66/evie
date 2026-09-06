# Resume runs only after safe dependency recovery

When a disabled, failed, or disconnected dependency becomes ready again, Evie
may automatically resume a paused Workflow Run only if no external effect has
an unknown outcome and no human interruption is outstanding. Outcome Unknown
must reconcile first, and a run awaiting human judgment stays paused. Recovery
of availability is not evidence that a prior request did or did not occur.
