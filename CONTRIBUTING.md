# Contributing to Quark

Quark is source-available under the Business Source License 1.1, so
contributions follow two ground rules that keep the project's IP clean:

## 1. Contributor grant

By opening a PR you assign the right to use, relicense and commercially
license your contribution to the project Licensor (Fasad Salatov). This is
required because the Licensor sells commercial licenses for the Licensed
Work; contributions that cannot be relicensed cannot be merged.

## 2. Developer Certificate of Origin (DCO)

Every commit must be signed off:

```bash
git commit -s -m "feat: ..."
```

The `-s` flag adds a `Signed-off-by:` line certifying you wrote the code (or
have the right to submit it) under the [DCO 1.1](https://developercertificate.org/).
PRs with unsigned commits will be asked to rebase.

## Practical notes

- Keep PRs focused — one logical change per PR.
- Protocol changes require a spec update in the same PR.
- Breaking protocol changes are deferred to v2.0 (see stability guarantee in the spec).
