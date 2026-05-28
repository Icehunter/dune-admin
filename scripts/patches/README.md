# scripts/patches/

`install.sh` applies every `*.patch` file in this directory (in lexical
order) to the source tree after the `git checkout`. Use this slot for:

- **Unmerged upstream fixes** you want to layer in while a PR is in review.
- **Operator-local modifications** (custom branding, internal tweaks)
  that you don't want to commit upstream.

## How patches are applied

The script runs `git apply --check` first; if the patch fails *and* a
reverse-apply succeeds, the patch is assumed to be already applied and
is skipped silently. Otherwise the install aborts and prints the
`git apply --check` output so you can debug.

Re-running the install is safe — the source tree is reset to the
upstream branch before patches are applied, so you always get the same
result.

## How to generate a patch

From a working tree that already contains the change you want to ship:

```bash
git diff origin/<branch> -- <files> > scripts/patches/0099-my-change.patch
# Sanity-check that it applies clean on a fresh checkout:
git stash && git apply --check scripts/patches/0099-my-change.patch && git stash pop
```

Number patches `NNNN-name.patch` (4 digits) so lexical order matches
intended apply order if patches depend on each other.

## How to opt out

```bash
./install.sh --no-patches
# or point at an alternate directory:
./install.sh --patches-dir /path/to/other/patches
```

This directory ships empty in the repo. Operators populate it as needed.
