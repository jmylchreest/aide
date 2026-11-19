# Release publication recovery

Use the current `release.yml` workflow to finish npm publication for an existing
stable GitHub release when the original CI artifacts have expired:

```sh
gh workflow run release.yml --ref main -f recover_npm_tag=v0.2.1
```

The recovery path downloads the original main npm tarball, six binaries, and
`checksums.txt` from that release. It verifies the checksums and available GitHub
asset digests, checks package identities and dependency versions, and packs the
binary packages using manifests from the original tag. It does not rebuild
runtime code, move the tag, or replace GitHub release assets.

The normal publishing job then publishes binaries before the main package and
verifies registry availability. Publication uses GitHub OIDC without an npm token
fallback, so all seven npm packages must trust `jmylchreest/aide`, `release.yml`,
with direct publishing enabled. Recovery refuses to move `latest` backwards.

Dispatch from the branch containing the workflow fix. Rerunning the original
failed run uses its original workflow, which does not pick up newer fixes.

Run the helper's integrity checks locally with:

```sh
python3 -m unittest discover -s .github/scripts -p 'test_*.py'
```
