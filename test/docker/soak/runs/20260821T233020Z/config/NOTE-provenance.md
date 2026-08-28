`docker-compose.yml` was copied in by hand after the run started.

`soak.sh` reads and freezes `$here/docker-compose.yml`, but in this tree the
compose lives one level up at `test/docker/docker-compose.yml` (the script came
from synth-heal, where it sat beside the script). So the automatic freeze wrote
no compose file, and `heal_flags` / the effective drop patterns fell back to
defaults rather than being read from the compose. The fallbacks happen to be
correct for this run — healing is unconditional in the DI conductor and no drop
patterns are set — but the manifest's `synthetic drops` field is empty because
the file could not be read, not because it was measured as empty.

The file here is the one the run actually used. The script cannot be fixed while
it is executing (bash reads a script incrementally; editing it mid-run corrupts
execution), so the fix lands after this run ends.
