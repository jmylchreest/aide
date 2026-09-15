# Frozen aide source fixture

This bounded snapshot copies production source and original usage tests unchanged
from aide commit `58816d9`. The experiment's external provenance manifest records
each original path and content hash. TypeScript files include the relative import
closure of the two host adapters; seven Go files document the downstream writer,
storage and accounting contracts. This is a navigation fixture, not a complete
build of the aide application. Do not install dependencies or access the network.

Original `src/test/*.test.ts` files use Vitest and are included as source evidence.
Implementation trials receive a separate, clearly labeled Bun harness and visible
tests. When that overlay is present, run `bun test tests` from this directory.
